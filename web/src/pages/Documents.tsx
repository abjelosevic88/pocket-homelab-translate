// Documents: drop a file (or pick one from Pocket Files / Documents), it is
// translated in the background with its layout kept, then download it or
// put it back where it came from.
import { useCallback, useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, uploadForm, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { useData, useOnChange } from '../lib/data'
import { ago, fmtBytes } from '../lib/format'
import { engineLabel, langName } from '../lib/translate'
import type { AppState, Doc, EngineChoice } from '../lib/types'
import { Icon } from '../components/icons'
import { LangSelect } from '../components/LangSelect'
import { Confirm, Empty, Loading, Menu, Modal, Segmented, Spinner, errMsg, rectOf, useToast, type Anchor } from '../components/ui'
import { FilePicker } from '../components/FilePicker'

interface Format { ext: string; format: string; label: string }

export default function Documents() {
  const { settings } = useApp()
  const { byCode, engines } = useData()
  const { toast } = useToast()
  const [docs, setDocs] = useState<Doc[] | null>(null)
  const [formats, setFormats] = useState<Format[]>([])
  const [max, setMax] = useState(50 << 20)
  const [from, setFrom] = useState('auto')
  const [to, setTo] = useState(settings?.target_lang || 'en')
  const [engine, setEngine] = useState<EngineChoice>(settings?.engine || 'auto')
  const [uploading, setUploading] = useState(0)
  const [over, setOver] = useState(false)
  const [apps, setApps] = useState<AppState[]>([])
  const [picker, setPicker] = useState<'files' | 'documents' | null>(null)
  const [menu, setMenu] = useState<{ d: Doc; at: Anchor } | null>(null)
  const [again, setAgain] = useState<Doc | null>(null)
  const [del, setDel] = useState<Doc | null>(null)
  const input = useRef<HTMLInputElement>(null)

  const load = useCallback(() => api.get<{ documents: Doc[]; formats: Format[]; max_bytes: number }>(`${V1}/documents`).then(r => { setDocs(r.documents); setFormats(r.formats); setMax(r.max_bytes) }).catch(() => setDocs([])), [])
  useEffect(() => { load() }, [load])
  useOnChange('documents', load)
  useEffect(() => { api.get<{ apps: AppState[] }>(`${V1}/apps`).then(r => setApps(r.apps)).catch(() => {}) }, [])
  const busy = docs?.some(d => d.status === 'queued' || d.status === 'working')
  useEffect(() => {
    if (!busy) return
    const t = window.setInterval(load, 2000)
    return () => window.clearInterval(t)
  }, [busy, load])

  const send = async (files: FileList | File[]) => {
    for (const f of Array.from(files)) {
      if (f.size > max) { toast(`${f.name} is bigger than ${fmtBytes(max)}`, 'err'); continue }
      setUploading(n => n + 1)
      try { await uploadForm(`${V1}/documents`, f, { from, to, engine }); } catch (e) { toast(`${f.name}: ${errMsg(e)}`, 'err') } finally { setUploading(n => n - 1) }
    }
    load()
  }

  const fromApp = async (app: 'files' | 'documents', ref: string, name: string) => {
    setPicker(null)
    try { await api.post(`${V1}/documents/from`, { app, ref, name, from, to, engine }); load() } catch (e) { toast(errMsg(e), 'err') }
  }

  const save = async (d: Doc, app: 'files' | 'documents') => {
    try {
      const r = await api.post<{ title: string; url: string }>(`${V1}/documents/${d.id}/save`, { app })
      toast(`Saved in ${app === 'files' ? 'Pocket Files' : 'Pocket Documents'}`)
      load()
      if (r.url) window.open(r.url, '_blank', 'noopener')
    } catch (e) { toast(errMsg(e), 'err') }
  }

  const appOK = (slug: string) => apps.find(a => a.slug === slug)?.state === 'on' || apps.find(a => a.slug === slug)?.state === 'partly'
  const pdfOff = engines && !engines.pdf.ready
  const exts = formats.map(f => f.ext).join(',')
  return (
    <div className="page stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Documents</h1><p className="muted">Word, PowerPoint, PDF, text, Markdown, subtitles and web pages. They come back with the same layout; PDFs come back as Word documents.</p></div>
      </div>

      <section className={`drop ${over ? 'over' : ''}`} onDragOver={e => { e.preventDefault(); setOver(true) }} onDragLeave={() => setOver(false)} onDrop={e => { e.preventDefault(); setOver(false); send(e.dataTransfer.files) }}>
        <div className="tr-langs center">
          <LangSelect ariaLabel="From" value={from} auto onChange={setFrom} />
          <Icon name="chevron-right" size={18} className="muted" />
          <LangSelect ariaLabel="Into" value={to} onChange={setTo} />
        </div>
        <Segmented size="sm" label="Engine" value={engine} onChange={setEngine} options={[{ value: 'auto', label: 'Auto' }, { value: 'ai', label: 'AI', disabled: !engines?.ai.ready, title: engines?.ai.detail }, { value: 'offline', label: 'Offline' }]} />
        <Icon name="file" size={30} />
        <p><b>Drop documents here</b> or</p>
        <div className="row wrap center">
          <button type="button" className="btn primary" onClick={() => input.current?.click()}>{uploading ? <Spinner /> : <Icon name="upload" size={16} />}Choose files</button>
          <button type="button" className="btn" onClick={() => setPicker('files')} disabled={!appOK('files')} title={appOK('files') ? undefined : 'Pocket Files is not connected (Settings → Connected apps)'}><Icon name="file" size={16} />From Pocket Files</button>
          <button type="button" className="btn" onClick={() => setPicker('documents')} disabled={!appOK('documents')} title={appOK('documents') ? undefined : 'Pocket Documents is not connected (Settings → Connected apps)'}><Icon name="archive" size={16} />From Pocket Documents</button>
        </div>
        <p className="muted small">Up to {fmtBytes(max)} each.{pdfOff && ' PDFs need pdftotext on this server.'} The AI keeps the meaning across a whole document; the offline engines work without it.</p>
        <input ref={input} type="file" multiple hidden accept={exts} onChange={e => { if (e.target.files) send(e.target.files); e.target.value = '' }} />
      </section>

      {!docs ? <Loading /> : docs.length === 0 ? <Empty icon="file" title="No documents yet">Translated documents stay here to download again until you remove them.</Empty> : (
        <ul className="list card">
          {docs.map(d => {
            const pct = d.total ? Math.round(d.done * 100 / d.total) : 0
            return (
              <li key={d.id} className="list-row doc-row">
                <span className={`list-ico st-${d.status === 'done' ? 'done' : d.status === 'failed' ? 'overdue' : 'ok'}`}><Icon name={d.status === 'failed' ? 'alert' : 'file'} size={16} /></span>
                <span className="grow min0">
                  <span className="row-title ellipsis">{d.status === 'done' ? d.result_name : d.name}</span>
                  <span className="muted small block-line ellipsis">
                    {langName(d.detected_lang || d.source_lang, byCode)} → {langName(d.target_lang, byCode)} · {fmtBytes(d.size)} · {ago(d.created_at)}
                    {d.status === 'done' && d.used && <> · {engineLabel(d.used)}</>}
                    {d.saved.length > 0 && <> · saved in {d.saved.map(s => s.app === 'files' ? 'Files' : 'Documents').join(', ')}</>}
                  </span>
                  {(d.status === 'working' || d.status === 'queued') && <span className="doc-progress" aria-label={`${pct}%`}><span style={{ width: `${d.status === 'queued' ? 0 : Math.max(3, pct)}%` }} /></span>}
                  {d.status === 'failed' && <span className="small bad block-line">{d.error}</span>}
                </span>
                <span className="row-side">
                  {d.status === 'done' ? <a className="btn sm" href={`${V1}/documents/${d.id}/result`}><Icon name="download" size={14} /><span className="hide-sm">Download</span></a>
                    : d.status === 'failed' ? <button type="button" className="btn sm" onClick={() => setAgain(d)}>Try again</button>
                      : <span className="pill"><Spinner size={11} /> {d.status === 'queued' ? 'Waiting' : `${d.done} of ${d.total || '…'}`}</span>}
                </span>
                <button type="button" className="btn ghost icon-only" aria-label="More" onClick={e => setMenu({ d, at: rectOf(e.currentTarget) })}><Icon name="more" size={16} /></button>
              </li>
            )
          })}
        </ul>
      )}

      {menu && <Menu anchor={menu.at} width={260} onClose={() => setMenu(null)} items={[
        ...(menu.d.status === 'done' ? [
          { label: 'Download the translation', icon: 'download', onSelect: () => { window.location.href = `${V1}/documents/${menu.d.id}/result` } },
          { label: 'Save in Pocket Files', icon: 'file', disabled: !appOK('files'), onSelect: () => save(menu.d, 'files') },
          { label: 'Save in Pocket Documents', icon: 'archive', disabled: !appOK('documents'), onSelect: () => save(menu.d, 'documents') },
          'sep' as const,
        ] : []),
        { label: 'Download the original', icon: 'file', onSelect: () => { window.location.href = `${V1}/documents/${menu.d.id}/original` } },
        ...(menu.d.status === 'done' || menu.d.status === 'failed' ? [{ label: 'Translate again…', icon: 'refresh', onSelect: () => setAgain(menu.d) }] : []),
        ...(menu.d.status !== 'working' ? [{ label: 'Remove', icon: 'trash', danger: true, onSelect: () => setDel(menu.d) }] : []),
      ]} />}
      {again && <AgainModal d={again} onClose={() => setAgain(null)} onDone={load} />}
      {del && <Confirm title="Remove this document?" message={<>“{del.name}” and its translation are removed from Pocket Translate. Copies saved in your other apps stay.</>} confirmLabel="Remove"
        onCancel={() => setDel(null)} onConfirm={async () => { const d = del; setDel(null); try { await api.del(`${V1}/documents/${d.id}`); load() } catch (e) { toast(errMsg(e), 'err') } }} />}
      {picker && <FilePicker app={picker} onClose={() => setPicker(null)} onPick={(ref, name) => fromApp(picker, ref, name)} accept={formats.map(f => f.ext)} />}
      {!engines?.ai.ready && !engines?.argos.ready && !engines?.nllb.ready && engines && <div className="callout warn"><Icon name="alert" size={15} /><div>No engine is ready to translate. <Link to="/settings/engines">See the engines</Link>.</div></div>}
    </div>
  )
}

function AgainModal({ d, onClose, onDone }: { d: Doc; onClose: () => void; onDone: () => void }) {
  const { engines } = useData()
  const { toast } = useToast()
  const [to, setTo] = useState(d.target_lang)
  const [engine, setEngine] = useState<EngineChoice>(d.engine || 'auto')
  const go = async () => {
    try { await api.post(`${V1}/documents/${d.id}/retry`, { to, engine }); onDone(); onClose() } catch (e) { toast(errMsg(e), 'err') }
  }
  return (
    <Modal title="Translate again" onClose={onClose} size="sm" footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" onClick={go}>Translate</button></>}>
      <div className="stack">
        <p className="muted small">“{d.name}” is translated again; the last translation is replaced.</p>
        <LangSelect ariaLabel="Into" value={to} onChange={setTo} />
        <Segmented size="sm" value={engine} onChange={setEngine} options={[{ value: 'auto', label: 'Auto' }, { value: 'ai', label: 'AI', disabled: !engines?.ai.ready }, { value: 'offline', label: 'Offline' }]} />
      </div>
    </Modal>
  )
}
