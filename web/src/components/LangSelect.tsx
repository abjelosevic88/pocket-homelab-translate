// Picking a language: the ones you use first, then every language; each
// says whether it works offline here.
import { useMemo } from 'react'
import { useApp } from '../lib/app-context'
import { useData } from '../lib/data'
import { offlineKnows } from '../lib/translate'
import { Select, type SelectOption } from './Select'

export function LangSelect({ value, onChange, auto, any, ariaLabel, className = '', disabled, detected }: {
  value: string; onChange: (v: string) => void; auto?: boolean; any?: boolean; ariaLabel: string; className?: string; disabled?: boolean
  /** shown on "Detect language" once known */
  detected?: string
}) {
  const { settings } = useApp()
  const { langs, byCode, engines } = useData()
  const options = useMemo<SelectOption[]>(() => {
    const out: SelectOption[] = []
    const aiReady = !!engines?.ai.ready
    const hint = (code: string) => {
      const l = byCode[code]
      if (!engines || !l) return undefined
      if (offlineKnows(l)) return aiReady ? undefined : 'offline'
      return aiReady ? 'AI only' : 'needs AI'
    }
    if (auto) out.push({ value: 'auto', label: detected && detected !== 'auto' ? `Detect · ${byCode[detected]?.name || detected}` : 'Detect language' })
    if (any) out.push({ value: '*', label: 'Any language' })
    const recent = (settings?.recent_langs || []).filter(c => byCode[c])
    for (const c of recent) out.push({ value: c, label: byCode[c].name, hint: hint(c), group: 'Recent' })
    for (const l of langs || []) {
      if (recent.includes(l.code)) continue
      out.push({ value: l.code, label: l.name, hint: hint(l.code) || (l.native !== l.name ? l.native : undefined), group: 'All languages' })
    }
    return out
  }, [langs, byCode, engines, settings?.recent_langs, auto, any, detected])
  return <Select className={`lang-sel ${className}`} ariaLabel={ariaLabel} value={value} onChange={onChange} options={options} disabled={disabled} placeholder="Language" />
}
