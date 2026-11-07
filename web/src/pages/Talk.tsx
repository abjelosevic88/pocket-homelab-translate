// Conversations: two people, two languages. Each says something (or types
// it); it is written out, translated for the other and read aloud. On a
// phone "face to face" turns the other person's half around.
import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, uploadForm, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { useData, useOnChange, announceChange } from '../lib/data'
import { ago, fmtISOTime } from '../lib/format'
import { canRecord, canSpeak, speak, useRecorder, useSpeaking } from '../lib/speech'
import { dirOf, langName, langShort, speechTag } from '../lib/translate'
import type { Conversation, Turn } from '../lib/types'
import { Icon } from '../components/icons'
import { LangSelect } from '../components/LangSelect'
import { Confirm, Empty, Loading, MenuButton, Modal, Spinner, errMsg, useIsMobile, useToast } from '../components/ui'
import { useCrumb } from '../components/Layout'
import { sendToNotes } from '../components/PhraseTools'

export default function Talk() {
  const { id } = useParams()
  return id ? <ConversationView id={Number(id)} /> : <ConversationList />
}

function ConversationList() {
  const { byCode } = useData()
  const [list, setList] = useState<Conversation[] | null>(null)
  const [adding, setAdding] = useState(false)
  const load = useCallback(() => { api.get<{ conversations: Conversation[] }>(`${V1}/conversations`).then(r => setList(r.conversations)).catch(() => setList([])) }, [])
  useEffect(load, [load])
  useOnChange('conversations', load)
  return (
    <div className="page stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Conversation</h1><p className="muted">Talk with someone who speaks another language: each of you speaks, the other hears it in their language.</p></div>
        <button type="button" className="btn primary" onClick={() => setAdding(true)}><Icon name="plus" size={16} />New conversation</button>
      </div>
      {!list ? <Loading /> : list.length === 0 ? <Empty icon="message" title="No conversations yet">Start one with the button above: pick your language and theirs.</Empty> : (
        <ul className="list card">
          {list.map(c => (
            <li key={c.id}>
              <Link to={`/talk/${c.id}`} className="list-row">
                <span className="list-ico"><Icon name="message" size={16} /></span>
                <span className="grow min0">
                  <span className="row-title ellipsis">{c.title}</span>
                  <span className="muted small block-line ellipsis">{c.last || 'Nothing said yet'}</span>
                </span>
                <span className="row-side"><span className="pill">{langName(c.lang_a, byCode)} ↔ {langName(c.lang_b, byCode)}</span><span className="muted tiny">{c.turns} · {ago(c.updated_at)}</span></span>
              </Link>
            </li>
          ))}
        </ul>
      )}
      {adding && <NewConversation onClose={() => setAdding(false)} />}
    </div>
  )
}

export function NewConversation({ onClose }: { onClose: () => void }) {
  const { settings } = useApp()
  const navigate = useNavigate()
  const { toast } = useToast()
  const mine = settings?.recent_langs?.[0] || settings?.target_lang || 'en'
  const [a, setA] = useState(mine)
  const [b, setB] = useState(settings?.recent_langs?.find(c => c !== mine) || (mine === 'en' ? 'de' : 'en'))
  const [title, setTitle] = useState('')
  const go = async () => {
    try { const c = await api.post<Conversation>(`${V1}/conversations`, { lang_a: a, lang_b: b, title }); announceChange('conversations'); onClose(); navigate(`/talk/${c.id}`) } catch (e) { toast(errMsg(e), 'err') }
  }
  return (
    <Modal title="New conversation" onClose={onClose} size="sm" footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" onClick={go} disabled={a === b}>Start</button></>}>
      <div className="stack">
        <label className="field"><span className="label">You speak</span><LangSelect ariaLabel="You speak" value={a} onChange={setA} /></label>
        <label className="field"><span className="label">They speak</span><LangSelect ariaLabel="They speak" value={b} onChange={setB} /></label>
        <label className="field"><span className="label">Name (optional)</span><input value={title} onChange={e => setTitle(e.target.value)} placeholder="At the pharmacy" /></label>
      </div>
    </Modal>
  )
}

