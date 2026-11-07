// Everything translated, newest first, by day: search it, star it, open it
// in the translator again, or clear it.
import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, useData, useOnChange } from '../lib/data'
import { fmtDate, fmtISOTime } from '../lib/format'
import { KIND_WORDS, engineLabel, langName, langShort } from '../lib/translate'
import type { Translation } from '../lib/types'
import { Icon } from '../components/icons'
import { Confirm, Empty, Loading, Menu, errMsg, rectOf, useDebounce, useToast, type Anchor } from '../components/ui'
import { Select } from '../components/Select'
import { EditPhraseModal, sendToLearn, sendToNotes } from '../components/PhraseTools'

const PAGE = 60

export default function History() {
  const { settings } = useApp()
  const { byCode } = useData()
  const { toast } = useToast()
  const navigate = useNavigate()
  const [sp] = useSearchParams()
  const [q, setQ] = useState(sp.get('q') || '')
  const dq = useDebounce(q, 250)
  const [kind, setKind] = useState('')
  const [lang, setLang] = useState('')
  const [list, setList] = useState<Translation[] | null>(null)
  const [total, setTotal] = useState(0)
  const [clear, setClear] = useState(false)
  const [menu, setMenu] = useState<{ t: Translation; at: Anchor } | null>(null)
  const [edit, setEdit] = useState<Translation | null>(null)

  const load = useCallback((more = false) => {
    const p = new URLSearchParams({ limit: String(PAGE), offset: String(more ? list?.length || 0 : 0) })
    if (dq) p.set('q', dq)
    if (kind) p.set('kind', kind)
    if (lang) p.set('lang', lang)
    api.get<{ translations: Translation[]; total: number }>(`${V1}/translations?${p}`).then(r => { setList(l => more ? [...(l || []), ...r.translations] : r.translations); setTotal(r.total) }).catch(() => setList([]))
  }, [dq, kind, lang]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { load() }, [load])
  const reload = useCallback(() => load(), [load])
  useOnChange('history', reload)

  const star = async (t: Translation) => {
    try { const n = await api.put<Translation>(`${V1}/translations/${t.id}`, { starred: !t.starred }); setList(l => (l || []).map(x => x.id === t.id ? n : x)); announceChange('phrasebook') } catch (e) { toast(errMsg(e), 'err') }
  }
  const remove = async (t: Translation) => {
    try { await api.del(`${V1}/translations/${t.id}`); setList(l => (l || []).filter(x => x.id !== t.id)); setTotal(n => n - 1) } catch (e) { toast(errMsg(e), 'err') }
  }
  const open = (t: Translation) => { sessionStorage.setItem('ptl-draft', t.source_text); navigate('/') }

  const days: [string, Translation[]][] = []
  for (const t of list || []) {
    const d = new Date(t.created_at)
    const key = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
    const last = days[days.length - 1]
    if (last && last[0] === key) last[1].push(t); else days.push([key, [t]])
  }
  const langsUsed = Array.from(new Set([...(settings?.recent_langs || [])]))
  return (
    <div className="page stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">History</h1><p className="muted">{settings?.keep_history === false ? 'History is switched off (Settings → Translating); only starred phrases are kept.' : settings?.history_days ? `Kept for ${settings.history_days} days; starred phrases stay.` : 'Everything you translated. Starred phrases are your phrasebook.'}</p></div>
        <button type="button" className="btn" onClick={() => setClear(true)} disabled={!total}><Icon name="trash" size={15} />Clear history</button>
      </div>
      <div className="row wrap filters">
        <input className="search-in" value={q} onChange={e => setQ(e.target.value)} placeholder="Search what you translated" aria-label="Search" />
        <Select ariaLabel="Kind" value={kind} onChange={setKind} options={[{ value: '', label: 'Everything' }, ...Object.entries(KIND_WORDS).map(([k, v]) => ({ value: k, label: v.label }))]} />
        <Select ariaLabel="Language" value={lang} onChange={setLang} options={[{ value: '', label: 'Every language' }, ...langsUsed.map(c => ({ value: c, label: langName(c, byCode) }))]} />
      </div>
      {!list ? <Loading /> : list.length === 0 ? <Empty icon="clock" title={dq || kind || lang ? 'Nothing found' : 'Nothing translated yet'}>{dq || kind || lang ? 'Try other words or filters.' : 'What you translate shows up here.'}</Empty> : (
        <>
          {days.map(([day, items]) => (
            <section key={day} className="day-sec">
              <h3 className="day-h">{fmtDate(day)}</h3>
              <ul className="list card">
                {items.map(t => {
                  const k = KIND_WORDS[t.kind] || KIND_WORDS.text
                  return (
                    <li key={t.id} className="list-row hist-row">
                      <span className="list-ico" title={k.label}><Icon name={k.icon} size={15} /></span>
                      <button type="button" className="grow min0 row-main as-btn" onClick={() => open(t)} title="Open in the translator">
                        <span className="row-title ellipsis">{t.source_text}</span>
                        <span className="small ellipsis block-line hist-out">{t.result_text}</span>
                        <span className="muted tiny">{langShort(t.detected_lang || t.source_lang)} → {langShort(t.target_lang)} · {fmtISOTime(t.created_at)} · {engineLabel(t.engine, t.model)}{t.source && ` · from Pocket ${t.source[0].toUpperCase()}${t.source.slice(1)}`}</span>
                      </button>
                      <button type="button" className={`btn ghost icon-only ${t.starred ? 'starred' : ''}`} onClick={() => star(t)} aria-label={t.starred ? 'Take out of your phrasebook' : 'Save to your phrasebook'} aria-pressed={t.starred}><Icon name="star" size={16} /></button>
                      <button type="button" className="btn ghost icon-only" onClick={e => setMenu({ t, at: rectOf(e.currentTarget) })} aria-label="More"><Icon name="more" size={16} /></button>
                    </li>
                  )
                })}
              </ul>
            </section>
          ))}
          {list.length < total && <button type="button" className="btn self-start" onClick={() => load(true)}>Show more ({total - list.length})</button>}
        </>
      )}
      {menu && <Menu anchor={menu.at} width={240} align="right" onClose={() => setMenu(null)} items={[
        { label: 'Open in the translator', icon: 'languages', onSelect: () => open(menu.t) },
        { label: 'Copy the translation', icon: 'copy', onSelect: async () => { await navigator.clipboard.writeText(menu.t.result_text).catch(() => {}); toast('Copied') } },
        { label: 'Edit', icon: 'edit', onSelect: () => setEdit(menu.t) },
        { label: 'Learn it (Pocket Learn)', icon: 'brain', onSelect: async () => { const n = await sendToLearn(menu.t, toast); if (n) setList(l => (l || []).map(x => x.id === n.id ? n : x)) } },
        { label: 'Save as a note (Pocket Notes)', icon: 'note', onSelect: () => sendToNotes(`${V1}/translations/${menu.t.id}/note`, toast) },
        'sep',
        { label: 'Delete', icon: 'trash', danger: true, onSelect: () => remove(menu.t) },
      ]} />}
      {edit && <EditPhraseModal t={edit} onClose={() => setEdit(null)} onSaved={n => setList(l => (l || []).map(x => x.id === n.id ? n : x))} />}
      {clear && <Confirm title="Clear your history?" message="Everything you translated is removed, except the phrases in your phrasebook (starred)." confirmLabel="Clear history"
        onCancel={() => setClear(false)} onConfirm={async () => { setClear(false); try { const r = await api.del<{ removed: number }>(`${V1}/translations`); toast(`${r.removed} removed`); load() } catch (e) { toast(errMsg(e), 'err') } }} />}
    </div>
  )
}
