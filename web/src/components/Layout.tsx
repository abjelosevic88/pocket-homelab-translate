// The app shell: sidebar, top bar, search palette, shortcuts — the Pocket Calendar layout.
import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { DataProvider, useData } from '../lib/data'
import { Icon } from './icons'
import { Sidebar } from './Sidebar'
import { CommandPalette, type PaletteCommand } from './CommandPalette'
import { ShortcutsSheet } from './ShortcutsSheet'
import { ImportModal } from './ImportModal'
import { NewConversation } from '../pages/Talk'
import { Avatar, Loading, type MenuItem, MenuButton, modalOpen, useIsMobile, useMediaQuery } from './ui'
import { AskButton } from './AskButton'

export { announceChange, useOnChange } from '../lib/data'

const SIDEBAR_KEY = 'ptl-sidebar'

/** The page's own name for the top bar breadcrumb. */
const CrumbCtx = createContext<(t: string) => void>(() => {})
export function useCrumb(title: string | undefined) {
  const set = useContext(CrumbCtx)
  useEffect(() => { set(title || ''); return () => set('') }, [title, set])
}

export type CreateKind = 'translate' | 'photo' | 'document' | 'talk' | 'import'

/** Start something from anywhere (a page's button). */
export const openCreate = (k: CreateKind) => window.dispatchEvent(new CustomEvent('ptl:create', { detail: { kind: k } }))

/** The new-translation menu: the sidebar's caret. */
export function createItems(onCreate: (k: CreateKind) => void): MenuItem[] {
  return [
    { label: 'New translation', icon: 'languages', hint: 'N', onSelect: () => onCreate('translate') },
    { label: 'Translate a photo', icon: 'camera', hint: 'O', onSelect: () => onCreate('photo') },
    { label: 'Translate a document', icon: 'file', onSelect: () => onCreate('document') },
    { label: 'New conversation', icon: 'message', hint: 'T', onSelect: () => onCreate('talk') },
    'sep',
    { label: 'Bring phrases from Google Translate…', icon: 'upload', onSelect: () => onCreate('import') },
  ]
}

export default function Layout() {
  return <DataProvider><Shell /></DataProvider>
}