function ConversationView({ id }: { id: number }) {
  const { settings } = useApp()
  const { byCode, engines } = useData()
  const { toast } = useToast()
  const navigate = useNavigate()
  const mobile = useIsMobile()
  const [c, setC] = useState<Conversation | null>(null)
  const [turns, setTurns] = useState<Turn[] | null>(null)
  const [err, setErr] = useState('')
  const [face, setFace] = useState(() => localStorage.getItem('ptl-face') === '1')
  const [del, setDel] = useState(false)
  const [rename, setRename] = useState(false)
  const [speakKey, toggleSpeak] = useSpeaking()
  const list = useRef<HTMLDivElement>(null)
  useCrumb(c?.title)

  const load = useCallback(() => {
    api.get<{ conversation: Conversation; turns: Turn[] }>(`${V1}/conversations/${id}`).then(r => { setC(r.conversation); setTurns(r.turns) }).catch(e => setErr(errMsg(e)))
  }, [id])
  useEffect(load, [load])
  useEffect(() => { list.current?.scrollTo({ top: list.current.scrollHeight, behavior: 'smooth' }) }, [turns?.length])

  const added = (t: Turn, error?: string) => {
    setTurns(ts => [...(ts || []), t])
    if (error) { toast(`Not translated: ${error}`, 'err'); return }
    if (settings?.auto_speak && canSpeak && t.result_text) speak(t.result_text, speechTag(t.target_lang, byCode))
  }
  const setLangs = async (patch: Partial<Conversation>) => {
    if (!c) return
    try { setC(await api.put<Conversation>(`${V1}/conversations/${c.id}`, { ...patch })) } catch (e) { toast(errMsg(e), 'err') }
  }
  const retry = async (t: Turn) => {
    try { const r = await api.post<{ turn: Turn }>(`${V1}/turns/${t.id}/retry`); setTurns(ts => (ts || []).map(x => x.id === t.id ? r.turn : x)) } catch (e) { toast(errMsg(e), 'err') }
  }
  const remove = async (t: Turn) => { try { await api.del(`${V1}/turns/${t.id}`); setTurns(ts => (ts || []).filter(x => x.id !== t.id)) } catch (e) { toast(errMsg(e), 'err') } }

  if (err) return <div className="page"><div className="callout danger"><Icon name="alert" size={15} /><span>{err}</span></div></div>
  if (!c || !turns) return <Loading />
  const faceOn = face && mobile
  return (
    <div className={`talk ${faceOn ? 'face' : ''}`}>
      <div className="talk-head">
        <div className="tr-langs">
          <LangSelect ariaLabel="You speak" value={c.lang_a} onChange={v => setLangs({ lang_a: v })} />
          <button type="button" className="btn ghost icon-only" title="Swap sides" aria-label="Swap sides" onClick={() => setLangs({ lang_a: c.lang_b, lang_b: c.lang_a })}><Icon name="swap" size={18} /></button>
          <LangSelect ariaLabel="They speak" value={c.lang_b} onChange={v => setLangs({ lang_b: v })} />
        </div>
        <span className="spacer" />
        {mobile && <button type="button" className={`btn sm ${face ? 'primary' : ''}`} onClick={() => { setFace(!face); localStorage.setItem('ptl-face', face ? '0' : '1') }} title="Turn their half around, for a phone on the table"><Icon name="rotate-cw" size={14} />Face to face</button>}
        <MenuButton label="More" className="btn ghost icon-only" align="right" width={250} items={() => [
          { label: 'Rename', icon: 'edit', onSelect: () => setRename(true) },
          { label: 'Save as a note (Pocket Notes)', icon: 'note', onSelect: () => sendToNotes(`${V1}/conversations/${c.id}/note`, toast) },
          { label: 'Download as text', icon: 'download', onSelect: () => { window.location.href = `${V1}/conversations/${c.id}/transcript.txt` } },
          'sep',
          { label: 'Delete the conversation', icon: 'trash', danger: true, onSelect: () => setDel(true) },
        ]}><Icon name="more" size={18} /></MenuButton>
      </div>
      {engines && !engines.speech.ready && <p className="talk-hint muted small"><Icon name="info" size={12} /> Speaking needs speech to text: {engines.speech.detail}. You can type meanwhile.</p>}

      {faceOn && <Side c={c} side="b" flipped onAdded={added} />}
      <div className="talk-turns" ref={list}>
        {turns.length === 0 && <div className="talk-empty muted">
          <Icon name="message" size={22} />
          <p>Press the microphone on your side and speak; the other person presses theirs. Each of you hears the other in your own language.</p>
        </div>}
        {turns.map(t => {
          const mine = t.side === 'a'
          return (
            <div key={t.id} className={`bubble ${mine ? 'mine' : 'theirs'}`}>
              <div className="bubble-src" dir={dirOf(t.source_lang, byCode)}>{t.source_text}</div>
              {t.result_text ? <div className="bubble-out" dir={dirOf(t.target_lang, byCode)}>{t.result_text}</div>
                : <div className="bubble-out bad small">Not translated <button type="button" className="linkish" onClick={() => retry(t)}>Try again</button></div>}
              <div className="bubble-meta">
                <span className="muted tiny">{mine ? 'You' : 'They'} · {langShort(t.source_lang)} → {langShort(t.target_lang)} · {fmtISOTime(t.created_at)}{t.spoken ? '' : ' · typed'}</span>
                {canSpeak && t.result_text && <button type="button" className={`btn ghost icon-only xs ${speakKey === `t${t.id}` ? 'on' : ''}`} onClick={() => toggleSpeak(`t${t.id}`, t.result_text, speechTag(t.target_lang, byCode))} aria-label="Read aloud" title="Read aloud"><Icon name="volume" size={14} /></button>}
                <button type="button" className="btn ghost icon-only xs" onClick={() => remove(t)} aria-label="Remove" title="Remove"><Icon name="trash" size={13} /></button>
              </div>
            </div>
          )
        })}
      </div>
      <div className={`talk-sides ${faceOn ? 'one' : ''}`}>
        {!faceOn && <Side c={c} side="b" onAdded={added} />}
        <Side c={c} side="a" onAdded={added} />
      </div>
      {rename && <RenameModal c={c} onClose={() => setRename(false)} onSaved={setC} />}
      {del && <Confirm title="Delete this conversation?" message="Everything said in it is removed." confirmLabel="Delete" onCancel={() => setDel(false)}
        onConfirm={async () => { try { await api.del(`${V1}/conversations/${c.id}`); announceChange('conversations'); navigate('/talk') } catch (e) { toast(errMsg(e), 'err') } }} />}
    </div>
  )
}

