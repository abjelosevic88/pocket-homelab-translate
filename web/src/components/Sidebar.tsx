// Left column: a new translation (and the other ways to start), the
// sections, then the phrasebook's collections and the engines' state.
// The Pocket Calendar sidebar.
import { useEffect, useState } from 'react'
import { Link, NavLink, useLocation, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { useData, useOnChange } from '../lib/data'
import type { Collection } from '../lib/types'
import { Icon } from './icons'
import { createItems, type CreateKind } from './Layout'
import { type Anchor, Menu, rectOf } from './ui'

export function Sidebar({ onCreate, onNavigate }: { onCreate: (k: CreateKind) => void; onNavigate: () => void }) {
  const { status, settings } = useApp()
  const { engines } = useData()
  const navigate = useNavigate()
  const loc = useLocation()
  const [menu, setMenu] = useState<Anchor | null>(null)
  const [cols, setCols] = useState<Collection[]>([])
  const loadCols = () => { api.get<{ collections: Collection[] }>(`${V1}/collections`).then(r => setCols(r.collections)).catch(() => {}) }
  useEffect(loadCols, [])
  useOnChange('phrasebook', loadCols)
  const title = settings?.title || status?.title || 'Pocket Translate'
  const curCol = loc.pathname === '/phrasebook' ? new URLSearchParams(loc.search).get('c') : null
  const nav: { to: string; label: string; icon: string; active: boolean; dim?: boolean; hint?: string }[] = [
    { to: '/', label: 'Translate', icon: 'languages', active: loc.pathname === '/' },
    { to: '/photo', label: 'Photo', icon: 'camera', active: loc.pathname === '/photo', dim: engines ? !engines.ocr.ready : false, hint: engines && !engines.ocr.ready ? 'Needs tesseract on the server' : undefined },
    { to: '/documents', label: 'Documents', icon: 'file', active: loc.pathname === '/documents' },
    { to: '/talk', label: 'Conversation', icon: 'message', active: loc.pathname.startsWith('/talk') },
    { to: '/phrasebook', label: 'Phrasebook', icon: 'book', active: loc.pathname === '/phrasebook' && curCol === null },
    { to: '/history', label: 'History', icon: 'clock', active: loc.pathname === '/history' },
    { to: '/glossary', label: 'Glossary', icon: 'book-open', active: loc.pathname === '/glossary' },
  ]
  const offline = engines ? (engines.argos.ready || engines.nllb.ready) : true
  return (
    <aside className="sidebar" aria-label="Pocket Translate">
      <div className="sb-brand">
        <img src="/favicon.svg" alt="" className="sb-logo" />
        <span className="sb-title">{title !== 'Pocket Translate' ? title : <>Pocket <b>Translate</b></>}</span>
        {status?.version && <span className="sb-version" title={`Version ${status.version}`}>{/^v?\d+\.\d+/.test(status.version) ? 'v' + status.version.replace(/^v/, '').replace(/[-+].*$/, '') : status.version}</span>}
      </div>
      <div className="create-split">
        <button type="button" className="btn primary create-btn" onClick={() => onCreate('translate')} title="New translation (N)">
          <Icon name="plus" size={15} strokeWidth={2.2} />New translation
        </button>
        <button type="button" className="btn primary create-more" aria-label="More ways to start" aria-haspopup="menu" aria-expanded={!!menu} onClick={e => setMenu(menu ? null : rectOf(e.currentTarget))}>
          <Icon name="chevron-down" size={15} />
        </button>
      </div>
      {menu && <Menu anchor={menu} items={createItems(onCreate)} onClose={() => setMenu(null)} width={290} />}

      <nav className="sb-nav" aria-label="Sections">
        {nav.map(n => (
          <NavLink key={n.to} to={n.to} onClick={onNavigate} title={n.hint} className={() => `sb-nav-item ${n.active ? 'active' : ''} ${n.dim ? 'dim' : ''}`}>
            <Icon name={n.icon} size={16} />{n.label}
          </NavLink>
        ))}
      </nav>

      {cols.filter(c => c.name).length > 0 && <section className="sb-sec">
        <h4 className="sb-h">Collections</h4>
        {cols.filter(c => c.name).map(c => (
          <button key={c.name} type="button" className={`sb-row sb-list ${curCol === c.name ? 'active' : ''}`} onClick={() => { onNavigate(); navigate(`/phrasebook?c=${encodeURIComponent(c.name)}`) }}>
            <span className="sb-list-ico" aria-hidden="true"><Icon name="star" size={13} /></span>
            <span className="grow ellipsis">{c.name}</span>
            <span className="muted small tnum">{c.count}</span>
          </button>
        ))}
      </section>}

      {engines && <section className="sb-sec">
        <h4 className="sb-h">Engines</h4>
        <Link to="/settings/engines" className="sb-row sb-list" onClick={onNavigate} title={engines.ai.ready ? 'Pocket AI translates with tone and context' : engines.ai.detail}>
          <span className={`sb-dot ${engines.ai.ready ? 'ok' : ''}`} aria-hidden="true" /><span className="grow">AI</span><span className="muted small">{engines.ai.ready ? 'ready' : 'off'}</span>
        </Link>
        <Link to="/settings/engines" className="sb-row sb-list" onClick={onNavigate} title="Argos and NLLB, on this server">
          <span className={`sb-dot ${offline ? 'ok' : ''}`} aria-hidden="true" /><span className="grow">Offline</span><span className="muted small">{engines.argos.ready && engines.nllb.ready ? 'Argos + NLLB' : engines.nllb.ready ? 'NLLB' : engines.argos.ready ? 'Argos' : 'off'}</span>
        </Link>
      </section>}

      <div className="sb-foot">
        <Link to="/settings" className="sb-foot-link" onClick={onNavigate}><Icon name="settings" size={15} />Settings</Link>
        <button type="button" className="sb-foot-link" onClick={() => { onNavigate(); window.dispatchEvent(new CustomEvent('ptl:shortcuts')) }}><Icon name="keyboard" size={15} />Shortcuts</button>
      </div>
    </aside>
  )
}
