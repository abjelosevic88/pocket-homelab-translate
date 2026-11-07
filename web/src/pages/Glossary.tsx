// The glossary: your own words, always translated the same way (names,
// work terms, how your team says it). Every engine uses it.
import { useCallback, useEffect, useRef, useState } from 'react'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { useData } from '../lib/data'
import { langName, langShort } from '../lib/translate'
import type { Term } from '../lib/types'
import { Icon } from '../components/icons'
import { LangSelect } from '../components/LangSelect'
import { Confirm, Empty, Field, Loading, Modal, Switch, errMsg, useDebounce, useToast } from '../components/ui'

export default function Glossary() {
  const { byCode } = useData()
  const { toast } = useToast()
  const [q, setQ] = useState('')
  const dq = useDebounce(q, 250)
  const [list, setList] = useState<Term[] | null>(null)
  const [edit, setEdit] = useState<Partial<Term> | null>(null)
  const [del, setDel] = useState<Term | null>(null)
  const [importing, setImporting] = useState(false)
  const load = useCallback(() => { api.get<{ terms: Term[] }>(`${V1}/glossary?q=${encodeURIComponent(dq)}`).then(r => setList(r.terms)).catch(() => setList([])) }, [dq])
  useEffect(load, [load])

  const pairs: [string, Term[]][] = []
  for (const t of list || []) {
    const k = `${t.source_lang}→${t.target_lang}`
    const last = pairs[pairs.length - 1]
    if (last && last[0] === k) last[1].push(t); else pairs.push([k, [t]])
  }
  return (
    <div className="page stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Glossary</h1><p className="muted">Words that should always be translated the same way: names, products, the terms your work uses. The AI also gives them the right grammatical ending; the offline engines put them in as written.</p></div>
        <div className="row wrap">
          <button type="button" className="btn" onClick={() => setImporting(true)}><Icon name="upload" size={15} /><span className="hide-sm">Import</span></button>
          <a className="btn" href={`${V1}/glossary.csv`}><Icon name="download" size={15} /><span className="hide-sm">Export</span></a>
          <button type="button" className="btn primary" onClick={() => setEdit({ source_lang: 'en', target_lang: '' })}><Icon name="plus" size={15} />Add a term</button>
        </div>
      </div>
      <input className="search-in" value={q} onChange={e => setQ(e.target.value)} placeholder="Search the glossary" aria-label="Search" />
      {!list ? <Loading /> : list.length === 0 ? <Empty icon="book" title={dq ? 'Nothing found' : 'No terms yet'}>{dq ? 'Try other words.' : 'Add a term like “invoice → faktura” and it is used every time you translate from English into Serbian.'}</Empty> : pairs.map(([k, terms]) => (
        <section key={k} className="day-sec">
          <h3 className="day-h">{langName(terms[0].source_lang, byCode)} → {langName(terms[0].target_lang, byCode)}</h3>
          <ul className="list card">
            {terms.map(t => (
              <li key={t.id} className="list-row">
                <button type="button" className="grow min0 row-main as-btn" onClick={() => setEdit(t)}>
                  <span className="row-title"><span className="ellipsis">{t.term}</span><Icon name="chevron-right" size={13} className="muted" /><span className="ellipsis term-to">{t.translation}</span>{t.match_case && <span className="pill tiny" title="Only with the same capital letters">Aa</span>}</span>
                  {t.note && <span className="muted small ellipsis block-line">{t.note}</span>}
                </button>
                <span className="muted tiny">{langShort(t.source_lang === '*' ? 'any' : t.source_lang)} → {langShort(t.target_lang)}</span>
                <button type="button" className="btn ghost icon-only" onClick={() => setDel(t)} aria-label="Delete"><Icon name="trash" size={15} /></button>
              </li>
            ))}
          </ul>
        </section>
      ))}
      {edit && <TermModal t={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); load() }} />}
      {del && <Confirm title="Delete this term?" message={<>“{del.term} → {del.translation}” is no longer used.</>} confirmLabel="Delete" onCancel={() => setDel(null)}
        onConfirm={async () => { const t = del; setDel(null); try { await api.del(`${V1}/glossary/${t.id}`); load() } catch (e) { toast(errMsg(e), 'err') } }} />}
      {importing && <ImportTerms onClose={() => setImporting(false)} onDone={load} />}
    </div>
  )
}

