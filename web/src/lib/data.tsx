// What the whole app shares: the languages (with what can be done in each
// here), which engines work, and a way for pages to say "something changed".
import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import { api, V1 } from './api'
import type { Engines, Lang } from './types'

interface Data {
  langs: Lang[] | null
  byCode: Record<string, Lang>
  engines: Engines | null
  reload: () => void
}
const Ctx = createContext<Data>({ langs: null, byCode: {}, engines: null, reload: () => {} })
export const useData = () => useContext(Ctx)

export function DataProvider({ children }: { children: React.ReactNode }) {
  const [langs, setLangs] = useState<Lang[] | null>(null)
  const [engines, setEngines] = useState<Engines | null>(null)
  const reload = useCallback(() => {
    api.get<{ languages: Lang[] }>(`${V1}/languages`).then(r => setLangs(r.languages)).catch(() => setLangs([]))
    api.get<Engines>(`${V1}/engines`).then(setEngines).catch(() => {})
  }, [])
  useEffect(() => { reload() }, [reload])
  useOnChange('engines', reload)
  const byCode = useMemo(() => Object.fromEntries((langs || []).map(l => [l.code, l])), [langs])
  return <Ctx.Provider value={{ langs, byCode, engines, reload }}>{children}</Ctx.Provider>
}

/** Tell the open pages that something changed ("history", "documents"…). */
export function announceChange(what: string) { window.dispatchEvent(new CustomEvent('ptl:changed', { detail: what })) }

export function useOnChange(what: string, fn: () => void) {
  useEffect(() => {
    const h = (e: Event) => { const d = (e as CustomEvent<string>).detail; if (d === what || d === '*') fn() }
    window.addEventListener('ptl:changed', h)
    return () => window.removeEventListener('ptl:changed', h)
  }, [what, fn])
}
