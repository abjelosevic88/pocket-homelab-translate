// Bringing phrases in: Google Translate's saved phrases (Takeout), a CSV,
// or a Pocket Translate export. The same file twice adds nothing twice.
import { useRef, useState } from 'react'
import { V1 } from '../lib/api'
import { announceChange } from '../lib/data'
import { Icon } from './icons'
import { Field, Modal, Spinner, errMsg, useToast } from './ui'

export function ImportModal({ onClose, onDone }: { onClose: () => void; onDone?: () => void }) {
  const { toast } = useToast()
  const input = useRef<HTMLInputElement>(null)
  const [file, setFile] = useState<File | null>(null)
  const [col, setCol] = useState('')
  const [busy, setBusy] = useState(false)
  const [out, setOut] = useState<Record<string, number> | null>(null)
  const go = async () => {
    if (!file) return
    setBusy(true)
    try {
      const res = await fetch(`${V1}/import${col ? `?collection=${encodeURIComponent(col)}` : ''}`, { method: 'POST', body: await file.arrayBuffer(), credentials: 'same-origin' })
      const j = await res.json()
      if (!res.ok) throw new Error(j.error || res.statusText)
      setOut(j)
      announceChange('phrasebook'); announceChange('history'); announceChange('conversations')
      onDone?.()
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title="Bring phrases in" onClose={onClose} footer={out ? <button type="button" className="btn primary" onClick={onClose}>Done</button> : <>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={go} disabled={!file || busy}>{busy && <Spinner />}Bring in</button>
    </>}>
      {out ? (
        <div className="stack">
          <p><b>{out.added ?? 0}</b> new {out.added === 1 ? 'phrase' : 'phrases'}{out.unchanged ? `, ${out.unchanged} already here` : ''}{out.skipped ? `, ${out.skipped} rows skipped` : ''}.</p>
          {(out.terms || out.conversations) ? <p className="muted small">Also {out.terms || 0} glossary terms and {out.conversations || 0} conversations.</p> : null}
        </div>
      ) : (
        <div className="stack">
          <p className="muted">From <b>Google Translate</b>: in Google Takeout pick “Translate”, unzip it and choose <code>Saved translations.csv</code>. Any CSV with the columns <i>from, to, text, translation</i> works too, and so does a Pocket Translate export (JSON) with everything in it.</p>
          <button type="button" className="btn" onClick={() => input.current?.click()}><Icon name="upload" size={15} />{file ? file.name : 'Choose a file'}</button>
          <input ref={input} type="file" hidden accept=".csv,.tsv,.txt,.json" onChange={e => setFile(e.target.files?.[0] || null)} />
          <Field label="Put the phrases in a collection (optional)"><input value={col} onChange={e => setCol(e.target.value)} placeholder="From Google Translate" /></Field>
        </div>
      )}
    </Modal>
  )
}
