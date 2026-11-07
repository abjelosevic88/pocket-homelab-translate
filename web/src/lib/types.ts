export interface User { id: number; email: string; name: string; role: 'admin' | 'member'; created_at: string }
export interface APIToken { id: number; user_id: number; name: string; prefix: string; last_used_at: string | null; created_at: string }
export type ConnMode = 'core' | 'direct' | 'off'
export type ConnVia = '' | 'core' | 'direct'
export type AppSlug = 'voice' | 'learn' | 'notes' | 'files' | 'documents'
type ConnFields = { [K in AppSlug as `${K}_mode`]: ConnMode } & { [K in AppSlug as `${K}_via`]: ConnVia } & { [K in AppSlug as `${K}_detail` | `${K}_url` | `${K}_token` | `${K}_public_url` | `${K}_open_url`]: string } & { [K in AppSlug as `${K}_configured`]: boolean }
export type EngineChoice = 'auto' | 'ai' | 'offline'
export type Tone = '' | 'formal' | 'casual'
export type Settings = ConnFields & {
  core_configured: boolean
  core_registered?: boolean
  title: string; date_format: string; time_format: string; week_start: number
  source_lang: string; target_lang: string; recent_langs: string[]; engine: EngineChoice; tone: Tone
  live_translate: boolean; keep_history: boolean; history_days: number; keep_app_history: boolean
  ocr_langs: string[]; speech: 'voice' | 'openai'; speech_model: string; auto_speak: boolean
  learn_deck: string; notes_folder: string; files_folder: string
  notify_docs: boolean; notify_url: string; notify_format: string; notify_configured: boolean
  ai_provider: 'auto' | 'pocket' | 'openai'; ai_pocket_model: string; openai_api_key: string; openai_model: string; openai_base_url: string
  argos_url?: string; argos_key?: string; nllb_url?: string; nllb_key?: string
  setup_complete: boolean; base_url: string; demo: boolean
}
export type AppStateName = 'on' | 'partly' | 'off' | 'link_off' | 'missing' | 'old' | 'no_core' | 'not_set_up' | 'error'
export interface AppState { slug: AppSlug; name: string; reason: string; mode: ConnMode; via: ConnVia; state: AppStateName; detail: string; fix: string; on: string[]; open_url: string }

export interface Lang { code: string; name: string; native: string; rtl?: boolean; speech: string; ocr: string; argos: boolean; nllb: boolean; read: boolean }
export interface EngineState { configured: boolean; ready: boolean; detail: string; languages: number }
export interface Engines {
  ai: { ready: boolean; provider: string; detail: string }
  argos: EngineState; nllb: EngineState
  ocr: { ready: boolean; languages: string[] }
  pdf: { ready: boolean }
  speech: { ready: boolean; detail: string; via: string }
}
export type Kind = 'text' | 'photo' | 'speech' | 'app' | 'import'
export interface Translation {
  id: number; kind: Kind; source: string; source_lang: string; detected_lang: string; target_lang: string
  source_text: string; result_text: string; engine: string; model: string; tone: string
  starred: boolean; collection: string; note: string; learn_ref: string; created_at: string; updated_at: string
}
export interface TranslateResult {
  text: string; detected: string; engine: string; provider: string; alternatives: string[]; terms: number; note: string; to: string
  translation?: Translation
}
export interface Term { id: number; source_lang: string; target_lang: string; term: string; translation: string; note: string; match_case: boolean; created_at: string }
export type DocStatus = 'queued' | 'working' | 'done' | 'failed'
export interface Doc {
  id: number; name: string; format: string; mime: string; size: number; source_lang: string; detected_lang: string; target_lang: string
  engine: EngineChoice; used: string; status: DocStatus; done: number; total: number; error: string; result_name: string; origin: string
  saved: { app: string; title: string; url: string }[]; created_at: string; finished_at: string
}
export interface Conversation { id: number; title: string; lang_a: string; lang_b: string; created_at: string; updated_at: string; turns: number; last: string }
export interface Turn { id: number; conversation_id: number; side: 'a' | 'b'; spoken: boolean; source_lang: string; target_lang: string; source_text: string; result_text: string; engine: string; created_at: string }
export interface Collection { name: string; count: number }
export interface Stats { total: number; starred: number; today: number; week: number; documents: number; conversations: number; terms: number; pairs: { from: string; to: string; count: number }[]; kinds: Record<string, number> }
