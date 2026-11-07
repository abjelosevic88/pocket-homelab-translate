// Small tools shared by the translator, the phrasebook and the history:
// explaining a phrase, putting it in a collection, editing it, sending it
// to Pocket Learn or Pocket Notes.
import { useEffect, useState } from 'react'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { useData } from '../lib/data'
import { langName } from '../lib/translate'
import type { Collection, Translation } from '../lib/types'
import { Icon } from './icons'
import { LangSelect } from './LangSelect'
import { Field, Loading, Modal, Segmented, Switch, errMsg, useToast } from './ui'

interface Explanation { meaning: string; literal: string; usage: string; register: string; examples: { text: string; translation: string }[] }

export function ExplainModal({ text, translation, from, to, onClose }: { text: string; translation: string; from: string; to: string; onClose: () => void }) {
  const { byCode } = useData()
  const [which, setWhich] = useState<'src' | 'out'>('src')
  const [ex, setEx] = useState<Explanation | null>(null)
  const [err, setErr] = useState('')
  useEffect(() => {
    setEx(null); setErr('')
    const body = which === 'src' ? { text, from, to } : { text: translation, from: to, to: from || to }
    api.post<Explanation>(`${V1}/explain`, body).then(setEx).catch(e => setErr(errMsg(e)))
  }, [which, text, translation, from, to])
  return (
    <Modal title="Explain" onClose={onClose} size="lg">
      <div className="stack">
        {from && from !== to && <Segmented size="sm" value={which} onChange={setWhich} options={[{ value: 'src', label: `The ${langName(from, byCode)} text` }, { value: 'out', label: `The ${langName(to, byCode)} translation` }]} />}
        <blockquote className="sel-quote">{which === 'src' ? text : translation}</blockquote>
        {err ? <div className="callout danger"><Icon name="alert" size={15} /><span>{err}</span></div> : !ex ? <Loading label="Asking the AI…" /> : (
          <div className="explain">
            <p>{ex.meaning}</p>
            {ex.literal && <p><span className="muted">Word for word:</span> {ex.literal}</p>}
            {ex.usage && <p><span className="muted">How it is used:</span> {ex.usage}</p>}
            {ex.register && <p><span className="pill">{ex.register}</span></p>}
            {ex.examples?.length > 0 && <ul className="explain-ex">{ex.examples.map((e, i) => <li key={i}><b>{e.text}</b><span className="muted">{e.translation}</span></li>)}</ul>}
          </div>
        )}
      </div>
    </Modal>
  )
}

export function useCollections() {
  const [list, setList] = useState<Collection[]>([])
  useEffect(() => { api.get<{ collections: Collection[] }>(`${V1}/collections`).then(r => setList(r.collections)).catch(() => {}) }, [])
  return list
}

export function CollectionModal({ t, onClose, onSaved }: { t: Translation; onClose: () => void; onSaved: (t: Translation) => void }) {
  const cols = useCollections()
  const { toast } = useToast()
  const [name, setName] = useState(t.collection)
  const save = async (v: string) => {
    try { const n = await api.put<Translation>(`${V1}/translations/${t.id}`, { collection: v, starred: true }); onSaved(n); onClose() } catch (e) { toast(errMsg(e), 'err') }
  }
  const names = Array.from(new Set(['Travel', 'Work', 'Restaurant', 'Doctor', 'Everyday', ...cols.map(c => c.name).filter(Boolean)]))
  return (
    <Modal title="Collection" onClose={onClose} size="sm" footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" onClick={() => save(name.trim())}>Save</button></>}>
      <div className="stack">
        <p className="muted small">Collections keep your phrasebook in order: for a trip, for work, for the doctor.</p>
        <input value={name} onChange={e => setName(e.target.value)} placeholder="A new collection" autoFocus onKeyDown={e => { if (e.key === 'Enter') save(name.trim()) }} />
        <div className="chips">{names.map(n => <button key={n} type="button" className={`chip-btn ${n === name ? 'on' : ''}`} onClick={() => save(n)}>{n}</button>)}
          {t.collection && <button type="button" className="chip-btn" onClick={() => save('')}>No collection</button>}</div>
      </div>
    </Modal>
  )
}

