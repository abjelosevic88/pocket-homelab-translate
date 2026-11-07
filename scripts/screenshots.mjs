// README screenshots and a browser smoke test, against a demo instance (DEMO_DATA=true seeds months of translating):
//
//   DATA_DIR=/tmp/translate-demo DEMO_DATA=true ADMIN_EMAIL=demo@example.com ADMIN_PASSWORD=demodemo1 ADMIN_NAME=Aleksandar PORT=18251 ./pocket-translate
//   CDP=http://127.0.0.1:9340 BASE=http://127.0.0.1:18251 node scripts/screenshots.mjs   # any Chrome with remote debugging
//   SHOTS=0 … node scripts/screenshots.mjs                                                # checks only
// playwright-core comes from web/node_modules, or set PW_FROM to a package.json next to an install of it.
import { createRequire } from 'node:module'
import assert from 'node:assert/strict'

const require = createRequire(process.env.PW_FROM || new URL('../web/package.json', import.meta.url))
let pw
try { pw = require('playwright') } catch { pw = require('playwright-core') }
const { chromium } = pw
const base = process.env.BASE || 'http://127.0.0.1:18251'
const shots = process.env.SHOTS !== '0'
const only = process.env.ONLY || ''
const dir = process.env.OUT || new URL('../docs/screenshots/', import.meta.url).pathname
const browser = process.env.CDP ? await chromium.connectOverCDP(process.env.CDP) : await chromium.launch()
const errors = []

async function session({ phone = false, dark = false } = {}) {
  const ctx = await browser.newContext({
    viewport: phone ? { width: 390, height: 844 } : { width: 1440, height: 900 },
    deviceScaleFactor: phone ? 3 : 2, isMobile: phone, hasTouch: phone, colorScheme: dark ? 'dark' : 'light',
    userAgent: phone ? 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1' : undefined,
  })
  const page = await ctx.newPage()
  page.on('pageerror', e => errors.push(`pageerror: ${e}`))
  page.on('console', m => { if (m.type() === 'error' && !/Failed to load resource|Content Security Policy/.test(m.text())) errors.push(m.text()) })
  await page.goto(base + '/login')
  const res = await page.evaluate(() => fetch('/api/v1/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: 'demo@example.com', password: 'demodemo1' }) }).then(r => r.status))
  assert.equal(res, 200, 'demo sign-in')
  return { ctx, page }
}
async function go(page, path, wait = 1500) {
  await page.goto(base + path, { waitUntil: 'networkidle' })
  // headless Chrome freezes CSS fade-ins: show everything at once
  await page.addStyleTag({ content: '*, *::before, *::after { transition: none !important; animation: none !important; }' }).catch(() => {})
  await page.evaluate(() => document.fonts.ready)
  await page.waitForTimeout(wait)
}
async function shot(page, name) { if (shots && (!only || only.split(',').includes(name))) await page.screenshot({ path: dir + name + '.png' }) }
const api = async (page, path) => page.evaluate(p => fetch('/api/v1' + p).then(r => r.json()), path)
const sample = 'The meeting is moved to Thursday at ten. Could you send me the invoice by email before then?'

// ---- desktop ----
{
  const { ctx, page } = await session()
  await go(page, '/?text=' + encodeURIComponent(sample), 2500)
  assert.ok((await page.content()).includes('Recent'), 'the translator shows the recent translations')
  await shot(page, 'desktop-translate')
  await go(page, '/phrasebook', 1200)
  assert.ok((await page.content()).includes('Potremmo avere il conto'), 'the phrasebook has its phrases')
  await shot(page, 'desktop-phrasebook')
  await go(page, '/documents', 1200)
  assert.ok((await page.content()).includes('Rental agreement'), 'documents are listed')
  await shot(page, 'desktop-documents')
  const convs = await api(page, '/conversations')
  await go(page, `/talk/${convs.conversations.find(c => c.title.includes('Florence')).id}`, 1500)
  assert.ok((await page.content()).includes('mal di gola'), 'a conversation shows what was said')
  await shot(page, 'desktop-conversation')
  await go(page, '/history', 1200)
  await shot(page, 'desktop-history')
  await go(page, '/glossary', 1000)
  assert.ok((await page.content()).includes('faktura'), 'the glossary has its terms')
  await shot(page, 'desktop-glossary')
  await go(page, '/photo', 800)
  await shot(page, 'desktop-photo')
  await go(page, '/settings/engines', 1500)
  await shot(page, 'desktop-settings-engines')
  await go(page, '/settings/translating', 1200)
  await shot(page, 'desktop-settings-translating')
  await ctx.close()
}

// ---- phone ----
{
  const { ctx, page } = await session({ phone: true })
  await go(page, '/?text=' + encodeURIComponent('Where is the nearest pharmacy?'), 2500)
  await shot(page, 'phone-translate')
  const convs = await api(page, '/conversations')
  await go(page, `/talk/${convs.conversations.find(c => c.title.includes('heating')).id}`, 1500)
  await shot(page, 'phone-conversation')
  await go(page, '/phrasebook', 1200)
  await shot(page, 'phone-phrasebook')
  await go(page, '/documents', 1200)
  await shot(page, 'phone-documents')
  await ctx.close()
}

// ---- dark ----
{
  const { ctx, page } = await session({ dark: true })
  await go(page, '/?text=' + encodeURIComponent(sample), 2500)
  await shot(page, 'desktop-translate-dark')
  await ctx.close()
}

await browser.close()
if (errors.length) { console.error(errors.join('\n')); process.exit(1) }
console.log('ok')
