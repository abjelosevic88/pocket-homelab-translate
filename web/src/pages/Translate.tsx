// The translator: type, paste, dictate or drop a photo on one side, the
// translation on the other. Offline engines translate as you type; the AI
// when you press Translate (or ⌘/Ctrl + Enter).
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { api, uploadForm, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, useData } from '../lib/data'
import { canRecord, canSpeak, useRecorder, useSpeaking } from '../lib/speech'
import { dirOf, effectiveEngine, engineLabel, langName, speechTag } from '../lib/translate'
import type { EngineChoice, Settings, Tone, TranslateResult, Translation } from '../lib/types'
import { ago } from '../lib/format'
import { Icon } from '../components/icons'
import { LangSelect } from '../components/LangSelect'
import { Segmented, Spinner, errMsg, useToast } from '../components/ui'
import { Select } from '../components/Select'
import { CollectionModal, ExplainModal } from '../components/PhraseTools'

const MAX = 20000

export default function Translate() {
  const { settings, setSettings } = useApp()
  const { byCode, engines } = useData()
  const { toast } = useToast()
  const navigate = useNavigate()
  const [sp, setSp] = useSearchParams()
  const [from, setFrom] = useState(settings?.source_lang || 'auto')
  const [to, setTo] = useState(settings?.target_lang || 'en')
  const [engine, setEngine] = useState<EngineChoice>(settings?.engine || 'auto')
  const [tone, setTone] = useState<Tone>(settings?.tone || '')
  const [text, setText] = useState(() => sp.get('text') || sessionStorage.getItem('ptl-draft') || '')
  const [res, setRes] = useState<TranslateResult | null>(null)
  const resRef = useRef<TranslateResult | null>(null)
  resRef.current = res
  const [saved, setSaved] = useState<Translation | null>(null)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const [kind, setKind] = useState<'text' | 'photo' | 'speech'>('text')
  const [explain, setExplain] = useState(false)
  const [collect, setCollect] = useState(false)
  const [recent, setRecent] = useState<Translation[]>([])
  const [speakKey, toggleSpeak] = useSpeaking()
  const rec = useRecorder(90)
  const [hearing, setHearing] = useState(false)
  const box = useRef<HTMLTextAreaElement>(null)
  const seq = useRef(0)
  const keepTimer = useRef(0)
  const lastKept = useRef('')

  const [refining, setRefining] = useState(false)
  const [altBusy, setAltBusy] = useState(false)

  const eff = effectiveEngine(engine, engines)
  const offlineReady = !!engines && (engines.argos.ready || engines.nllb.ready)
  // as you type: the offline engines right away; with the AI, an offline
  // draft first and the AI's translation once you pause
  const live = !!settings?.live_translate && (eff === 'offline' || (eff === 'ai' && offlineReady))
  const aiReady = !!engines?.ai.ready

  useEffect(() => { sessionStorage.setItem('ptl-draft', text) }, [text])
  useEffect(() => { if (sp.get('text')) { sp.delete('text'); setSp(sp, { replace: true }) } }, []) // eslint-disable-line react-hooks/exhaustive-deps
  const loadRecent = useCallback(() => { api.get<{ translations: Translation[] }>(`${V1}/translations?limit=6`).then(r => setRecent(r.translations)).catch(() => {}) }, [])
  useEffect(loadRecent, [loadRecent])

  // the person's choices are remembered
  const remember = (patch: Partial<Settings>) => { api.put<Settings>(`${V1}/settings`, patch).then(setSettings).catch(() => {}) }

  /** Keeps a live translation in the history once it has settled. */
  const keep = (r: TranslateResult, src: string, k: string) => {
    window.clearTimeout(keepTimer.current)
    if (!settings?.keep_history || r.translation) return
    keepTimer.current = window.setTimeout(async () => {
      const key = src + '\u0000' + r.to
      if (lastKept.current === key) return
      lastKept.current = key
      try {
        const t = await api.post<Translation & { kept?: boolean }>(`${V1}/translations`, { source_text: src, result_text: r.text, source_lang: from, target_lang: r.to, detected_lang: r.detected, engine: r.engine, provider: r.provider, kind: k, tone, starred: false })
        if (t.id) { setSaved(t); loadRecent(); announceChange('history') }
      } catch { /* not kept: fine */ }
    }, 3500)
  }

  /** A quick offline translation, shown while the AI works (or as you type). */
  const draft = (src: string, n: number, done: () => boolean) => {
    api.post<TranslateResult>(`${V1}/translate`, { text: src, from, to, engine: 'offline', save: false })
      .then(r => { if (n === seq.current && !done()) { setRes({ ...r, quick: true }); setErr('') } })
      .catch(() => { /* the AI answers anyway */ })
  }

  const run = async (opts: { save: boolean; alts?: boolean; src?: string; quick?: boolean } = { save: true }) => {
    const src = opts.src ?? text
    if (!src.trim()) { setRes(null); setErr(''); setSaved(null); return }
    const n = ++seq.current
    setBusy(true)
    setErr('')
    let done = false
    const ai = eff === 'ai'
    if (ai && offlineReady && opts.quick !== false) draft(src, n, () => done)
    setRefining(ai)
    try {
      const r = await api.post<TranslateResult>(`${V1}/translate`, { text: src, from, to, engine, tone, alternatives: !!opts.alts, kind, save: opts.save })
      done = true
      if (n !== seq.current) return
      setRes(r)
      setSaved(r.translation || null)
      if (r.translation) { loadRecent(); announceChange('history') } else if (!opts.save) keep(r, src, kind)
    } catch (e) {
      done = true
      if (n !== seq.current) return
      // keep a quick draft that is already there, and say why it stayed
      const cur = resRef.current
      if (cur?.quick) setRes({ ...cur, note: `The AI did not answer (${errMsg(e)}), so this is the offline translation.` })
      else { setErr(errMsg(e)); setRes(null) }
    } finally {
      if (n === seq.current) { setBusy(false); setRefining(false) }
    }
  }

  /** Other ways to say it, asked for when wanted (they cost the AI a second more). */
  const moreWays = async () => {
    if (!res) return
    setAltBusy(true)
    try {
      const r = await api.post<TranslateResult>(`${V1}/translate`, { text, from, to, engine: 'ai', tone, alternatives: true, save: false })
      setRes(cur => cur ? { ...cur, alternatives: r.alternatives.filter(a => a !== cur.text).length ? r.alternatives.filter(a => a !== cur.text) : [r.text].filter(a => a !== cur.text) } : cur)
    } catch (e) { toast(errMsg(e), 'err') } finally { setAltBusy(false) }
  }
  const runRef = useRef(run)
  runRef.current = run

  // as you type: offline drafts quickly, the AI (when it is the engine) once you pause
  useEffect(() => {
    if (!live) return
    if (eff === 'offline') {
      const t = window.setTimeout(() => runRef.current({ save: false }), text.length > 400 ? 900 : 550)
      return () => window.clearTimeout(t)
    }
    if (!text.trim()) { setRes(null); setErr(''); setSaved(null); return }
    const t1 = window.setTimeout(() => draftRef.current(), text.length > 400 ? 500 : 250)
    const t2 = window.setTimeout(() => runRef.current({ save: false, quick: false }), text.length > 400 ? 1600 : 900)
    return () => { window.clearTimeout(t1); window.clearTimeout(t2) }
  }, [text, from, to, live, tone, eff])
  const draftRef = useRef(() => {})
  draftRef.current = () => { const n = ++seq.current; setRefining(true); draft(text, n, () => false) }
  // a new pair or engine translates the same text again
  useEffect(() => { if (!live && res && text.trim()) runRef.current({ save: true }) }, [to, from, engine, tone]) // eslint-disable-line react-hooks/exhaustive-deps

  const swap = () => {
    const src = from === 'auto' ? (res?.detected || '') : from
    if (!src || src === to) { toast('Pick the language of your text first', 'err'); return }
    setFrom(to)
    setTo(src)
    if (res?.text) setText(res.text)
    setRes(null)
    setSaved(null)
    remember({ source_lang: to, target_lang: src })
  }

  const onKey = (e: React.KeyboardEvent) => {
    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') { e.preventDefault(); run({ save: true }) }
  }

  const clear = () => { setText(''); setRes(null); setSaved(null); setErr(''); setKind('text'); box.current?.focus() }

  const copy = async () => {
    if (!res) return
    try { await navigator.clipboard.writeText(res.text); toast('Copied') } catch { toast('Could not copy', 'err') }
  }

  const ensureSaved = async (): Promise<Translation | null> => {
    if (saved) return saved
    if (!res) return null
    window.clearTimeout(keepTimer.current)
    const t = await api.post<Translation>(`${V1}/translations`, { source_text: text, result_text: res.text, source_lang: from, target_lang: res.to, detected_lang: res.detected, engine: res.engine, provider: res.provider, kind, tone, starred: true })
    setSaved(t)
    return t
  }

  const star = async () => {
    try {
      if (saved) {
        const t = await api.put<Translation>(`${V1}/translations/${saved.id}`, { starred: !saved.starred })
        setSaved(t)
        toast(t.starred ? 'In your phrasebook' : 'Taken out of your phrasebook')
      } else {
        await ensureSaved()
        toast('In your phrasebook')
      }
      announceChange('phrasebook')
      loadRecent()
    } catch (e) { toast(errMsg(e), 'err') }
  }

  const dictate = async () => {
    if (rec.recording) {
      const r = await rec.stop()
      if (!r) return
      setHearing(true)
      try {
        const lang = from === 'auto' ? '' : from
        const out = await uploadForm<{ text: string; language: string }>(`${V1}/speech`, new File([r.blob], r.name, { type: r.blob.type }), lang ? { language: lang } : {})
        const next = text.trim() ? text.trimEnd() + ' ' + out.text : out.text
        setText(next)
        setKind('speech')
        if (!live) run({ save: true, src: next })
      } catch (e) { toast(errMsg(e), 'err') } finally { setHearing(false) }
      return
    }
    try { await rec.start() } catch { toast('The microphone could not be used (allow it in the browser)', 'err') }
  }

  const onPaste = (e: React.ClipboardEvent) => {
    const file = Array.from(e.clipboardData.files).find(f => f.type.startsWith('image/'))
    if (file) { e.preventDefault(); navigate('/photo', { state: { file } }) }
  }

  const detected = from === 'auto' ? res?.detected : undefined
  const srcLang = from === 'auto' ? (res?.detected || '') : from
  const overLimit = text.length > MAX
  const toneOff = eff !== 'ai'
  const engineOpts = useMemo(() => [
    { value: 'auto' as EngineChoice, label: 'Auto', title: 'The AI when there is one, else the offline engine' },
    { value: 'ai' as EngineChoice, label: 'AI', disabled: !aiReady, title: aiReady ? 'Pocket AI: tone, context, alternatives' : (engines?.ai.detail || 'No AI is set up (Settings → AI)') },
    { value: 'offline' as EngineChoice, label: 'Offline', disabled: !!engines && !engines.argos.ready && !engines.nllb.ready, title: 'On this server only: Argos and NLLB' },
  ], [aiReady, engines])

  return (
    <div className="tr-page">
      <div className="tr-bar">
        <div className="tr-langs">
          <LangSelect ariaLabel="From" value={from} auto detected={detected} onChange={v => { setFrom(v); remember({ source_lang: v }) }} />
          <button type="button" className="btn ghost icon-only tr-swap" onClick={swap} aria-label="Swap languages" title="Swap languages"><Icon name="swap" size={18} /></button>
          <LangSelect ariaLabel="To" value={to} onChange={v => { setTo(v); remember({ target_lang: v }) }} />
        </div>
        <div className="tr-opts">
          <Segmented size="sm" label="Engine" value={engine} onChange={v => { setEngine(v); remember({ engine: v }) }} options={engineOpts} />
          <Select className="tr-tone" ariaLabel="Tone" value={tone} disabled={toneOff} onChange={v => { setTone(v as Tone); remember({ tone: v as Tone }) }}
            options={[{ value: '', label: 'Any tone' }, { value: 'formal', label: 'Formal' }, { value: 'casual', label: 'Casual' }]} />
        </div>
      </div>

      <div className="tr-panes">
        <section className={`tr-pane src ${rec.recording ? 'rec' : ''}`}>
          <textarea ref={box} className="tr-text" dir={dirOf(srcLang, byCode)} value={text} onChange={e => { setText(e.target.value); if (kind !== 'text' && !e.target.value) setKind('text') }} onKeyDown={onKey} onPaste={onPaste}
            placeholder={live ? (eff === 'ai' ? 'Type or paste text. A quick translation shows as you type; the AI polishes it when you pause.' : 'Type or paste text. It is translated as you type.') : 'Type or paste text, then press Translate (⌘/Ctrl + Enter).'} aria-label="Text to translate" autoFocus spellCheck />
          <div className="tr-tools">
            {canRecord && <button type="button" className={`btn ghost icon-only ${rec.recording ? 'tr-rec' : ''}`} onClick={dictate} disabled={hearing || (engines ? !engines.speech.ready && !rec.recording : false)}
              title={engines && !engines.speech.ready ? engines.speech.detail : rec.recording ? 'Stop and write it out' : 'Dictate'} aria-label={rec.recording ? 'Stop dictating' : 'Dictate'}>
              {hearing ? <Spinner /> : <Icon name={rec.recording ? 'stop' : 'mic'} size={18} />}
            </button>}
            {rec.recording && <span className="tr-meter" aria-hidden="true"><span style={{ transform: `scaleX(${0.08 + rec.level * 0.92})` }} /></span>}
            {rec.recording && <span className="muted small tnum">{rec.seconds}s</span>}
            <Link to="/photo" className="btn ghost icon-only" title="Translate a photo" aria-label="Translate a photo"><Icon name="camera" size={18} /></Link>
            {canSpeak && text.trim() && srcLang && <button type="button" className={`btn ghost icon-only ${speakKey === 'src' ? 'on' : ''}`} onClick={() => toggleSpeak('src', text, speechTag(srcLang, byCode))} title="Read aloud" aria-label="Read aloud"><Icon name="volume" size={18} /></button>}
            <span className="spacer" />
            <span className={`small tnum ${overLimit ? 'bad' : 'muted'}`}>{text.length.toLocaleString()}{overLimit ? ` / ${MAX.toLocaleString()}` : ''}</span>
            <button type="button" className="btn ghost icon-only" onClick={clear} disabled={!text} title="Clear" aria-label="Clear"><Icon name="x" size={18} /></button>
          </div>
        </section>

        <section className="tr-pane out" aria-live="polite">
          <div className="tr-result" dir={dirOf(to, byCode)}>
            {err ? <div className="tr-err"><Icon name="alert" size={16} /><div><b>Not translated.</b> {err}{/AI|offline|engine/i.test(err) && <> <Link to="/settings/engines">See the engines</Link>.</>}</div></div>
              : res ? <p className={`tr-out ${busy && !res.quick ? 'stale' : ''}`}>{res.text}</p>
                : <p className="tr-placeholder">{busy ? '' : 'The translation appears here.'}</p>}
          </div>
          {res && !err && res.alternatives.length > 0 && (
            <div className="tr-alts"><span className="muted small">Or:</span>{res.alternatives.map(a => <button key={a} type="button" className="chip-btn" onClick={() => setRes({ ...res, text: a })} title="Use this one">{a}</button>)}</div>
          )}
          <div className="tr-tools">
            <span className="tr-meta muted small">
              {busy && !res?.quick ? <><Spinner size={12} /> Translating…</> : res && !err ? <>
                <Icon name={res.engine === 'ai' ? 'sparkles' : 'server'} size={12} /> {res.quick && refining ? <>Quick translation · {engines?.ai.provider === 'openai' ? 'the AI' : 'Pocket AI'} is polishing it…</> : engineLabel(res.engine, res.provider)}
                {detected && <> · from {langName(detected, byCode)}</>}
                {res.terms > 0 && <> · {res.terms} glossary {res.terms === 1 ? 'term' : 'terms'}</>}
              </> : !live && text.trim() ? 'Press Translate' : ''}
            </span>
            <span className="spacer" />
            {res && !err && <>
              {canSpeak && <button type="button" className={`btn ghost icon-only ${speakKey === 'out' ? 'on' : ''}`} onClick={() => toggleSpeak('out', res.text, speechTag(res.to, byCode))} title="Read aloud" aria-label="Read the translation aloud"><Icon name="volume" size={18} /></button>}
              <button type="button" className="btn ghost icon-only" onClick={copy} title="Copy" aria-label="Copy the translation"><Icon name="copy" size={18} /></button>
              <button type="button" className={`btn ghost icon-only ${saved?.starred ? 'starred' : ''}`} onClick={star} title={saved?.starred ? 'In your phrasebook' : 'Save to your phrasebook'} aria-label="Save to your phrasebook" aria-pressed={!!saved?.starred}><Icon name="star" size={18} /></button>
              {saved?.starred && <button type="button" className="btn ghost sm" onClick={() => setCollect(true)}>{saved.collection || 'Collection…'}</button>}
              {res.engine === 'ai' && res.alternatives.length === 0 && <button type="button" className="btn ghost icon-only" onClick={moreWays} disabled={altBusy} title="Other ways to say it" aria-label="Other ways to say it">{altBusy ? <Spinner /> : <Icon name="list" size={18} />}</button>}
              <button type="button" className="btn ghost icon-only" onClick={() => setExplain(true)} disabled={!aiReady} title={aiReady ? 'Explain this phrase' : 'Explaining needs AI'} aria-label="Explain"><Icon name="help" size={18} /></button>
            </>}
          </div>
          {res?.note && <p className="tr-note small"><Icon name="info" size={13} /> {res.note}</p>}
        </section>
      </div>

      <div className="tr-actions">
        {!live && <button type="button" className="btn primary" onClick={() => run({ save: true })} disabled={busy || !text.trim() || overLimit}>
          {busy ? <Spinner /> : <Icon name="languages" size={16} />}Translate
        </button>}
        {eff === 'none' && engines && <div className="callout warn"><Icon name="alert" size={15} /><div>Nothing can translate yet: {engine === 'ai' ? engines.ai.detail || 'no AI is set up' : 'no engine is ready'}. <Link to="/settings/engines">Set one up</Link>.</div></div>}
      </div>

      {recent.length > 0 && <section className="tr-recent">
        <div className="sec-head"><h3>Recent</h3><span className="spacer" /><Link to="/history" className="btn ghost sm">All history</Link></div>
        <ul className="list card">
          {recent.map(t => (
            <li key={t.id}>
              <button type="button" className="list-row tr-recent-row" onClick={() => { setText(t.source_text); setFrom(t.source_lang || 'auto'); setTo(t.target_lang); setRes({ text: t.result_text, detected: t.detected_lang, engine: t.engine, provider: t.model, alternatives: [], terms: 0, note: '', to: t.target_lang }); setSaved(t); window.scrollTo({ top: 0 }) }}>
                <span className="grow min0">
                  <span className="row-title ellipsis">{t.source_text}</span>
                  <span className="muted small ellipsis block-line">{t.result_text}</span>
                </span>
                <span className="row-side"><span className="pill">{langName(t.detected_lang || t.source_lang, byCode)} → {langName(t.target_lang, byCode)}</span><span className="muted tiny">{ago(t.created_at)}</span></span>
                {t.starred && <Icon name="star" size={14} className="star-on" />}
              </button>
            </li>
          ))}
        </ul>
      </section>}

      {explain && res && <ExplainModal text={text} translation={res.text} from={srcLang} to={res.to} onClose={() => setExplain(false)} />}
      {collect && saved && <CollectionModal t={saved} onClose={() => setCollect(false)} onSaved={t => { setSaved(t); announceChange('phrasebook') }} />}
    </div>
  )
}
