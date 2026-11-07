// Reading aloud (the browser's own voices, on the device) and recording
// from the microphone (written out on the server by Pocket Voice or the
// person's OpenAI key).
import { useCallback, useEffect, useRef, useState } from 'react'

export const canSpeak = typeof window !== 'undefined' && 'speechSynthesis' in window

let voices: SpeechSynthesisVoice[] = []
if (canSpeak) {
  const load = () => { voices = window.speechSynthesis.getVoices() }
  load()
  window.speechSynthesis.addEventListener?.('voiceschanged', load)
}

/** A voice for a BCP 47 tag ("sr-RS"), else one for its language, else none. */
function voiceFor(tag: string): SpeechSynthesisVoice | undefined {
  const t = tag.toLowerCase()
  const lang = t.split('-')[0]
  return voices.find(v => v.lang.toLowerCase() === t) || voices.find(v => v.lang.toLowerCase().replace('_', '-').startsWith(lang + '-')) || voices.find(v => v.lang.toLowerCase() === lang)
    // Serbian Latin is often voiced by Croatian or Bosnian voices
    || (lang === 'sr' ? voices.find(v => /^(hr|bs)/i.test(v.lang)) : undefined)
}

export function hasVoice(tag: string): boolean { return canSpeak && (!!voiceFor(tag) || voices.length === 0) }

/** Reads text aloud; resolves when done. Calling it again stops the last one. */
export function speak(text: string, tag: string, rate = 1): Promise<void> {
  if (!canSpeak || !text.trim()) return Promise.resolve()
  window.speechSynthesis.cancel()
  return new Promise(resolve => {
    const u = new SpeechSynthesisUtterance(text)
    u.lang = tag
    const v = voiceFor(tag)
    if (v) u.voice = v
    u.rate = rate
    u.onend = () => resolve()
    u.onerror = () => resolve()
    window.speechSynthesis.speak(u)
  })
}

export function stopSpeaking() { if (canSpeak) window.speechSynthesis.cancel() }

/** Which text is being read aloud now (for the speaker buttons). */
export function useSpeaking(): [string, (key: string, text: string, tag: string) => void] {
  const [key, setKey] = useState('')
  const toggle = useCallback((k: string, text: string, tag: string) => {
    if (key === k) { stopSpeaking(); setKey(''); return }
    setKey(k)
    speak(text, tag).then(() => setKey(cur => (cur === k ? '' : cur)))
  }, [key])
  useEffect(() => () => stopSpeaking(), [])
  return [key, toggle]
}

export const canRecord = typeof navigator !== 'undefined' && !!navigator.mediaDevices?.getUserMedia && typeof MediaRecorder !== 'undefined'

function pickType(): string {
  for (const t of ['audio/webm;codecs=opus', 'audio/webm', 'audio/mp4', 'audio/ogg;codecs=opus']) {
    if (MediaRecorder.isTypeSupported?.(t)) return t
  }
  return ''
}

export interface Recording { blob: Blob; name: string }

/** Microphone recording: start, then stop to get the sound. */
export function useRecorder(maxSeconds = 120) {
  const [recording, setRecording] = useState(false)
  const [seconds, setSeconds] = useState(0)
  const [level, setLevel] = useState(0)
  const rec = useRef<MediaRecorder | null>(null)
  const chunks = useRef<Blob[]>([])
  const done = useRef<((r: Recording | null) => void) | null>(null)
  const stream = useRef<MediaStream | null>(null)
  const timer = useRef<number>(0)
  const raf = useRef<number>(0)
  const ctxRef = useRef<AudioContext | null>(null)

  const cleanup = () => {
    window.clearInterval(timer.current)
    cancelAnimationFrame(raf.current)
    stream.current?.getTracks().forEach(t => t.stop())
    stream.current = null
    ctxRef.current?.close().catch(() => {})
    ctxRef.current = null
    setRecording(false)
    setLevel(0)
  }

  const start = useCallback(async (): Promise<void> => {
    if (rec.current) return
    const s = await navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true } })
    stream.current = s
    const type = pickType()
    const r = new MediaRecorder(s, type ? { mimeType: type } : undefined)
    chunks.current = []
    r.ondataavailable = e => { if (e.data.size) chunks.current.push(e.data) }
    r.onstop = () => {
      const t = r.mimeType || type || 'audio/webm'
      const blob = new Blob(chunks.current, { type: t })
      const ext = t.includes('mp4') ? 'm4a' : t.includes('ogg') ? 'ogg' : 'webm'
      cleanup()
      rec.current = null
      done.current?.(blob.size > 800 ? { blob, name: `speech.${ext}` } : null)
      done.current = null
    }
    rec.current = r
    r.start(250)
    setSeconds(0)
    setRecording(true)
    const began = Date.now()
    timer.current = window.setInterval(() => {
      const n = Math.floor((Date.now() - began) / 1000)
      setSeconds(n)
      if (n >= maxSeconds) r.state === 'recording' && r.stop()
    }, 250)
    try {
      const ac = new AudioContext()
      ctxRef.current = ac
      const an = ac.createAnalyser()
      an.fftSize = 512
      ac.createMediaStreamSource(s).connect(an)
      const buf = new Uint8Array(an.fftSize)
      let last = 0
      const tick = (t: number) => {
        if (t - last > 120) { // calm: a few updates a second
          last = t
          an.getByteTimeDomainData(buf)
          let peak = 0
          for (const v of buf) peak = Math.max(peak, Math.abs(v - 128))
          setLevel(Math.min(1, peak / 64))
        }
        raf.current = requestAnimationFrame(tick)
      }
      raf.current = requestAnimationFrame(tick)
    } catch { /* no level meter */ }
  }, [maxSeconds])

  const stop = useCallback((): Promise<Recording | null> => new Promise(resolve => {
    const r = rec.current
    if (!r) { resolve(null); return }
    done.current = resolve
    if (r.state === 'recording') r.stop()
  }), [])

  const cancel = useCallback(() => {
    const r = rec.current
    done.current = null
    if (r && r.state === 'recording') { r.onstop = () => { cleanup(); rec.current = null }; r.stop() }
  }, [])

  useEffect(() => () => { cancel() }, [cancel])
  return { recording, seconds, level, start, stop, cancel }
}
