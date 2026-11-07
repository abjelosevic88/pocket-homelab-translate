// The phrasebook: the translations you starred, in collections, to read
// aloud, learn (Pocket Learn) and take along.
import { useCallback, useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { announceChange, useData, useOnChange } from '../lib/data'
import { canSpeak, useSpeaking } from '../lib/speech'
import { dirOf, langName, langShort, speechTag } from '../lib/translate'
import type { Collection, Translation } from '../lib/types'
import { Icon } from '../components/icons'
import { Empty, Loading, Menu, errMsg, rectOf, useDebounce, useToast, type Anchor } from '../components/ui'
import { AddPhraseModal, EditPhraseModal, sendToLearn, sendToNotes } from '../components/PhraseTools'
import { ImportModal } from '../components/ImportModal'

export default function Phrasebook() {
  const { byCode } = useData()
  const { toast } = useToast()
  const [sp, setSp] = useSearchParams()
  const col = sp.get('c')
  const [q, setQ] = useState(sp.get('q') || '')
  const dq = useDebounce(q, 250)
  const [list, setList] = useState<Translation[] | null>(null)
  const [cols, setCols] = useState<Collection[]>([])
  const [edit, setEdit] = useState<Translation | null>(null)
  const [adding, setAdding] = useState(false)
  const [importing, setImporting] = useState(false)
  const [menu, setMenu] = useState<{ t: Translation; at: Anchor } | null>(null)
  const [speakKey, toggleSpeak] = useSpeaking()
  const [reveal, setReveal] = useState(false)
  const [shown, setShown] = useState<Record<number, boolean>>({})

  const load = useCallback(() => {
    const p = new URLSearchParams({ starred: '1', limit: '500' })
    if (dq) p.set('q', dq)
    if (col !== null) p.set('collection', col)
    api.get<{ translations: Translation[] }>(`${V1}/translations?${p}`).then(r => setList(r.translations)).catch(() => setList([]))
    api.get<{ collections: Collection[] }>(`${V1}/collections`).then(r => setCols(r.collections)).catch(() => {})
  }, [dq, col])
  useEffect(load, [load])
  useOnChange('phrasebook', load)

  const update = (t: Translation) => { setList(l => (l || []).map(x => x.id === t.id ? t : x).filter(x => x.starred)); announceChange('history') }
  const unstar = async (t: Translation) => {
    try { await api.put(`${V1}/translations/${t.id}`, { starred: false }); setList(l => (l || []).filter(x => x.id !== t.id)); toast('Taken out of your phrasebook') } catch (e) { toast(errMsg(e), 'err') }
  }
  const pickCol = (c: string | null) => { const n = new URLSearchParams(sp); if (c === null) n.delete('c'); else n.set('c', c); setSp(n, { replace: true }) }
  const total = cols.reduce((a, c) => a + c.count, 0)

  return (
    <div className="page stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Phrasebook</h1><p className="muted">Phrases you keep: star any translation. Hide the translations to test yourself, or send them to Pocket Learn as flashcards.</p></div>
        <div className="row wrap">
          <button type="button" className="btn" onClick={() => setImporting(true)}><Icon name="upload" size={15} /><span className="hide-sm">Import</span></button>
          <a className="btn" href={`${V1}/export.csv?starred=1`}><Icon name="download" size={15} /><span className="hide-sm">Export</span></a>
          <button type="button" className="btn primary" onClick={() => setAdding(true)}><Icon name="plus" size={15} />Add a phrase</button>
        </div>
      </div>
      <div className="row wrap filters">
        <input className="search-in" value={q} onChange={e => setQ(e.target.value)} placeholder="Search your phrasebook" aria-label="Search" />
        <label className="check-row small"><input type="checkbox" checked={reveal} onChange={e => { setReveal(e.target.checked); setShown({}) }} /> Hide translations (test yourself)</label>
      </div>
      {cols.length > 0 && <div className="chips" role="tablist" aria-label="Collections">
        <button type="button" className={`chip-btn ${col === null ? 'on' : ''}`} onClick={() => pickCol(null)}>All <span className="muted">{total}</span></button>
        {cols.map(c => <button key={c.name || '-'} type="button" className={`chip-btn ${col === c.name ? 'on' : ''}`} onClick={() => pickCol(c.name)}>{c.name || 'No collection'} <span className="muted">{c.count}</span></button>)}
      </div>}
      {!list ? <Loading /> : list.length === 0 ? <Empty icon="star" title={dq || col !== null ? 'Nothing here' : 'Your phrasebook is empty'}>{dq || col !== null ? 'Try another search or collection.' : 'Star a translation (☆) to keep it here, or add a phrase by hand. Saved phrases from Google Translate come in with Import.'}</Empty> : (
        <div className="phrases">
          {list.map(t => {
            const from = t.detected_lang || t.source_lang
            const hidden = reveal && !shown[t.id]
            return (
              <article key={t.id} className="phrase card">
                <div className="phrase-src" dir={dirOf(from, byCode)}>{t.source_text}</div>
                <button type="button" className={`phrase-out ${hidden ? 'hidden' : ''}`} dir={dirOf(t.target_lang, byCode)} onClick={() => reveal && setShown(s => ({ ...s, [t.id]: !s[t.id] }))} disabled={!reveal} aria-label={hidden ? 'Show the translation' : undefined}>
                  {hidden ? 'Tap to see the translation' : t.result_text}
                </button>
                {t.note && <p className="phrase-note muted small">{t.note}</p>}
                <div className="phrase-foot">
                  <span className="pill" title={`${langName(from, byCode)} → ${langName(t.target_lang, byCode)}`}>{langShort(from)} → {langShort(t.target_lang)}</span>
                  {t.collection && <span className="pill">{t.collection}</span>}
                  {t.learn_ref && <span className="pill st-done" title="A flashcard in Pocket Learn"><Icon name="brain" size={11} />Learning</span>}
                  <span className="spacer" />
                  {canSpeak && <button type="button" className={`btn ghost icon-only sm ${speakKey === `p${t.id}` ? 'on' : ''}`} onClick={() => toggleSpeak(`p${t.id}`, t.result_text, speechTag(t.target_lang, byCode))} aria-label="Read aloud" title="Read aloud"><Icon name="volume" size={16} /></button>}
                  <button type="button" className="btn ghost icon-only sm starred" onClick={() => unstar(t)} aria-label="Take out of your phrasebook" title="Take out of your phrasebook"><Icon name="star" size={16} /></button>
                  <button type="button" className="btn ghost icon-only sm" onClick={e => setMenu({ t, at: rectOf(e.currentTarget) })} aria-label="More"><Icon name="more" size={16} /></button>
                </div>
              </article>
            )
          })}
        </div>
      )}
      {menu && <Menu anchor={menu.at} width={250} align="right" onClose={() => setMenu(null)} items={[
        { label: 'Edit', icon: 'edit', onSelect: () => setEdit(menu.t) },
        { label: 'Copy the translation', icon: 'copy', onSelect: async () => { await navigator.clipboard.writeText(menu.t.result_text).catch(() => {}); toast('Copied') } },
        { label: menu.t.learn_ref ? 'Add to Pocket Learn again' : 'Learn it (Pocket Learn)', icon: 'brain', onSelect: async () => { const n = await sendToLearn(menu.t, toast); if (n) update(n) } },
        { label: 'Save as a note (Pocket Notes)', icon: 'note', onSelect: () => sendToNotes(`${V1}/translations/${menu.t.id}/note`, toast) },
        'sep',
        { label: 'Take out of the phrasebook', icon: 'star', onSelect: () => unstar(menu.t) },
        { label: 'Delete', icon: 'trash', danger: true, onSelect: async () => { try { await api.del(`${V1}/translations/${menu.t.id}`); setList(l => (l || []).filter(x => x.id !== menu.t.id)) } catch (e) { toast(errMsg(e), 'err') } } },
      ]} />}
      {edit && <EditPhraseModal t={edit} onClose={() => setEdit(null)} onSaved={update} />}
      {adding && <AddPhraseModal collection={col || ''} onClose={() => setAdding(false)} onSaved={() => { load(); toast('Added to your phrasebook') }} />}
      {importing && <ImportModal onClose={() => setImporting(false)} onDone={load} />}
    </div>
  )
}
