import type { AppSlug, AppStateName, Engines, Lang } from './types'

export const APP_INFO: Record<AppSlug, { name: string; icon: string; what: string }> = {
  voice: { name: 'Pocket Voice', icon: 'mic', what: 'speech written out on this server' },
  learn: { name: 'Pocket Learn', icon: 'brain', what: 'flashcards from your phrasebook' },
  notes: { name: 'Pocket Notes', icon: 'note', what: 'translations and conversations as notes' },
  files: { name: 'Pocket Files', icon: 'file', what: 'documents from and to your files' },
  documents: { name: 'Pocket Documents', icon: 'archive', what: 'documents from and to your archive' },
}

export const STATE_WORDS: Record<AppStateName, string> = {
  on: 'Connected', partly: 'Partly connected', off: 'Switched off here', link_off: 'Switched off in Pocket Core', missing: 'Not on this box',
  old: 'Needs a newer version', no_core: 'No Pocket Core', not_set_up: 'Not set up', error: 'Not reachable',
}

export const KIND_WORDS: Record<string, { label: string; icon: string }> = {
  text: { label: 'Typed', icon: 'text' }, photo: { label: 'Photo', icon: 'camera' }, speech: { label: 'Spoken', icon: 'mic' },
  app: { label: 'From another app', icon: 'link' }, import: { label: 'Imported', icon: 'upload' },
}

export function langName(code: string, byCode: Record<string, Lang>): string {
  if (!code || code === 'auto') return 'Detect language'
  if (code === '*') return 'Any language'
  return byCode[code]?.name || code
}

/** The short form for chips: "EN", "SR", "SR Ћ". */
export function langShort(code: string): string {
  if (code === 'sr-Latn') return 'SR'
  if (code === 'sr-Cyrl') return 'СР'
  if (code === 'zt') return 'ZH繁'
  if (code === 'pb') return 'PT-BR'
  return code.toUpperCase()
}

export function speechTag(code: string, byCode: Record<string, Lang>): string { return byCode[code]?.speech || code }

/** How a translation was made, in words. */
export function engineLabel(engine: string, provider?: string): string {
  switch (engine) {
    case 'ai': return provider === 'openai' ? 'AI (your OpenAI key)' : 'Pocket AI'
    case 'argos': return 'Offline · Argos'
    case 'nllb': return 'Offline · NLLB'
    case 'none': return 'Same language'
    case 'import': return 'Imported'
    case 'manual': return 'Written by you'
  }
  return engine || ''
}

/** What "auto" will use for a pair right now (for the hint in the bar). */
export function effectiveEngine(choice: string, engines: Engines | null): 'ai' | 'offline' | 'none' {
  if (!engines) return 'none'
  const offline = engines.argos.ready || engines.nllb.ready
  if (choice === 'ai') return engines.ai.ready ? 'ai' : 'none'
  if (choice === 'offline') return offline ? 'offline' : 'none'
  return engines.ai.ready ? 'ai' : offline ? 'offline' : 'none'
}

/** Can a language be translated into offline here? */
export function offlineKnows(l: Lang | undefined): boolean { return !!l && (l.argos || l.nllb) }

export function dirOf(code: string, byCode: Record<string, Lang>): 'rtl' | 'ltr' { return byCode[code]?.rtl ? 'rtl' : 'ltr' }