function TermModal({ t, onClose, onSaved }: { t: Partial<Term>; onClose: () => void; onSaved: () => void }) {
  const { settings } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState<Partial<Term>>({ ...t, target_lang: t.target_lang || settings?.target_lang || 'en' })
  const save = async () => {
    try {
      if (t.id) await api.put(`${V1}/glossary/${t.id}`, f); else await api.post(`${V1}/glossary`, f)
      onSaved()
    } catch (e) { toast(errMsg(e), 'err') }
  }
  return (
    <Modal title={t.id ? 'Edit term' : 'Add a term'} onClose={onClose} footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" onClick={save} disabled={!f.term?.trim() || !f.translation?.trim()}>Save</button></>}>
      <div className="stack">
        <div className="two-col tight">
          <Field label="From" group><LangSelect ariaLabel="From" value={f.source_lang || '*'} any onChange={v => setF({ ...f, source_lang: v })} /></Field>
          <Field label="Into" group><LangSelect ariaLabel="Into" value={f.target_lang || ''} onChange={v => setF({ ...f, target_lang: v })} /></Field>
        </div>
        <div className="two-col tight">
          <Field label="Term"><input value={f.term || ''} onChange={e => setF({ ...f, term: e.target.value })} autoFocus placeholder="invoice" /></Field>
          <Field label="Always translated as"><input value={f.translation || ''} onChange={e => setF({ ...f, translation: e.target.value })} placeholder="faktura" /></Field>
        </div>
        <Field label="Note (optional)" help="Why, or when it applies; the AI reads it too."><input value={f.note || ''} onChange={e => setF({ ...f, note: e.target.value })} /></Field>
        <label className="check-row"><Switch checked={!!f.match_case} onChange={v => setF({ ...f, match_case: v })} label="Same capital letters only" /><span><b>Same capital letters only</b><br /><span className="muted small">For names: “Apple” the company, not “apple” the fruit.</span></span></label>
      </div>
    </Modal>
  )
}

function ImportTerms({ onClose, onDone }: { onClose: () => void; onDone: () => void }) {
  const { settings } = useApp()
  const { toast } = useToast()
  const input = useRef<HTMLInputElement>(null)
  const [file, setFile] = useState<File | null>(null)
  const [from, setFrom] = useState('en')
  const [to, setTo] = useState(settings?.target_lang || 'en')
  const [out, setOut] = useState<{ added: number; updated: number; skipped: number } | null>(null)
  const go = async () => {
    if (!file) return
    try {
      const res = await fetch(`${V1}/glossary/import?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`, { method: 'POST', body: await file.arrayBuffer(), credentials: 'same-origin' })
      const j = await res.json()
      if (!res.ok) throw new Error(j.error || res.statusText)
      setOut(j); onDone()
    } catch (e) { toast(errMsg(e), 'err') }
  }
  return (
    <Modal title="Import a glossary" onClose={onClose} footer={out ? <button type="button" className="btn primary" onClick={onClose}>Done</button> : <><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" disabled={!file} onClick={go}>Import</button></>}>
      {out ? <p><b>{out.added}</b> added, {out.updated} updated{out.skipped ? `, ${out.skipped} skipped` : ''}.</p> : (
        <div className="stack">
          <p className="muted small">A CSV or TSV with two columns, <i>term, translation</i> (a DeepL glossary exports like this), or this app’s own export with the languages in it.</p>
          <div className="two-col tight">
            <Field label="From" group><LangSelect ariaLabel="From" value={from} any onChange={setFrom} /></Field>
            <Field label="Into" group><LangSelect ariaLabel="Into" value={to} onChange={setTo} /></Field>
          </div>
          <button type="button" className="btn" onClick={() => input.current?.click()}><Icon name="upload" size={15} />{file ? file.name : 'Choose a file'}</button>
          <input ref={input} type="file" hidden accept=".csv,.tsv,.txt" onChange={e => setFile(e.target.files?.[0] || null)} />
        </div>
      )}
    </Modal>
  )
}
