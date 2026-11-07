// A photo of a menu, a sign or a letter: the text is read on the server
// (tesseract), you can fix what it got wrong, then it is translated.
import { useEffect, useRef, useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { api, uploadForm, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, useData } from '../lib/data'
import { canSpeak, useSpeaking } from '../lib/speech'
import { dirOf, engineLabel, langName, speechTag } from '../lib/translate'
import type { TranslateResult, Translation } from '../lib/types'
import { Icon } from '../components/icons'
import { LangSelect } from '../components/LangSelect'
import { Spinner, errMsg, useToast } from '../components/ui'
import { useCrumb } from '../components/Layout'

export default function Photo() {
  useCrumb(undefined)
  const { settings } = useApp()
  const { byCode, engines } = useData()
  const { toast } = useToast()
  const loc = useLocation()
  const [file, setFile] = useState<File | null>((loc.state as { file?: File } | null)?.file || null)
  const [url, setUrl] = useState('')
  const [from, setFrom] = useState('auto')
  const [to, setTo] = useState(settings?.target_lang || 'en')
  const [text, setText] = useState('')
  const [reading, setReading] = useState(false)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const [res, setRes] = useState<TranslateResult | null>(null)
  const [saved, setSaved] = useState<Translation | null>(null)
  const [lines, setLines] = useState(true)
  const [speakKey, toggleSpeak] = useSpeaking()
  const pick = useRef<HTMLInputElement>(null)
  const shoot = useRef<HTMLInputElement>(null)
  const ocrReady = engines ? engines.ocr.ready : true

  useEffect(() => {
    if (!file) { setUrl(''); return }
    const u = URL.createObjectURL(file)
    setUrl(u)
    return () => URL.revokeObjectURL(u)
  }, [file])

  const read = async (f: File) => {
    setReading(true); setErr(''); setText(''); setRes(null); setSaved(null)
    try {
      const r = await uploadForm<{ text: string }>(`${V1}/ocr`, f, from !== 'auto' ? { from } : {})
      setText(r.text)
      translate(r.text)
    } catch (e) { setErr(errMsg(e)) } finally { setReading(false) }
  }
  useEffect(() => { if (file) read(file) }, [file]) // eslint-disable-line react-hooks/exhaustive-deps

  const prepared = (t: string) => lines ? t : t.replace(/([^\n])\n(?!\n)/g, '$1 ')

  const translate = async (t = text) => {
    if (!t.trim()) return
    setBusy(true); setErr('')
    try {
      const r = await api.post<TranslateResult>(`${V1}/translate`, { text: prepared(t), from, to, kind: 'photo', about: 'text read from a photo (a menu, a sign, a label or a letter); fix obvious reading mistakes' })
      setRes(r); setSaved(r.translation || null)
      if (r.translation) announceChange('history')
    } catch (e) { setErr(errMsg(e)) } finally { setBusy(false) }
  }

  const onFile = (f: File | undefined) => {
    if (!f) return
    if (!f.type.startsWith('image/')) { toast('That is not a photo', 'err'); return }
    setFile(f)
  }

  const star = async () => {
    if (!res) return
    try {
      const t = saved ? await api.put<Translation>(`${V1}/translations/${saved.id}`, { starred: !saved.starred })
        : await api.post<Translation>(`${V1}/translations`, { source_text: prepared(text), result_text: res.text, source_lang: from, target_lang: res.to, detected_lang: res.detected, engine: res.engine, provider: res.provider, kind: 'photo', starred: true })
      setSaved(t); toast(t.starred ? 'In your phrasebook' : 'Taken out of your phrasebook'); announceChange('phrasebook')
    } catch (e) { toast(errMsg(e), 'err') }
  }

  useEffect(() => {
    const h = (e: ClipboardEvent) => { const f = Array.from(e.clipboardData?.files || []).find(x => x.type.startsWith('image/')); if (f) { e.preventDefault(); setFile(f) } }
    window.addEventListener('paste', h)
    return () => window.removeEventListener('paste', h)
  }, [])

  const src = from === 'auto' ? res?.detected || '' : from
  return (
    <div className="page stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Photo</h1><p className="muted">A menu, a sign, a label or a letter: take a photo and read it in your language.</p></div>
      </div>
      {!ocrReady && <div className="callout warn"><Icon name="alert" size={15} /><div>Reading photos needs tesseract on this server. It comes with the Docker image; <Link to="/settings/engines">see the engines</Link>.</div></div>}
      <div className="tr-bar">
        <div className="tr-langs">
          <LangSelect ariaLabel="The photo is in" value={from} auto detected={from === 'auto' ? res?.detected : undefined} onChange={v => setFrom(v)} />
          <Icon name="chevron-right" size={18} className="muted" />
          <LangSelect ariaLabel="Translate into" value={to} onChange={v => { setTo(v); if (text) setTimeout(() => translate(), 0) }} />
        </div>
      </div>
      {!file ? (
        <div className="drop" onDragOver={e => e.preventDefault()} onDrop={e => { e.preventDefault(); onFile(e.dataTransfer.files[0]) }}>
          <Icon name="camera" size={32} />
          <p><b>Take a photo or pick one.</b> You can also drop it here or paste it.</p>
          <div className="row wrap center">
            <button type="button" className="btn primary" onClick={() => shoot.current?.click()} disabled={!ocrReady}><Icon name="camera" size={16} />Take a photo</button>
            <button type="button" className="btn" onClick={() => pick.current?.click()} disabled={!ocrReady}><Icon name="image" size={16} />Pick a photo</button>
          </div>
          <p className="muted small">Hold the phone straight above the text, in good light. The photo is read on this server and not kept.</p>
        </div>
      ) : (
        <div className="photo-grid">
          <figure className="photo-fig">
            <img src={url} alt="Your photo" />
            <figcaption className="row wrap">
              <button type="button" className="btn sm" onClick={() => shoot.current?.click()}><Icon name="camera" size={14} />Another photo</button>
              <button type="button" className="btn sm ghost" onClick={() => file && read(file)} disabled={reading}><Icon name="refresh" size={14} />Read again</button>
              <button type="button" className="btn sm ghost" onClick={() => { setFile(null); setText(''); setRes(null) }}><Icon name="x" size={14} />Clear</button>
            </figcaption>
          </figure>
          <div className="stack">
            <section className="tr-pane src">
              {reading ? <div className="tr-reading"><Spinner /> Reading the text…</div> : (
                <textarea className="tr-text sm" dir={dirOf(src, byCode)} value={text} onChange={e => setText(e.target.value)} aria-label="Text read from the photo" placeholder="The text read from the photo appears here; fix what it got wrong." />
              )}
              <div className="tr-tools">
                <label className="check-row small"><input type="checkbox" checked={lines} onChange={e => setLines(e.target.checked)} /> Keep the lines (menus, lists)</label>
                <span className="spacer" />
                <button type="button" className="btn sm primary" onClick={() => translate()} disabled={busy || reading || !text.trim()}>{busy ? <Spinner /> : <Icon name="languages" size={14} />}Translate</button>
              </div>
            </section>
            <section className="tr-pane out">
              <div className="tr-result" dir={dirOf(to, byCode)}>
                {err ? <div className="tr-err"><Icon name="alert" size={16} /><div>{err}</div></div>
                  : res ? <p className={`tr-out ${busy ? 'stale' : ''}`}>{res.text}</p> : <p className="tr-placeholder">{reading || busy ? '' : 'The translation appears here.'}</p>}
              </div>
              {res && !err && <div className="tr-tools">
                <span className="tr-meta muted small">{engineLabel(res.engine, res.provider)}{res.detected && from === 'auto' && <> · from {langName(res.detected, byCode)}</>}</span>
                <span className="spacer" />
                {canSpeak && <button type="button" className={`btn ghost icon-only ${speakKey === 'out' ? 'on' : ''}`} onClick={() => toggleSpeak('out', res.text, speechTag(res.to, byCode))} aria-label="Read aloud" title="Read aloud"><Icon name="volume" size={18} /></button>}
                <button type="button" className="btn ghost icon-only" onClick={async () => { await navigator.clipboard.writeText(res.text).catch(() => {}); toast('Copied') }} aria-label="Copy" title="Copy"><Icon name="copy" size={18} /></button>
                <button type="button" className={`btn ghost icon-only ${saved?.starred ? 'starred' : ''}`} onClick={star} aria-label="Save to your phrasebook" title="Save to your phrasebook"><Icon name="star" size={18} /></button>
              </div>}
              {res?.note && <p className="tr-note small"><Icon name="info" size={13} /> {res.note}</p>}
            </section>
          </div>
        </div>
      )}
      <input ref={pick} type="file" accept="image/*" hidden onChange={e => { onFile(e.target.files?.[0]); e.target.value = '' }} />
      <input ref={shoot} type="file" accept="image/*" capture="environment" hidden onChange={e => { onFile(e.target.files?.[0]); e.target.value = '' }} />
    </div>
  )
}
