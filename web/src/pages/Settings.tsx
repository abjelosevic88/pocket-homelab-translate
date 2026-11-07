import { AskButton } from '../components/AskButton'
import React, { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { APIToken, AppSlug, AppState, ConnMode, Engines, Lang, Settings as SettingsT, User } from '../lib/types'
import { DATE_FORMATS, TIME_FORMATS, fmtBytes, fmtDateTime, type DateFormat, type TimeFormat } from '../lib/format'
import { Avatar, Confirm, CoreHint, Field, Loading, Modal, Segmented, SettingRow, Switch, Tabs, useAsync, useIsMobile, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { Select } from '../components/Select'
import { ShortcutsSheet } from '../components/ShortcutsSheet'
import { PocketModelSelect } from '../components/PocketModelSelect'
import { ImportModal } from '../components/ImportModal'
import { LangSelect } from '../components/LangSelect'
import { DataProvider } from '../lib/data'
import { APP_INFO, STATE_WORDS } from '../lib/translate'

type TabId = 'general' | 'translating' | 'engines' | 'notifications' | 'apps' | 'ai' | 'profile' | 'people' | 'tokens' | 'data' | 'about'

const TABS: { id: TabId; label: string; icon: string; sub: string }[] = [
  { id: 'general', label: 'General', icon: 'home', sub: 'Dates, times and the app title.' },
  { id: 'translating', label: 'Translating', icon: 'languages', sub: 'Your languages, the engine and tone, the history, photos and speech.' },
  { id: 'engines', label: 'Engines', icon: 'server', sub: 'What translates: your AI, and the offline engines on this server (Argos and NLLB), with reading photos and PDFs.' },
  { id: 'notifications', label: 'Notifications', icon: 'bell', sub: 'A message when a long document is translated.' },
  { id: 'apps', label: 'Connected apps', icon: 'link', sub: 'Pocket Voice for speech, Learn for flashcards, Notes, Files and Documents.' },
  { id: 'ai', label: 'AI', icon: 'sparkles', sub: 'Who translates with tone and context: Pocket AI through Core, or your own OpenAI key.' },
  { id: 'profile', label: 'Profile', icon: 'user', sub: 'Your name, email and password.' },
  { id: 'people', label: 'People', icon: 'users', sub: 'Everyone who has an account on this box. Each person has their own history, phrasebook and glossary.' },
  { id: 'tokens', label: 'API & shortcuts', icon: 'key', sub: 'Personal tokens for Siri Shortcuts, scripts and tools made for LibreTranslate.' },
  { id: 'data', label: 'Backup & data', icon: 'database', sub: 'Bring phrases from Google Translate, export everything, download the database.' },
  { id: 'about', label: 'About', icon: 'info', sub: 'Version, numbers and the server.' },
]

export default function Settings() {
  const { tab: raw } = useParams()
  const navigate = useNavigate()
  const mobile = useIsMobile()
  const tab: TabId = TABS.find(t => t.id === raw)?.id || 'general'
  const current = TABS.find(t => t.id === tab)!
  const go = (id: TabId) => navigate(`/settings/${id}`, { replace: true })
  useEffect(() => { document.querySelector('.set-content')?.scrollTo({ top: 0 }) }, [tab])
  useEffect(() => {
    const h = (e: KeyboardEvent) => { if (e.key === 'Escape' && !document.querySelector('.modal-backdrop, .menu, .sel-pop')) navigate('/') }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [navigate])
  return (
    <DataProvider><div className="set-page">
      <header className="set-header">
        <button type="button" className="btn ghost icon-only" aria-label="Back" title="Back (esc)" onClick={() => navigate('/')}><Icon name="chevron-left" size={18} /></button>
        <img src="/favicon.svg" alt="" width={24} height={24} className="set-logo" />
        <h1>Settings</h1>
        <span style={{ flex: 1 }} />
        <AskButton />
      </header>
      {mobile && <div className="set-tabs-mobile"><Tabs tabs={TABS.map(t => ({ id: t.id, label: t.label, icon: t.icon }))} value={tab} onChange={go} /></div>}
      <div className="set-body">
        {!mobile && (
          <nav className="set-nav" aria-label="Settings sections">
            {TABS.map(t => (
              <button key={t.id} type="button" className={`set-nav-item ${t.id === tab ? 'active' : ''}`} aria-current={t.id === tab ? 'page' : undefined} onClick={() => go(t.id)}>
                <Icon name={t.icon} size={16} />{t.label}
              </button>
            ))}
          </nav>
        )}
        <main className="set-content">
          <div className="set-inner" key={tab}>
            <div className="set-title"><h2>{current.label}</h2><p>{current.sub}</p></div>
            {tab === 'general' && <General />}
            {tab === 'translating' && <TranslatingTab />}
            {tab === 'engines' && <EnginesTab />}
            {tab === 'notifications' && <Notifications />}
            {tab === 'apps' && <Integrations />}
            {tab === 'ai' && <AITab />}
            {tab === 'profile' && <Profile />}
            {tab === 'people' && <Users />}
            {tab === 'tokens' && <Tokens />}
            {tab === 'data' && <Data />}
            {tab === 'about' && <About />}
          </div>
        </main>
      </div>
    </div></DataProvider>
  )
}

function Section({ title, sub, icon, actions, children }: { title: React.ReactNode; sub?: React.ReactNode; icon?: string; actions?: React.ReactNode; children?: React.ReactNode }) {
  return (
    <section className="set-section">
      <div className="set-section-head">
        {icon && <span className="set-app-ico"><Icon name={icon} size={17} /></span>}
        <div className="grow"><h3>{title}</h3>{sub && <p>{sub}</p>}</div>
        {actions && <div className="row">{actions}</div>}
      </div>
      {children && <div className="set-section-body">{children}</div>}
    </section>
  )
}

function useSettingsForm() {
  const { settings, setSettings, user } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState<SettingsT | null>(settings)
  const [busy, setBusy] = useState(false)
  useEffect(() => { if (!f && settings) setF(settings) }, [settings]) // eslint-disable-line react-hooks/exhaustive-deps
  const save = async (patch?: Partial<SettingsT>) => {
    if (!f) return
    setBusy(true)
    try { const s = await api.put<SettingsT>(`${V1}/settings`, { ...f, ...patch }); setSettings(s); setF(s); toast('Saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return { f, setF, save, busy, isAdmin: user?.role === 'admin' }
}

function useTester() {
  const [test, setTest] = useState<Record<string, string>>({})
  const run = async (which: string, url: string, key: string, extra: Record<string, string> = {}) => {
    setTest(t => ({ ...t, [which]: 'testing…' }))
    try { const r = await api.post<{ ok: boolean; message?: string; error?: string }>(`${V1}/integrations/test`, { which, url, key, ...extra }); setTest(t => ({ ...t, [which]: r.ok ? `✓ ${r.message}` : `✕ ${r.error}` })) } catch (e) { setTest(t => ({ ...t, [which]: `✕ ${(e as Error).message}` })) }
  }
  const status = (which: string) => <span className={`small ${test[which]?.startsWith('✓') ? 'ok' : test[which]?.startsWith('✕') ? 'bad' : 'muted'}`}>{test[which]}</span>
  return { run, status }
}

function General() {
  const { f, setF, save, busy, isAdmin } = useSettingsForm()
  if (!f) return <Loading />
  return (
    <div className="stack loose">
      <Section title="Dates" icon="calendar">
        <SettingRow title="Week starts on"><Segmented size="sm" value={f.week_start} onChange={v => setF({ ...f, week_start: v })} options={[{ value: 1, label: 'Monday' }, { value: 6, label: 'Saturday' }, { value: 7, label: 'Sunday' }]} /></SettingRow>
        <SettingRow title="Date format"><Select ariaLabel="Date format" value={f.date_format} onChange={v => setF({ ...f, date_format: v as DateFormat })} options={DATE_FORMATS.map(d => ({ value: d.id, label: d.label }))} /></SettingRow>
        <SettingRow title="Time format"><Select ariaLabel="Time format" value={f.time_format} onChange={v => setF({ ...f, time_format: v as TimeFormat })} options={TIME_FORMATS.map(d => ({ value: d.id, label: d.label }))} /></SettingRow>
        {isAdmin && <SettingRow title="App title" help="Shown in the sidebar for everyone on this box."><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} /></SettingRow>}
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button></div>
    </div>
  )
}




function TranslatingTab() {
  const { f, setF, save, busy } = useSettingsForm()
  const { data: eng } = useAsync(() => api.get<Engines>(`${V1}/engines`), [])
  const { data: langs } = useAsync(() => api.get<{ languages: Lang[] }>(`${V1}/languages`).then(r => r.languages), [])
  if (!f) return <Loading />
  const ocrHave = eng?.ocr.languages || []
  const ocrLangs = Array.from(new Map((langs || []).filter(l => l.ocr && ocrHave.includes(l.ocr)).map(l => [l.ocr, l.name.replace(/ \((Latin|Cyrillic)\)$/, m => m)])).entries())
  const toggleOCR = (c: string) => setF({ ...f, ocr_langs: f.ocr_langs.includes(c) ? f.ocr_langs.filter(x => x !== c) : [...f.ocr_langs, c] })
  return (
    <div className="stack loose">
      <Section title="Your languages" icon="languages">
        <SettingRow title="Translate from" help="Detect finds the language by itself."><LangSelect ariaLabel="From" value={f.source_lang} auto onChange={v => setF({ ...f, source_lang: v })} /></SettingRow>
        <SettingRow title="Translate into" help="Serbian comes in Latin or Cyrillic: pick the one you read."><LangSelect ariaLabel="Into" value={f.target_lang} onChange={v => setF({ ...f, target_lang: v })} /></SettingRow>
      </Section>
      <Section title="How" icon="sliders">
        <SettingRow title="Engine" help="Auto uses the AI when there is one and the offline engines otherwise (or when the AI does not answer)."><Segmented size="sm" value={f.engine} onChange={v => setF({ ...f, engine: v })} options={[{ value: 'auto', label: 'Auto' }, { value: 'ai', label: 'AI' }, { value: 'offline', label: 'Offline' }]} /></SettingRow>
        <SettingRow title="Tone" help="Formal or casual “you” (Sie/du, Vi/ti). Only the AI can do this."><Segmented size="sm" value={f.tone} onChange={v => setF({ ...f, tone: v })} options={[{ value: '', label: 'Any' }, { value: 'formal', label: 'Formal' }, { value: 'casual', label: 'Casual' }]} /></SettingRow>
        <SettingRow title="Translate as I type" help="With the offline engines (quick and free). The AI translates when you press Translate."><Switch checked={f.live_translate} onChange={v => setF({ ...f, live_translate: v })} label="Translate as I type" /></SettingRow>
      </Section>
      <Section title="History" icon="clock">
        <SettingRow title="Keep a history" help="What you translate is kept so you can find it again. Off: only starred phrases are kept."><Switch checked={f.keep_history} onChange={v => setF({ ...f, keep_history: v })} label="Keep a history" /></SettingRow>
        <SettingRow title="Keep it for" help="Older translations go; the phrasebook stays."><Select ariaLabel="Keep history for" value={String(f.history_days)} onChange={v => setF({ ...f, history_days: Number(v) })} options={[{ value: '0', label: 'Always' }, { value: '7', label: 'A week' }, { value: '30', label: 'A month' }, { value: '90', label: 'Three months' }, { value: '365', label: 'A year' }]} /></SettingRow>
        <SettingRow title="What other apps translate" help="When Pocket Chat or Mail translates through this app, keep it in your history too."><Switch checked={f.keep_app_history} onChange={v => setF({ ...f, keep_app_history: v })} label="Keep what other apps translate" /></SettingRow>
      </Section>
      <Section title="Photos" icon="camera" sub="The languages a photo is read in when you do not say which. Fewer languages read faster and better.">
        {!eng ? <Loading /> : !eng.ocr.ready ? <p className="muted small">Reading photos needs tesseract on this server (it is in the Docker image).</p> : (
          <div className="chips">{ocrLangs.map(([c, name]) => <button key={c} type="button" className={`chip-btn ${f.ocr_langs.includes(c) ? 'on' : ''}`} onClick={() => toggleOCR(c)} aria-pressed={f.ocr_langs.includes(c)}>{name}</button>)}</div>
        )}
      </Section>
      <Section title="Speech" icon="mic" sub="What you say (dictation, conversations) is written out on the server, then translated. Reading aloud uses your device’s own voices.">
        <SettingRow title="Written out by" help="Pocket Voice runs whisper on this server; nothing leaves the box."><Segmented size="sm" value={f.speech} onChange={v => setF({ ...f, speech: v })} options={[{ value: 'voice', label: 'Pocket Voice' }, { value: 'openai', label: 'OpenAI key' }]} /></SettingRow>
        {f.speech === 'openai' && <SettingRow title="Speech model"><input value={f.speech_model} onChange={e => setF({ ...f, speech_model: e.target.value })} placeholder="whisper-1" /></SettingRow>}
        <SettingRow title="Read conversations aloud" help="Each translation is spoken for the other person."><Switch checked={f.auto_speak} onChange={v => setF({ ...f, auto_speak: v })} label="Read aloud" /></SettingRow>
        {eng && <p className={`small ${eng.speech.ready ? 'ok' : 'muted'}`}><Icon name={eng.speech.ready ? 'check-circle' : 'info'} size={13} /> {eng.speech.ready ? 'Speech works.' : eng.speech.detail}</p>}
      </Section>
      <Section title="Into your other apps" icon="link">
        <SettingRow title="Pocket Learn deck" help="Phrases become flashcards here."><input value={f.learn_deck} onChange={e => setF({ ...f, learn_deck: e.target.value })} placeholder="Phrasebook" /></SettingRow>
        <SettingRow title="Pocket Notes folder"><input value={f.notes_folder} onChange={e => setF({ ...f, notes_folder: e.target.value })} placeholder="Translations" /></SettingRow>
        <SettingRow title="Pocket Files folder" help="Translated documents go here (next to the original when it came from Files)."><input value={f.files_folder} onChange={e => setF({ ...f, files_folder: e.target.value })} placeholder="Translations" /></SettingRow>
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button></div>
    </div>
  )
}

function EnginesTab() {
  const { f, setF, save, busy, isAdmin } = useSettingsForm()
  const { run, status } = useTester()
  const { data: eng, reload } = useAsync(() => api.get<Engines>(`${V1}/engines`), [])
  if (!f) return <Loading />
  const state = (ok: boolean | undefined, detail?: string, n?: number) => ok ? <span className="conn-status ok"><span className="dot" />Ready{n ? ` · ${n} languages` : ''}</span> : <span className="conn-status bad"><span className="dot" />{detail || 'Not ready'}</span>
  return (
    <div className="stack loose">
      <Section title="AI" icon="sparkles" sub="Translates with context and tone, offers other ways to say it and explains phrases. Set it up under AI.">
        {!eng ? <Loading /> : state(eng.ai.ready, eng.ai.detail || 'No AI is set up')}
        {eng?.ai.ready && <p className="muted small">{eng.ai.provider === 'openai' ? 'Your own OpenAI key' : 'Pocket AI through Pocket Core'}.</p>}
      </Section>
      <Section title="Argos (LibreTranslate)" icon="server" sub="Offline, on this server: quick, about fifty languages, everything through English. Nothing leaves the box.">
        {!eng ? <Loading /> : state(eng.argos.ready, eng.argos.configured ? eng.argos.detail : 'Not set up', eng.argos.languages)}
        {isAdmin && <div className="two-col tight">
          <Field label="Address" help="A LibreTranslate server, e.g. http://libretranslate:5000"><input value={f.argos_url || ''} onChange={e => setF({ ...f, argos_url: e.target.value })} placeholder="http://libretranslate:5000" /></Field>
          <Field label="API key (if it asks for one)"><input value={f.argos_key || ''} onChange={e => setF({ ...f, argos_key: e.target.value })} autoComplete="off" /></Field>
        </div>}
        {isAdmin && <div className="row"><button type="button" className="btn sm" onClick={() => run('argos', f.argos_url || '', f.argos_key || '')}>Test</button>{status('argos')}</div>}
      </Section>
      <Section title="NLLB-200" icon="server" sub="Offline, on this server: two hundred languages, Serbian, Croatian and Bosnian among them, translated directly. Slower than Argos; used for what Argos does not know.">
        {!eng ? <Loading /> : state(eng.nllb.ready, eng.nllb.configured ? eng.nllb.detail : 'Not set up', eng.nllb.languages)}
        {isAdmin && <div className="two-col tight">
          <Field label="Address" help="Pocket Translate's NLLB engine (engines/nllb), e.g. http://nllb:5000"><input value={f.nllb_url || ''} onChange={e => setF({ ...f, nllb_url: e.target.value })} placeholder="http://nllb:5000" /></Field>
          <Field label="API key (if set)"><input value={f.nllb_key || ''} onChange={e => setF({ ...f, nllb_key: e.target.value })} autoComplete="off" /></Field>
        </div>}
        {isAdmin && <div className="row"><button type="button" className="btn sm" onClick={() => run('nllb', f.nllb_url || '', f.nllb_key || '')}>Test</button>{status('nllb')}</div>}
      </Section>
      <Section title="Reading photos and PDFs" icon="camera">
        {!eng ? <Loading /> : <>
          <SettingRow title="Photos (tesseract)">{state(eng.ocr.ready, 'tesseract is not on this server', eng.ocr.languages.length)}</SettingRow>
          <SettingRow title="PDFs (pdftotext)">{state(eng.pdf.ready, 'pdftotext is not on this server')}</SettingRow>
        </>}
      </Section>
      {!isAdmin && <p className="muted small">The offline engines are shared by everyone on this box; an admin sets their addresses.</p>}
      {isAdmin && <div className="row"><button type="button" className="btn primary" onClick={async () => { await save(); reload() }} disabled={busy}>Save changes</button></div>}
    </div>
  )
}

function Notifications() {
  const { f, setF, save, busy } = useSettingsForm()
  const { run, status } = useTester()
  if (!f) return <Loading />
  return (
    <div className="stack loose">
      <Section title="Push" icon="bell" actions={<span className={`badge ${f.notify_configured ? 'ok' : ''}`}>{f.notify_configured ? 'on' : 'not set'}</span>}
        sub={<>Where messages go: an Apprise gateway (<code>…/notify/&lt;key&gt;</code>), an ntfy topic URL, or any webhook that takes JSON.</>}>
        <SettingRow title="URL" stack><input value={f.notify_url} onChange={e => setF({ ...f, notify_url: e.target.value })} placeholder="http://apprise:8000/notify/translate" /></SettingRow>
        <SettingRow title="Format"><Segmented value={f.notify_format} onChange={v => setF({ ...f, notify_format: v })} options={[{ value: 'apprise', label: 'Apprise' }, { value: 'ntfy', label: 'ntfy' }, { value: 'json', label: 'JSON' }]} size="sm" /></SettingRow>
      </Section>
      <Section title="What to tell you" icon="clock">
        <SettingRow title="A long document is translated" help="Only for documents that took more than two minutes; short ones are ready before you look away."><Switch checked={f.notify_docs} onChange={v => setF({ ...f, notify_docs: v })} label="Document translated" /></SettingRow>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button>
        <button type="button" className="btn" onClick={() => run('notify', f.notify_url, '', { format: f.notify_format })} disabled={!f.notify_url}><Icon name="mail" size={15} />Send a test</button>
        {status('notify')}
      </div>
    </div>
  )
}

const APPS: { slug: AppSlug; title: string; sub: string; token: string; port: string }[] = [
  { slug: 'voice', title: 'Pocket Voice — speech', sub: 'What you say in a conversation or dictate is written out by whisper on this server. Without it, speech needs your own OpenAI key (Settings → Translating → Speech).', token: 'Pocket Voice → Settings → API tokens', port: '8237' },
  { slug: 'learn', title: 'Pocket Learn — flashcards', sub: 'Phrases from your phrasebook become flashcards, so you learn them.', token: 'Pocket Learn → Settings → API tokens', port: '8218' },
  { slug: 'notes', title: 'Pocket Notes — as notes', sub: 'Keep a translation or a whole conversation as a note.', token: 'Pocket Notes → Settings → API tokens', port: '8182' },
  { slug: 'files', title: 'Pocket Files — documents', sub: 'Pick a document from your files, and put the translated copy next to it.', token: 'Pocket Files → Settings → API tokens', port: '8187' },
  { slug: 'documents', title: 'Pocket Documents — your archive', sub: 'Pick a document from your archive, and file the translated copy there.', token: 'Pocket Documents → Settings → API tokens', port: '8183' },
]

function Integrations() {
  const { f, setF, save, busy } = useSettingsForm()
  const { run, status } = useTester()
  const { data: states } = useAsync(() => api.get<{ apps: AppState[] }>(`${V1}/apps`).then(r => r.apps).catch(() => [] as AppState[]), [f?.voice_mode, f?.learn_mode, f?.notes_mode, f?.files_mode, f?.documents_mode])
  if (!f) return <Loading />
  const coreOn = f.core_configured && f.core_registered !== false
  const eff = (m: ConnMode): ConnMode => !coreOn && m === 'core' ? 'off' : m
  const set = (k: string, v: string) => setF({ ...f, [k]: v } as SettingsT)
  return (
    <div className="stack loose">
      <div className="callout info"><Icon name="info" size={15} /><div>These connections are yours. Translate only adds things in them when you ask, and translating works without them. Through <b>Pocket Core</b> Translate acts as you in your own Pocket apps with nothing to paste (switch it off for yourself in Pocket Core → Settings → Apps acting for me); <b>Direct</b> uses a URL and an API token you enter.{!coreOn && <CoreHint />}</div></div>
      {APPS.map(a => {
        const mode = f[`${a.slug}_mode`]
        const via = f[`${a.slug}_via`]
        const name = APP_INFO[a.slug].name
        const st = states?.find(x => x.slug === a.slug)
        return (
          <Section key={a.slug} title={a.title} icon={APP_INFO[a.slug].icon} actions={st && st.state !== 'on' && st.state !== 'off' ? <span className={`badge ${st.state === 'partly' ? 'ok' : 'warn'}`}>{STATE_WORDS[st.state]}</span> : connBadge(eff(mode), via)} sub={a.sub}>
            <Connection app={name} mode={mode} via={via} detail={f[`${a.slug}_detail`]} core={coreOn} hasToken={!!f[`${a.slug}_token`]}
              onMode={m => save({ [`${a.slug}_mode`]: m } as Partial<SettingsT>)} onDisconnect={() => save({ [`${a.slug}_url`]: '', [`${a.slug}_token`]: '' } as Partial<SettingsT>)}
              test={<><button type="button" className="btn sm" onClick={() => run(a.slug, f[`${a.slug}_url`], f[`${a.slug}_token`])}>Test connection</button>{status(a.slug)}</>}
              direct={<div className="two-col tight">
                <Field label="URL" help="Inside Docker use http://host.docker.internal:PORT or the service name."><input value={f[`${a.slug}_url`]} onChange={e => set(`${a.slug}_url`, e.target.value)} placeholder={`http://host.docker.internal:${a.port}`} /></Field>
                <Field label="API token" help={a.token}><input value={f[`${a.slug}_token`]} onChange={e => set(`${a.slug}_token`, e.target.value)} autoComplete="off" /></Field>
              </div>} />
            {st?.fix && st.state !== 'on' && <p className="muted small"><Icon name="info" size={12} className="inline-ico" /> {st.fix}.</p>}
          </Section>
        )
      })}
      <Section title="Other Pocket apps" icon="layers" sub="Translate offers itself to the rest of the box through Pocket Core: Pocket Chat and Mail translate a message, Notes and Blog a page; Search finds your phrases, Pocket Day writes what you translated into your journal, and Pocket AI can translate when you ask it. Switched off until you allow them in Pocket Core → Links.">
        <dl className="set-dl">
          <div><dt>translate:run</dt><dd className="muted">Translate text for you, with your engine, tone and glossary; tell a text’s language; explain a phrase.</dd></div>
          <div><dt>phrasebook:read</dt><dd className="muted">Your phrasebook, history and glossary.</dd></div>
          <div><dt>phrasebook:write</dt><dd className="muted">Save a phrase to your phrasebook; that app may change only the phrases it added.</dd></div>
          <div><dt>events:read</dt><dd className="muted"><code>translate.phrase_saved</code>, <code>translate.document_done</code> — for Pocket Flows.</dd></div>
        </dl>
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button></div>
    </div>
  )
}

function AITab() {
  const { f, setF, save, busy } = useSettingsForm()
  const { run, status } = useTester()
  const { data: ai } = useAsync(() => api.get<{ ready: boolean; provider: string; detail: string }>(`${V1}/ai/status`), [])
  if (!f) return <Loading />
  const coreOn = f.core_configured && f.core_registered !== false
  const shown = !coreOn && f.ai_provider === 'pocket' ? 'openai' : (f.ai_provider || 'auto')
  return (
    <div className="stack loose">
      <Section title="What the AI does" icon="sparkles" sub="It translates with the whole meaning in view: the tone you ask for, your glossary, idioms said the way a native speaker would. It offers other ways to say a sentence and explains phrases. Only the text you translate is sent. Choose the offline engines (Settings → Translating) and nothing goes to an AI.">
        {ai && (ai.ready ? <p className="muted small row" style={{ gap: 6 }}><Icon name="sparkles" size={13} />{ai.provider === 'pocket' ? 'Pocket AI' : 'Your OpenAI key'} is ready.</p>
          : <div className="callout info"><Icon name="info" size={15} /><div>No AI connected{ai.detail ? ` (${ai.detail})` : ''}.{!coreOn && <CoreHint />}</div></div>)}
      </Section>
      <Section title="Provider" icon="sparkles">
        <div className="grid" style={{ gap: 12 }}>
          <Field group label="Who writes" help="Automatic prefers your enabled Pocket AI connection; without one it uses your OpenAI settings.">
            <Select ariaLabel="AI provider" value={shown} onChange={v => setF({ ...f, ai_provider: v as SettingsT['ai_provider'] })} options={[{ value: 'auto', label: coreOn ? 'Automatic — prefer Pocket AI' : 'Automatic' }, { value: 'pocket', label: 'Pocket AI', disabled: !coreOn, hint: coreOn ? undefined : 'needs Pocket Core' }, { value: 'openai', label: 'OpenAI / compatible API' }]} />
          </Field>
          {!coreOn && <CoreHint />}
          {coreOn && shown !== 'openai' && <PocketModelSelect value={f.ai_pocket_model || ''} onChange={v => setF({ ...f, ai_pocket_model: v })} />}
          {(shown === 'openai' || (!coreOn && shown === 'auto')) && <>
            <Field label="OpenAI API key" help="platform.openai.com → API keys. Stored on the server; shown masked."><input value={f.openai_api_key} onChange={e => setF({ ...f, openai_api_key: e.target.value })} autoComplete="off" placeholder="sk-…" /></Field>
            <div className="two-col tight">
              <Field label="Model" help="Default gpt-4o-mini."><input value={f.openai_model} onChange={e => setF({ ...f, openai_model: e.target.value })} placeholder="gpt-4o-mini" /></Field>
              <Field label="Base URL (optional)" help="Ollama, LM Studio, OpenRouter…"><input value={f.openai_base_url} onChange={e => setF({ ...f, openai_base_url: e.target.value })} placeholder="https://api.openai.com/v1" /></Field>
            </div>
          </>}
        </div>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button>
        {(shown === 'openai' || (!coreOn && shown === 'auto')) && <><button type="button" className="btn" onClick={() => run('openai', f.openai_base_url, f.openai_api_key)} disabled={!f.openai_api_key}>Test the OpenAI key</button>{status('openai')}</>}
      </div>
    </div>
  )
}

function Connection({ app, mode: saved, via, detail, core, onMode, onDisconnect, hasToken, test, direct }: {
  app: string; mode: ConnMode; via: string; detail: string; core: boolean; onMode: (m: ConnMode) => void; onDisconnect: () => void
  hasToken: boolean; test: React.ReactNode; direct: React.ReactNode
}) {
  const [advanced, setAdvanced] = useState(saved === 'direct')
  const mode: ConnMode = !core && saved === 'core' ? 'off' : saved
  const ok = via !== '' && mode !== 'off'
  const showDirect = !core || mode === 'direct' || advanced
  const line = mode === 'off' ? `Off: Translate does not reach ${app}.`
    : mode === 'core' ? (ok ? `Connected through Pocket Core, as you${detail ? ` (${detail})` : ''}. Nothing to fill in; switch it off for yourself in Pocket Core → Settings → Apps acting for me.` : `Not connected: ${detail}.`)
    : (ok ? `Direct connection with your own ${app} URL and token; use Test connection to check it.` : `Not connected: ${detail || 'add the URL and a token'}.`)
  return (
    <div className="conn-block">
      <div className="conn-mode">
        <Segmented size="sm" value={mode} onChange={m => onMode(m as ConnMode)} options={[
          { value: 'core', label: 'Pocket Core', title: core ? 'Through Pocket Core: per person, nothing to paste' : 'Needs Pocket Core: there is none on this box', disabled: !core },
          ...(showDirect ? [{ value: 'direct', label: 'Direct', title: 'Your own URL and API token' }] : []),
          { value: 'off', label: 'Off' },
        ]} />
        {!showDirect && <button type="button" className="conn-advanced" onClick={() => setAdvanced(true)}>Advanced: connect directly with a URL and key</button>}
      </div>
      <div className={`conn-status ${mode === 'off' ? '' : ok ? 'ok' : 'bad'}`}><span className="dot" />{line}</div>
      {mode === 'direct' && <>
        {direct}
        <div className="row wrap">{test}{hasToken && <button type="button" className="btn sm ghost danger-text" onClick={onDisconnect} title="Remove the saved URL and token">Disconnect</button>}</div>
      </>}
      {mode === 'core' && <div className="row wrap">{test}</div>}
    </div>
  )
}

const connBadge = (mode: ConnMode, via: string) => mode === 'off' ? <span className="badge">off</span>
  : via ? <span className="badge ok">{via === 'core' ? 'Pocket Core' : 'direct'}</span>
  : <span className="badge warn">not connected</span>

function Users() {
  const { user, users, reloadMeta, status } = useApp()
  const { toast } = useToast()
  const [edit, setEdit] = useState<Partial<User> | null>(null)
  const [del, setDel] = useState<User | null>(null)
  const isAdmin = user?.role === 'admin'
  const run = async (fn: () => Promise<unknown>, msg: string) => { try { await fn(); await reloadMeta(); toast(msg) } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <Section title="People on this box" actions={isAdmin && <button type="button" className="btn primary sm" onClick={() => setEdit({ role: 'member' })}><Icon name="plus" size={14} />Add person</button>}
      sub="Everyone signs in with their own account and has their own forms, answers, settings and connections. Admins manage accounts; they do not see anyone else’s forms or answers.">
      <div className="set-people">
        {users.map(u => (
          <div key={u.id} className="set-person">
            <Avatar name={u.name} email={u.email} size={34} />
            <div className="grow">
              <div className="set-person-name">{u.name || <span className="muted">—</span>}{u.id === user?.id && <span className="badge">you</span>}{u.role === 'admin' && <span className="badge accent">admin</span>}</div>
              <div className="muted small ellipsis">{u.email}</div>
            </div>
            {isAdmin && <span className="row" style={{ justifyContent: 'flex-end' }}><button type="button" className="btn ghost sm" onClick={() => setEdit(u)}>Edit</button>{u.id !== user?.id && <button type="button" className="btn ghost sm danger-text" onClick={() => setDel(u)}>Remove</button>}</span>}
          </div>
        ))}
      </div>
      {status?.auth_mode !== 'local' && <p className="muted small mt">Auth mode is <code>{status?.auth_mode}</code>: users are created automatically on first visit; passwords are not used here.</p>}
      {edit && <UserModal user={edit} local={status?.auth_mode === 'local'} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reloadMeta() }} />}
      {del && <Confirm confirmLabel="Remove" title={`Remove ${del.name || del.email}?`} message="Their account is deleted together with their forms and answers; their form links stop working." onCancel={() => setDel(null)} onConfirm={() => { const d = del; setDel(null); run(() => api.del(`${V1}/users/${d.id}`), 'User removed') }} />}
    </Section>
  )
}

function UserModal({ user, local, onClose, onSaved }: { user: Partial<User>; local: boolean; onClose: () => void; onSaved: () => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ name: user.name || '', email: user.email || '', password: '', role: user.role || 'member' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { if (user.id) await api.put(`${V1}/users/${user.id}`, f); else await api.post(`${V1}/users`, f); toast(user.id ? 'User saved' : 'User added'); onSaved() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={user.id ? 'Edit person' : 'Add a person'} onClose={onClose} size="sm">
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} autoFocus required maxLength={60} /></Field>
        <Field label="Email" help="Used to sign in."><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} required /></Field>
        {local && <Field label={user.id ? 'New password (leave empty to keep)' : 'Password'} help="At least 8 characters."><input type="password" value={f.password} onChange={e => setF({ ...f, password: e.target.value })} minLength={user.id ? 0 : 8} required={!user.id} autoComplete="new-password" /></Field>}
        <Field group label="Role"><Segmented value={f.role} onChange={v => setF({ ...f, role: v })} options={[{ value: 'member', label: 'Member' }, { value: 'admin', label: 'Admin' }]} /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{user.id ? 'Save' : 'Add person'}</button></div>
      </form>
    </Modal>
  )
}

function Profile() {
  const { user, setUser, status } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState({ name: user?.name || '', email: user?.email || '', current_password: '', new_password: '' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { setUser(await api.put(`${V1}/auth/me`, f)); setF(x => ({ ...x, current_password: '', new_password: '' })); toast('Profile saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <form onSubmit={save} className="stack loose">
      <div className="set-profile">
        <Avatar name={user?.name} email={user?.email} size={56} />
        <div className="grow"><h3>{user?.name || user?.email}</h3><p className="muted small">{user?.email} · {user?.role}</p></div>
      </div>
      <Section title="You">
        <div className="stack">
          <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} /></Field>
          <Field label="Email"><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} /></Field>
        </div>
      </Section>
      {status?.auth_mode === 'local' && <Section title="Password" sub="Leave empty to keep the current one."><div className="stack">
        <Field label="Current password"><input type="password" value={f.current_password} onChange={e => setF({ ...f, current_password: e.target.value })} autoComplete="current-password" /></Field>
        <Field label="New password" help="At least 8 characters."><input type="password" value={f.new_password} onChange={e => setF({ ...f, new_password: e.target.value })} autoComplete="new-password" /></Field>
      </div></Section>}
      <div className="row"><button className="btn primary" disabled={busy}>Save changes</button></div>
    </form>
  )
}

function Tokens() {
  const { toast } = useToast()
  const { settings } = useApp()
  const { data, reload } = useAsync(() => api.get<APIToken[]>(`${V1}/tokens`), [])
  const [name, setName] = useState('')
  const [fresh, setFresh] = useState('')
  const [del, setDel] = useState<APIToken | null>(null)
  const create = async (e: React.FormEvent) => {
    e.preventDefault()
    try { const r = await api.post<{ token: string }>(`${V1}/tokens`, { name: name || 'token' }); setFresh(r.token); setName(''); reload() } catch (e) { toast((e as Error).message, 'err') }
  }
  const base = settings?.base_url || window.location.origin
  return (
    <div className="stack loose">
      <Section title="Personal tokens" icon="key" sub={<>Use as <code>Authorization: Bearer &lt;token&gt;</code>. A token acts as you: it can read and change everything, feed a Homepage widget (<code>GET /api/v1/summary</code>) or send a recording from a Siri Shortcut. See <code>docs/api.md</code>.</>}>
        <form onSubmit={create} className="row mb"><input value={name} onChange={e => setName(e.target.value)} placeholder="Token name (e.g. iPhone Shortcut)" /><button className="btn primary"><Icon name="plus" size={15} />Create</button></form>
        {fresh && <div className="set-fresh mb"><div className="row"><Icon name="check-circle" size={16} /><b>Copy it now — it is shown once.</b></div><pre className="mono">{fresh}</pre></div>}
        {data && data.length > 0 ? (
          <table className="table compact">
            <thead><tr><th>Name</th><th>Prefix</th><th>Last used</th><th>Created</th><th /></tr></thead>
            <tbody>{data.map(t => <tr key={t.id}><td>{t.name}</td><td className="mono">{t.prefix}…</td><td className="muted">{t.last_used_at ? fmtDateTime(t.last_used_at) : 'never'}</td><td className="muted">{fmtDateTime(t.created_at)}</td><td className="right"><button type="button" className="btn ghost sm danger-text" onClick={() => setDel(t)}>Revoke</button></td></tr>)}</tbody>
          </table>
        ) : <p className="muted">No tokens yet.</p>}
        {del && <Confirm confirmLabel="Revoke" title={`Revoke “${del.name}”?`} message="Anything using this token stops working immediately." onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/tokens/${del.id}`); setDel(null); reload() }} />}
      </Section>
      <Section title="Endpoints" icon="code" sub={<>At <code>{base}/api/v1</code>.</>}>
        <dl className="set-dl">
          <div><dt><code>GET /responses?status=new</code></dt><dd className="muted">Your answers, with <code>form</code>, <code>q</code>, <code>email</code>, <code>day</code> and <code>starred</code> filters.</dd></div>
          <div><dt><code>GET /forms</code></dt><dd className="muted">Your forms with their answers, new answers and how many opened them; <code>POST /forms</code> with <code>{'{"template":"contact"}'}</code> makes one.</dd></div>
          <div><dt><code>GET /summary</code></dt><dd className="muted">New answers, forms taking answers and this week — for Homepage.</dd></div>
          <div><dt><code>GET /day/YYYY-MM-DD</code></dt><dd className="muted">The answers that came in that day — what Pocket Day reads.</dd></div>
          <div><dt><code>GET /forms/&lt;id&gt;/export.csv</code></dt><dd className="muted">Every answer of a form as a spreadsheet.</dd></div>
        </dl>
      </Section>
    </div>
  )
}

function Data() {
  const { user } = useApp()
  const [importing, setImporting] = useState(false)
  return (
    <div className="stack loose">
      <Section title="Bring phrases in" icon="upload" sub="Google Translate’s saved phrases (Google Takeout → Translate → Saved translations.csv), any CSV with from, to, text and translation, or a Pocket Translate export. Bringing the same file twice adds nothing twice. Glossaries come in on the Glossary page.">
        <button type="button" className="btn primary" onClick={() => setImporting(true)}><Icon name="upload" size={15} />Bring phrases…</button>
      </Section>
      <Section title="Export" icon="archive" sub="Everything of yours as one JSON file: the history and phrasebook, the glossary and the conversations, with the list of your documents (it comes back in with “Bring phrases”, also on another box). Or the history as one table for a spreadsheet.">
        <div className="row wrap"><a className="btn" href={`${V1}/export.json`}><Icon name="download" size={15} />Export JSON</a><a className="btn" href={`${V1}/export.csv`}><Icon name="download" size={15} />History as CSV</a><a className="btn" href={`${V1}/export.csv?starred=1`}><Icon name="download" size={15} />Phrasebook as CSV</a><a className="btn" href={`${V1}/glossary.csv`}><Icon name="download" size={15} />Glossary as CSV</a></div>
      </Section>
      {user?.role === 'admin' && <Section title="Database backup" icon="database" sub="The whole box, every account and every document — admins only.">
        <p className="muted">A consistent copy of the SQLite database (<code>VACUUM INTO</code>). Restore by stopping the app and replacing <code>pocket-translate.db</code> in the data folder.</p>
        <a className="btn" href={`${V1}/backup`}><Icon name="database" size={15} />Download database</a>
      </Section>}
      {importing && <ImportModal onClose={() => setImporting(false)} />}
    </div>
  )
}

function About() {
  const { data, error } = useAsync(() => api.get<Record<string, unknown>>(`${V1}/settings/system`), [])
  const [keys, setKeys] = useState(false)
  if (error) return <div className="callout danger"><Icon name="alert" size={15} /><span>{error}</span></div>
  if (!data) return <Loading />
  const counts = (data.counts || {}) as Record<string, number>
  const core = (data.core || {}) as { configured?: boolean; registered?: boolean; url?: string; error?: string }
  const tiles: [string, number | undefined, string][] = [['Translations', counts.translations, 'languages'], ['Documents', counts.documents, 'file'], ['Conversations', counts.conversations, 'message'], ['Glossary terms', counts.glossary, 'book-open']]
  const rows: [string, React.ReactNode][] = [
    ['Version', <code key="v">{String(data.version)}</code>],
    ['Uptime', String(data.uptime)],
    ['Runtime', `${String(data.go)} · ${String(data.os)}/${String(data.arch)}`],
    ['Time zone', String(data.timezone)],
    ['Sign-in', String(data.auth_mode)],
    ['Pocket Core', core.configured ? (core.registered ? <span key="c" className="ok">connected · {core.url}</span> : <span key="c" className="bad">{core.error || 'not registered yet'}</span>) : <span key="c" className="muted">not configured</span>],
    ['Database', <span key="db"><code>{String(data.db_path)}</code> · {fmtBytes(Number(data.db_bytes))}</span>],
    ['People', `${counts.users ?? 0} accounts on this box`],
  ]
  return (
    <div className="stack loose">
      <div className="set-about-hero">
        <img src="/favicon.svg" alt="" width={48} height={48} />
        <div>
          <h3>Pocket Translate</h3>
          <p className="muted small">Translate text, photos, documents and conversations on your own server — with Pocket AI, or offline with Argos and NLLB-200. A phrasebook, a glossary and your history, linked to Voice, Learn, Notes, Files and Documents. One binary, one SQLite file, one volume.</p>
        </div>
      </div>
      <div className="set-stats">
        {tiles.map(([label, n, icon]) => (
          <div key={label} className="set-stat"><Icon name={icon} size={16} /><span className="set-stat-n tnum">{(n ?? 0).toLocaleString()}</span><span className="set-stat-l">{label}</span></div>
        ))}
      </div>
      <Section title="This server">
        <dl className="set-dl">
          {rows.map(([k, v]) => <div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}
        </dl>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn" onClick={() => setKeys(true)}><Icon name="keyboard" size={15} />Keyboard shortcuts</button>
        <a className="btn" href="https://github.com/abjelosevic88/pocket-homelab-translate" target="_blank" rel="noreferrer"><Icon name="external" size={15} />Project on GitHub</a>
      </div>
      <p className="muted small">Open source (MIT). Part of the Pocket apps: Pocket Core connects them — Voice, Learn, Notes, Files, Documents, Chat, Mail, Day, Search, Pocket AI and more.</p>
      {keys && <ShortcutsSheet onClose={() => setKeys(false)} />}
    </div>
  )
}