/** Edit a phrase: its translation (your own correction), collection, note. */
export function EditPhraseModal({ t, onClose, onSaved }: { t: Translation; onClose: () => void; onSaved: (t: Translation) => void }) {
  const { byCode } = useData()
  const { toast } = useToast()
  const [res, setRes] = useState(t.result_text)
  const [col, setCol] = useState(t.collection)
  const [note, setNote] = useState(t.note)
  const [starred, setStarred] = useState(t.starred)
  const cols = useCollections()
  const save = async () => {
    try { onSaved(await api.put<Translation>(`${V1}/translations/${t.id}`, { result_text: res, collection: col, note, starred })); onClose() } catch (e) { toast(errMsg(e), 'err') }
  }
  return (
    <Modal title="Edit phrase" onClose={onClose} footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" onClick={save}>Save</button></>}>
      <div className="stack">
        <Field label={langName(t.detected_lang || t.source_lang, byCode)} group><blockquote className="sel-quote">{t.source_text}</blockquote></Field>
        <Field label={`${langName(t.target_lang, byCode)} — your own wording is kept`}><textarea rows={3} value={res} onChange={e => setRes(e.target.value)} /></Field>
        <div className="two-col tight">
          <Field label="Collection"><input list="ptl-cols" value={col} onChange={e => setCol(e.target.value)} placeholder="None" /></Field>
          <Field label="In your phrasebook" group><Switch checked={starred} onChange={setStarred} label="In your phrasebook" /></Field>
        </div>
        <datalist id="ptl-cols">{cols.map(c => c.name && <option key={c.name} value={c.name} />)}</datalist>
        <Field label="Note" help="When to say it, what to watch out for."><textarea rows={2} value={note} onChange={e => setNote(e.target.value)} /></Field>
      </div>
    </Modal>
  )
}

/** Add a phrase by hand (translated for you when you leave the translation empty). */
export function AddPhraseModal({ onClose, onSaved, collection }: { onClose: () => void; onSaved: (t: Translation) => void; collection?: string }) {
  const { settings } = useApp()
  const { toast } = useToast()
  const [src, setSrc] = useState('')
  const [res, setRes] = useState('')
  const [from, setFrom] = useState('auto')
  const [to, setTo] = useState(settings?.target_lang || 'en')
  const [col, setCol] = useState(collection || '')
  const [busy, setBusy] = useState(false)
  const save = async () => {
    setBusy(true)
    try { onSaved(await api.post<Translation>(`${V1}/translations`, { source_text: src, result_text: res, source_lang: from, target_lang: to, collection: col, starred: true })); onClose() } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title="Add a phrase" onClose={onClose} footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" disabled={!src.trim() || busy} onClick={save}>{res.trim() ? 'Add' : 'Translate and add'}</button></>}>
      <div className="stack">
        <div className="two-col tight">
          <Field label="From" group><LangSelect ariaLabel="From" value={from} auto onChange={setFrom} /></Field>
          <Field label="To" group><LangSelect ariaLabel="To" value={to} onChange={setTo} /></Field>
        </div>
        <Field label="Phrase"><textarea rows={2} value={src} onChange={e => setSrc(e.target.value)} autoFocus /></Field>
        <Field label="Translation" help="Leave it empty and it is translated for you."><textarea rows={2} value={res} onChange={e => setRes(e.target.value)} /></Field>
        <Field label="Collection"><input value={col} onChange={e => setCol(e.target.value)} placeholder="None" /></Field>
      </div>
    </Modal>
  )
}

export async function sendToLearn(t: Translation, toast: (m: string, k?: 'ok' | 'err') => void): Promise<Translation | null> {
  try {
    const r = await api.post<{ translation: Translation; deck: string; duplicate: boolean }>(`${V1}/translations/${t.id}/learn`, {})
    toast(r.duplicate ? `Already a card in ${r.deck}` : `A flashcard in ${r.deck} (Pocket Learn)`)
    return r.translation
  } catch (e) { toast(errMsg(e), 'err'); return null }
}

export async function sendToNotes(path: string, toast: (m: string, k?: 'ok' | 'err') => void) {
  try {
    const r = await api.post<{ url: string }>(path, {})
    toast('Saved in Pocket Notes')
    if (r.url) window.open(r.url, '_blank', 'noopener')
  } catch (e) { toast(errMsg(e), 'err') }
}
