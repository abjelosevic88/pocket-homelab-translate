// Picking a document in Pocket Files (folder by folder) or Pocket Documents (search).
import { useEffect, useState } from 'react'
import { api, V1 } from '../lib/api'
import { fmtBytes } from '../lib/format'
import { Icon } from './icons'
import { Empty, Loading, Modal, errMsg, useDebounce } from './ui'

interface Entry { name: string; path: string; dir: boolean; size: number; mime: string }
interface DocHit { id: number; title: string; created: string; mime: string }

export function FilePicker({ app, onClose, onPick, accept }: { app: 'files' | 'documents'; onClose: () => void; onPick: (ref: string, name: string) => void; accept: string[] }) {
  const [path, setPath] = useState('')
  const [entries, setEntries] = useState<Entry[] | null>(null)
  const [q, setQ] = useState('')
  const dq = useDebounce(q, 300)
  const [hits, setHits] = useState<DocHit[] | null>(null)
  const [err, setErr] = useState('')
  useEffect(() => {
    if (app !== 'files') return
    setEntries(null); setErr('')
    api.get<{ entries: Entry[] }>(`${V1}/apps/files?path=${encodeURIComponent(path)}`).then(r => setEntries(r.entries)).catch(e => setErr(errMsg(e)))
  }, [app, path])
  useEffect(() => {
    if (app !== 'documents') return
    setHits(null); setErr('')
    api.get<{ documents: DocHit[] }>(`${V1}/apps/documents?q=${encodeURIComponent(dq)}`).then(r => setHits(r.documents)).catch(e => setErr(errMsg(e)))
  }, [app, dq])
  const ok = (name: string) => accept.some(x => name.toLowerCase().endsWith(x))
  const up = path.includes('/') ? path.slice(0, path.lastIndexOf('/')) : ''
  return (
    <Modal title={app === 'files' ? 'Pick from Pocket Files' : 'Pick from Pocket Documents'} onClose={onClose} size="lg">
      {err ? <div className="callout danger"><Icon name="alert" size={15} /><span>{err}</span></div> : app === 'files' ? (
        <div className="stack">
          <div className="crumbs small"><button type="button" className="linkish" onClick={() => setPath('')}>Home</button>{path && path.split('/').map((p, i, all) => <span key={i}> / <button type="button" className="linkish" onClick={() => setPath(all.slice(0, i + 1).join('/'))}>{p}</button></span>)}</div>
          {!entries ? <Loading /> : (
            <ul className="list pick-files">
              {path && <li><button type="button" className="list-row" onClick={() => setPath(up)}><Icon name="chevron-left" size={16} /><span className="grow">Back</span></button></li>}
              {entries.filter(e => e.dir || ok(e.name)).map(e => (
                <li key={e.path}>
                  <button type="button" className="list-row" onClick={() => e.dir ? setPath(e.path.replace(/^\/+/, '')) : onPick(e.path, e.name)}>
                    <Icon name={e.dir ? 'archive' : 'file'} size={16} /><span className="grow ellipsis">{e.name}</span>{!e.dir && <span className="muted small">{fmtBytes(e.size)}</span>}
                  </button>
                </li>
              ))}
              {entries.filter(e => e.dir || ok(e.name)).length === 0 && <li className="muted small pad">Nothing here that can be translated.</li>}
            </ul>
          )}
        </div>
      ) : (
        <div className="stack">
          <input value={q} onChange={e => setQ(e.target.value)} placeholder="Search your documents" autoFocus />
          {!hits ? <Loading /> : hits.length === 0 ? <Empty title="Nothing found" /> : (
            <ul className="list pick-files">
              {hits.map(h => <li key={h.id}><button type="button" className="list-row" onClick={() => onPick(String(h.id), h.title)}><Icon name="file" size={16} /><span className="grow ellipsis">{h.title}</span><span className="muted small">{h.created?.slice(0, 10)}</span></button></li>)}
            </ul>
          )}
        </div>
      )}
    </Modal>
  )
}