function Shell() {
  const navigate = useNavigate()
  const loc = useLocation()
  const { langs } = useData()
  const mobile = useIsMobile()
  const narrow = useMediaQuery('(max-width: 1099px)')
  const drawer = mobile || narrow
  const [sidebarOpen, setSidebarOpen] = useState(() => !drawer && localStorage.getItem(SIDEBAR_KEY) !== '0')
  useEffect(() => { setSidebarOpen(!drawer && localStorage.getItem(SIDEBAR_KEY) !== '0') }, [drawer])
  const drawerOpenedAt = useRef(0)
  const toggleSidebar = () => setSidebarOpen(o => { drawerOpenedAt.current = performance.now(); const n = !o; if (!drawer) localStorage.setItem(SIDEBAR_KEY, n ? '1' : '0'); return n })
  const closeDrawer = () => { if (drawer) setSidebarOpen(false) }
  useEffect(() => { document.querySelector('.content')?.scrollTo({ top: 0 }) }, [loc.pathname])

  const [crumb, setCrumbRaw] = useState('')
  const setCrumb = useCallback((t: string) => setCrumbRaw(t), [])

  const [palette, setPalette] = useState(false)
  const [shortcuts, setShortcuts] = useState(false)
  const [importing, setImporting] = useState(false)
  const [talk, setTalk] = useState(false)

  const create = (k: CreateKind) => {
    closeDrawer()
    switch (k) {
      case 'translate': sessionStorage.removeItem('ptl-draft'); navigate('/', { state: { fresh: Date.now() } }); break
      case 'photo': navigate('/photo'); break
      case 'document': navigate('/documents'); break
      case 'talk': setTalk(true); break
      default: setImporting(true)
    }
  }
  const createRef = useRef(create)
  createRef.current = create

  useEffect(() => {
    const open = () => setShortcuts(true)
    const make = (e: Event) => { const d = (e as CustomEvent<{ kind: CreateKind }>).detail; createRef.current(d.kind) }
    window.addEventListener('ptl:shortcuts', open)
    window.addEventListener('ptl:create', make)
    return () => { window.removeEventListener('ptl:shortcuts', open); window.removeEventListener('ptl:create', make) }
  }, [])
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); setPalette(p => !p); return }
      if (e.metaKey || e.ctrlKey || e.altKey) return
      if (t.closest('input, textarea, select, [contenteditable="true"]') || modalOpen() || document.querySelector('.menu, .popover, .sel-pop')) return
      const go = (to: string) => { e.preventDefault(); navigate(to) }
      const mk = (k: CreateKind) => { e.preventDefault(); createRef.current(k) }
      switch (e.key) {
        case '/': e.preventDefault(); setPalette(true); break
        case '?': e.preventDefault(); setShortcuts(true); break
        case 'n': mk('translate'); break
        case 'o': go('/photo'); break
        case 'd': go('/documents'); break
        case 't': go('/talk'); break
        case 'p': go('/phrasebook'); break
        case 'h': go('/history'); break
        case 'g': go('/glossary'); break
        case ',': go('/settings'); break
        case '[': e.preventDefault(); toggleSidebar(); break
      }
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }) // eslint-disable-line react-hooks/exhaustive-deps

  const commands: PaletteCommand[] = [
    { id: 'new', label: 'New translation', icon: 'languages', hint: 'N', keywords: 'translate text type paste', run: () => create('translate') },
    { id: 'photo', label: 'Translate a photo', icon: 'camera', hint: 'O', keywords: 'picture menu sign camera ocr', run: () => navigate('/photo') },
    { id: 'docs', label: 'Documents', icon: 'file', hint: 'D', keywords: 'word pdf powerpoint docx pptx subtitles srt', run: () => navigate('/documents') },
    { id: 'talk', label: 'Conversations', icon: 'message', hint: 'T', keywords: 'speak talk voice interpreter', run: () => navigate('/talk') },
    { id: 'talk-new', label: 'New conversation', icon: 'mic', keywords: 'speak talk', run: () => create('talk') },
    { id: 'phrasebook', label: 'Phrasebook', icon: 'book', hint: 'P', keywords: 'starred saved phrases collections', run: () => navigate('/phrasebook') },
    { id: 'history', label: 'History', icon: 'clock', hint: 'H', keywords: 'past translations', run: () => navigate('/history') },
    { id: 'glossary', label: 'Glossary', icon: 'book-open', hint: 'G', keywords: 'terms words always', run: () => navigate('/glossary') },
    { id: 'import', label: 'Bring phrases from Google Translate…', icon: 'upload', keywords: 'import csv takeout', run: () => create('import') },
    { id: 'settings', label: 'Settings', icon: 'settings', hint: ',', run: () => navigate('/settings') },
    { id: 'tr-set', label: 'Settings: translating', icon: 'sliders', keywords: 'default language engine tone history photos speech', run: () => navigate('/settings/translating') },
    { id: 'eng-set', label: 'Settings: engines', icon: 'server', keywords: 'offline argos nllb libretranslate tesseract', run: () => navigate('/settings/engines') },
    { id: 'apps', label: 'Settings: connected apps', icon: 'link', keywords: 'voice learn notes files documents core', run: () => navigate('/settings/apps') },
    { id: 'ai-set', label: 'Settings: AI', icon: 'sparkles', keywords: 'pocket ai openai', run: () => navigate('/settings/ai') },
    { id: 'export', label: 'Settings: export everything', icon: 'download', keywords: 'backup json csv', run: () => navigate('/settings/data') },
    { id: 'shortcuts', label: 'Keyboard shortcuts', icon: 'keyboard', hint: '?', run: () => setShortcuts(true) },
  ]

  return (
    <CrumbCtx.Provider value={setCrumb}>
      <div className={`app ${sidebarOpen ? 'with-sidebar' : 'no-sidebar'} ${mobile ? 'is-mobile' : ''} ${drawer ? 'drawer-mode' : ''}`}>
        {drawer && sidebarOpen && <div className="drawer-backdrop" onClick={e => { if (e.timeStamp - drawerOpenedAt.current > 400) setSidebarOpen(false) }} />}
        <div className={`sidebar-wrap ${sidebarOpen ? 'open' : ''}`}>
          <Sidebar onCreate={create} onNavigate={closeDrawer} />
        </div>
        <div className="main">
          <TopBar onMenu={toggleSidebar} onPalette={() => setPalette(true)} mobile={mobile} compact={narrow} sidebarOpen={sidebarOpen} crumb={crumb} />
          <main className={`content ${/^\/talk\/\d/.test(loc.pathname) ? 'full' : ''}`}><div className="content-inner">{langs ? <Outlet /> : <Loading />}</div></main>
        </div>
        {mobile && !/^\/(talk\/\d|settings|photo)/.test(loc.pathname) && <button type="button" className="fab" aria-label="Translate a photo" title="Translate a photo" onClick={() => create('photo')}><Icon name="camera" size={24} strokeWidth={2} /></button>}
        {palette && <CommandPalette onClose={() => setPalette(false)} commands={commands} />}
        {shortcuts && <ShortcutsSheet onClose={() => setShortcuts(false)} />}
        {importing && <ImportModal onClose={() => setImporting(false)} />}
        {talk && <NewConversation onClose={() => setTalk(false)} />}
      </div>
    </CrumbCtx.Provider>
  )
}