/** One person's half: their language, a microphone and a line to type in. */
function Side({ c, side, flipped, onAdded }: { c: Conversation; side: 'a' | 'b'; flipped?: boolean; onAdded: (t: Turn, error?: string) => void }) {
  const { byCode, engines } = useData()
  const { toast } = useToast()
  const rec = useRecorder(90)
  const [busy, setBusy] = useState(false)
  const [text, setText] = useState('')
  const lang = side === 'a' ? c.lang_a : c.lang_b
  const name = langName(lang, byCode)
  const speechOK = !engines || engines.speech.ready
  const send = async (body: { text?: string; file?: File }) => {
    setBusy(true)
    try {
      const r = body.file
        ? await uploadForm<{ turn: Turn; error?: string }>(`${V1}/conversations/${c.id}/turns`, body.file, { side })
        : await api.post<{ turn: Turn; error?: string }>(`${V1}/conversations/${c.id}/turns`, { side, text: body.text })
      onAdded(r.turn, r.error)
      setText('')
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  const mic = async () => {
    if (rec.recording) {
      const r = await rec.stop()
      if (r) send({ file: new File([r.blob], r.name, { type: r.blob.type }) })
      return
    }
    try { await rec.start() } catch { toast('The microphone could not be used (allow it in the browser)', 'err') }
  }
  const label = side === 'a' ? 'You' : 'They'
  return (
    <div className={`side side-${side} ${flipped ? 'flipped' : ''} ${rec.recording ? 'rec' : ''}`}>
      <div className="side-who"><b>{label}</b> <span className="muted">· {name}</span></div>
      <div className="side-row">
        {canRecord && <button type="button" className={`mic-btn ${rec.recording ? 'on' : ''}`} onClick={mic} disabled={busy || !speechOK} aria-label={rec.recording ? 'Stop and translate' : `Speak ${name}`} title={speechOK ? undefined : engines?.speech.detail}>
          {busy ? <Spinner size={18} /> : <Icon name={rec.recording ? 'stop' : 'mic'} size={22} />}
          {rec.recording && <span className="mic-ring" style={{ transform: `scale(${1 + rec.level * 0.35})` }} aria-hidden="true" />}
        </button>}
        <form className="side-type" onSubmit={e => { e.preventDefault(); if (text.trim()) send({ text }) }}>
          <input value={text} onChange={e => setText(e.target.value)} placeholder={rec.recording ? `Listening… ${rec.seconds}s` : `Type in ${name}`} dir={dirOf(lang, byCode)} disabled={busy || rec.recording} aria-label={`Type in ${name}`} />
          <button type="submit" className="btn icon-only" disabled={busy || !text.trim()} aria-label="Send"><Icon name="send" size={16} /></button>
        </form>
      </div>
    </div>
  )
}

function RenameModal({ c, onClose, onSaved }: { c: Conversation; onClose: () => void; onSaved: (c: Conversation) => void }) {
  const [t, setT] = useState(c.title)
  const { toast } = useToast()
  const save = async () => { try { onSaved(await api.put<Conversation>(`${V1}/conversations/${c.id}`, { title: t })); announceChange('conversations'); onClose() } catch (e) { toast(errMsg(e), 'err') } }
  return (
    <Modal title="Rename" onClose={onClose} size="sm" footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" onClick={save}>Save</button></>}>
      <input value={t} onChange={e => setT(e.target.value)} autoFocus onKeyDown={e => { if (e.key === 'Enter') save() }} />
    </Modal>
  )
}