const SECTION: Record<string, [string, string]> = { '': ['Translate', '/'], photo: ['Photo', '/photo'], documents: ['Documents', '/documents'], talk: ['Conversation', '/talk'], phrasebook: ['Phrasebook', '/phrasebook'], history: ['History', '/history'], glossary: ['Glossary', '/glossary'] }

function TopBar({ onMenu, onPalette, mobile, compact, sidebarOpen, crumb }: { onMenu: () => void; onPalette: () => void; mobile: boolean; compact: boolean; sidebarOpen: boolean; crumb: string }) {
  const { user, themeMode, setThemeMode, status, setUser } = useApp()
  const navigate = useNavigate()
  const loc = useLocation()
  const [section, sectionTo] = SECTION[loc.pathname.split('/')[1] || ''] || ['', '/']
  const signOut = async () => { await api.post(`${V1}/auth/logout`).catch(() => {}); setUser(null); navigate('/login') }
  const avatarMenu = (): MenuItem[] => [
    { heading: user?.name || user?.email || '' },
    { label: 'Profile', icon: 'user', onSelect: () => navigate('/settings/profile') },
    { label: 'Settings', icon: 'settings', onSelect: () => navigate('/settings'), hint: ',' },
    { label: 'Keyboard shortcuts', icon: 'keyboard', onSelect: () => window.dispatchEvent(new CustomEvent('ptl:shortcuts')), hint: '?' },
    'sep',
    { heading: 'Appearance' },
    { label: 'System', checked: themeMode === 'system', onSelect: () => setThemeMode('system') },
    { label: 'Light', checked: themeMode === 'light', onSelect: () => setThemeMode('light') },
    { label: 'Dark', checked: themeMode === 'dark', onSelect: () => setThemeMode('dark') },
    ...(status?.auth_mode === 'local' ? ['sep' as const, { label: 'Sign out', icon: 'log-out', onSelect: signOut }] : []),
  ]
  return (
    <header className="topbar">
      <button type="button" className="btn ghost icon-only" onClick={onMenu} aria-label={sidebarOpen ? 'Hide the sidebar' : 'Show the sidebar'} title="Menu ([)"><Icon name={mobile || compact ? 'menu' : 'sidebar'} size={18} /></button>
      <h1 className="range-title">
        {crumb ? <><button type="button" className="crumb" onClick={() => navigate(sectionTo)}>{section}</button><Icon name="chevron-right" size={16} className="crumb-sep" /><span className="ellipsis">{crumb}</span></> : section}
      </h1>
      <span className="spacer" />
      <button type="button" className="btn ghost icon-only" onClick={onPalette} aria-label="Search" title="Search (⌘K or /)"><Icon name="search" size={18} /></button>
      <AskButton />
      <MenuButton label="Account" className="avatar-btn" align="right" width={220} items={avatarMenu}>
        <Avatar name={user?.name} email={user?.email} size={30} />
      </MenuButton>
    </header>
  )
}
