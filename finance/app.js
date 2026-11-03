'use strict'
// Big Bingo Finance - owner dashboard. No libraries, no inline scripts (strict CSP).

const BRAND = 'Big Bingo'
const TZ = 'Africa/Addis_Ababa'
const $ = (s, el = document) => el.querySelector(s)
const $$ = (s, el = document) => [...el.querySelectorAll(s)]
const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]))
const fmt = (n) => (n / 100).toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
const fmt0 = (n) => Math.round(n / 100).toLocaleString('en-US')
const int = (n) => Math.round(n).toLocaleString('en-US')
const compact = (n) => {
  const v = n / 100, a = Math.abs(v)
  if (a >= 1e6) return (v / 1e6).toFixed(2) + 'M'
  if (a >= 1e4) return (v / 1e3).toFixed(1) + 'k'
  return v.toLocaleString('en-US', { maximumFractionDigits: 0 })
}
const today = () => new Date().toLocaleDateString('en-CA', { timeZone: TZ })
const addDays = (s, n) => new Date(Date.parse(s + 'T00:00:00Z') + n * 864e5).toISOString().slice(0, 10)
const span = (a, b) => Math.round((Date.parse(b) - Date.parse(a)) / 864e5) + 1
const shortDay = (s) => new Date(s + 'T00:00:00Z').toLocaleDateString('en-US', { month: 'short', day: 'numeric', timeZone: 'UTC' })
const when = (iso) => new Date(iso).toLocaleString('en-US', { timeZone: TZ, month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
const joinDate = (ms) => new Date(ms).toLocaleDateString('en-US', { timeZone: TZ, year: 'numeric', month: 'short', day: 'numeric' })
const sign = (n) => (n > 0 ? 'pos' : n < 0 ? 'neg' : 'mut')
const COLORS = ['#2ee6a6', '#8b7bff', '#4dabff', '#ffc53d', '#ff6b8b', '#14b8a6', '#c084fc', '#fb923c']
const avatar = (name, id) => `<span class="av" style="background:${COLORS[Math.abs(Number(id) || 0) % COLORS.length]}">${esc((name || '?').trim().charAt(0).toUpperCase() || '?')}</span>`

const ICONS = {
  overview: '<rect x="3" y="3" width="7" height="9" rx="2"/><rect x="14" y="3" width="7" height="5" rx="2"/><rect x="14" y="12" width="7" height="9" rx="2"/><rect x="3" y="16" width="7" height="5" rx="2"/>',
  revenue: '<path d="M3 17l6-6 4 4 8-8"/><path d="M15 7h6v6"/>',
  people: '<circle cx="9" cy="8" r="3.5"/><path d="M2.5 20c.6-3.5 3.2-5.5 6.5-5.5s5.9 2 6.5 5.5"/><circle cx="17.5" cy="9" r="2.5"/><path d="M17 14.5c2.4.2 4 1.8 4.5 4.5"/>',
  cash: '<rect x="3" y="6" width="18" height="14" rx="3"/><path d="M3 10h18"/><circle cx="16.5" cy="15" r="1.2"/>',
  expenses: '<path d="M6 3h12v18l-3-2-3 2-3-2-3 2z"/><path d="M9 8h6M9 12h6"/>',
}
const icon = (k) => `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${ICONS[k]}</svg>`
const PAGES = [['overview', 'Overview'], ['revenue', 'Revenue'], ['people', 'People'], ['cash', 'Cash'], ['expenses', 'Expenses']]
const KINDS = {
  entry: ['Bet', 'v'], win: ['Prize won', 'g'], deposit: ['Deposit', 'b'], withdraw: ['Withdraw request', 'a'],
  refund: ['Refund', 'a'], signup_bonus: ['Signup bonus', 'v'], referral: ['Invite reward', 'v'], cashback: ['Cashback', 'v'],
}
const kindChip = (k) => `<span class="chip ${(KINDS[k] || ['', ''])[1]}">${esc((KINDS[k] || [k])[0])}</span>`

// ---------- state ----------
const S = { page: 'overview', preset: 30, from: '', to: '', cur: null, prev: null, owner: '', stamp: 0,
  people: { q: '', sort: 'joined', order: 'desc', page: 1, data: null }, ledgerKind: '', timer: null }

function setPreset(n) { S.preset = n; S.to = today(); S.from = addDays(S.to, -(n - 1)) }

async function api(path, opt = {}) {
  const o = { method: opt.method || 'GET', credentials: 'same-origin', headers: { 'X-Finance': '1' } }
  if (opt.body) { o.headers['Content-Type'] = 'application/json'; o.body = JSON.stringify(opt.body) }
  const r = await fetch('/finance/api' + path, o)
  if (r.status === 401 && path !== '/login') { renderLogin(); throw new Error('login') }
  const j = await r.json().catch(() => ({}))
  if (!r.ok) throw new Error(j.error || 'Something went wrong')
  return j
}

function toast(msg) {
  const t = $('#toast'); t.textContent = msg; t.hidden = false
  clearTimeout(toast.id); toast.id = setTimeout(() => { t.hidden = true }, 2800)
}

// ---------- numbers ----------
function totals(days) {
  const t = { stakes: 0, prizes: 0, gameRefunds: 0, entries: 0, deposits: 0, depositCount: 0, withdrawn: 0, withdrawCount: 0, signup: 0, referral: 0, cashback: 0, newUsers: 0, expenses: 0 }
  for (const d of days) for (const k in t) t[k] += d[k] || 0
  t.turnover = t.stakes - t.gameRefunds
  t.ggr = t.turnover - t.prizes // what the house kept from the games
  t.net = t.ggr - t.expenses
  t.margin = t.turnover ? (t.ggr / t.turnover) * 100 : 0
  return t
}
const dayGgr = (d) => d.stakes - d.gameRefunds - d.prizes

function delta(cur, prev, goodUp = true) {
  if (!S.prev) return ''
  if (!prev) return cur ? '<span class="delta flat">new</span>' : ''
  const p = ((cur - prev) / Math.abs(prev)) * 100
  if (Math.abs(p) < 0.5) return '<span class="delta flat">0%</span>'
  const up = p > 0, good = up === goodUp
  return `<span class="delta ${good ? 'up' : 'down'}">${up ? '▲' : '▼'} ${Math.abs(p).toFixed(p > 99 ? 0 : 1)}%</span>`
}

// ---------- charts (plain SVG) ----------
function niceTicks(min, max, count) {
  const raw = (max - min) / count
  const mag = Math.pow(10, Math.floor(Math.log10(raw)))
  const f = raw / mag
  const step = (f <= 1 ? 1 : f <= 2 ? 2 : f <= 5 ? 5 : 10) * mag
  const out = []
  for (let v = Math.floor(min / step) * step; v <= Math.ceil(max / step) * step + step / 2; v += step) out.push(v)
  return out
}

let chartId = 0
function drawChart(el, cfg) {
  if (!el) return
  const id = 'c' + ++chartId
  const W = Math.max(300, el.clientWidth), H = cfg.height || 280
  const pl = 48, pr = 10, pt = 12, pb = 28
  const n = cfg.labels.length
  let min = 0, max = 0
  for (const s of cfg.series) for (const v of s.values) { min = Math.min(min, v); max = Math.max(max, v) }
  if (max === min) max = min + 10000
  const ticks = niceTicks(min, max, 4)
  const lo = ticks[0], hi = ticks[ticks.length - 1]
  const iw = W - pl - pr, ih = H - pt - pb
  const y = (v) => pt + ih - ((v - lo) / (hi - lo)) * ih
  const bars = cfg.series.filter((s) => s.kind === 'bar')
  const band = iw / n
  const x = (i) => (bars.length ? pl + band * (i + 0.5) : n === 1 ? pl + iw / 2 : pl + (i * iw) / (n - 1))
  let svg = `<svg viewBox="0 0 ${W} ${H}" height="${H}" role="img" aria-label="${esc(cfg.title || 'chart')}"><defs>`
  cfg.series.forEach((s, i) => { svg += `<linearGradient id="${id}g${i}" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="${s.color}" stop-opacity=".38"/><stop offset="1" stop-color="${s.color}" stop-opacity="0"/></linearGradient>` })
  svg += '</defs>'
  for (const t of ticks) svg += `<line x1="${pl}" x2="${W - pr}" y1="${y(t)}" y2="${y(t)}" stroke="rgba(255,255,255,${t === 0 ? .28 : .07})"/><text x="${pl - 8}" y="${y(t) + 4}" text-anchor="end" fill="#6b7493" font-size="11">${compact(t)}</text>`
  const every = Math.max(1, Math.ceil(n / (W < 520 ? 4 : 8)))
  cfg.labels.forEach((l, i) => { if (i % every === 0 || i === n - 1 && n < 9) svg += `<text x="${x(i)}" y="${H - 8}" text-anchor="middle" fill="#6b7493" font-size="11">${shortDay(l)}</text>` })
  const bw = bars.length ? Math.max(2, (band * 0.72) / bars.length) : 0
  cfg.series.forEach((s, si) => {
    if (s.kind === 'bar') {
      const bi = bars.indexOf(s)
      s.values.forEach((v, i) => {
        const x0 = pl + band * i + band * 0.14 + bi * bw
        const y0 = y(Math.max(v, 0)), h = Math.max(1, Math.abs(y(v) - y(0)))
        svg += `<rect x="${x0}" y="${y0}" width="${Math.max(1, bw - 2)}" height="${h}" rx="3" fill="${s.color}" opacity=".92"/>`
      })
      return
    }
    const pts = s.values.map((v, i) => `${x(i).toFixed(1)},${y(v).toFixed(1)}`)
    if (s.kind === 'area') svg += `<path d="M${x(0)},${y(0)} L${pts.join(' L')} L${x(n - 1)},${y(0)} Z" fill="url(#${id}g${si})"/>`
    svg += `<polyline points="${pts.join(' ')}" fill="none" stroke="${s.color}" stroke-width="2.4" stroke-linejoin="round" stroke-linecap="round"/>`
  })
  svg += `<line class="cx" y1="${pt}" y2="${pt + ih}" stroke="rgba(255,255,255,.35)" stroke-dasharray="3 3" visibility="hidden"/><rect class="hit" x="${pl}" y="${pt}" width="${iw}" height="${ih}" fill="transparent"/></svg>`
  el.innerHTML = svg
  const cx = $('.cx', el), hit = $('.hit', el), tip = $('#tip')
  const idxAt = (clientX) => {
    const r = hit.getBoundingClientRect()
    const f = Math.min(1, Math.max(0, (clientX - r.left) / r.width))
    return bars.length ? Math.min(n - 1, Math.floor(f * n)) : Math.round(f * (n - 1))
  }
  hit.addEventListener('pointermove', (e) => {
    const i = idxAt(e.clientX)
    cx.setAttribute('x1', x(i)); cx.setAttribute('x2', x(i)); cx.setAttribute('visibility', 'visible')
    tip.innerHTML = `<b>${shortDay(cfg.labels[i])}</b>` + cfg.series.map((s) => `<div><span><i class="dot" style="background:${s.color}"></i> ${esc(s.name)}</span><span class="num">${fmt(s.values[i])}</span></div>`).join('')
    tip.hidden = false
    const tw = tip.offsetWidth
    tip.style.left = Math.min(window.innerWidth - tw - 8, e.clientX + 14) + 'px'
    tip.style.top = Math.max(8, e.clientY - 20 - tip.offsetHeight / 2) + 'px'
  })
  hit.addEventListener('pointerleave', () => { cx.setAttribute('visibility', 'hidden'); tip.hidden = true })
}

function donut(items) {
  const total = items.reduce((a, b) => a + b.value, 0)
  if (!total) return '<p class="empty">No bets in this period.</p>'
  const r = 54, c = 2 * Math.PI * r
  let off = 0
  const arcs = items.map((it, i) => {
    const len = (it.value / total) * c
    const s = `<circle cx="70" cy="70" r="${r}" fill="none" stroke="${COLORS[i % COLORS.length]}" stroke-width="16" stroke-dasharray="${len} ${c - len}" stroke-dashoffset="${-off}" transform="rotate(-90 70 70)"/>`
    off += len
    return s
  }).join('')
  const legend = items.map((it, i) => `<li><i class="dotb" style="background:${COLORS[i % COLORS.length]}"></i><span>${esc(it.label)}</span><span class="num">${esc(it.text)}</span></li>`).join('')
  return `<div class="donut"><svg viewBox="0 0 140 140"><circle cx="70" cy="70" r="${r}" fill="none" stroke="rgba(255,255,255,.07)" stroke-width="16"/>${arcs}<text x="70" y="66" text-anchor="middle" fill="#eef1fb" font-size="20" font-weight="800">${compact(total * 100)}</text><text x="70" y="84" text-anchor="middle" fill="#6b7493" font-size="10">TURNOVER</text></svg><ul>${legend}</ul></div>`
}

// ---------- shell ----------
function renderLogin(msg = '') {
  clearInterval(S.timer)
  $('#app').innerHTML = `
  <main class="login"><form class="box glass" id="lf" autocomplete="on">
    <div class="logo"><i>₿</i> ${esc(BRAND)} Finance</div>
    <h1>Owner login</h1><p class="sub">Only the owner can open this dashboard.</p>
    <label class="f" for="ph">Phone number</label>
    <input class="in" id="ph" name="username" inputmode="tel" autocomplete="username" placeholder="09XXXXXXXX" required>
    <label class="f" for="pw">Password</label>
    <div class="pw"><input class="in" id="pw" name="password" type="password" autocomplete="current-password" placeholder="••••••••••" required><button type="button" id="sh">Show</button></div>
    <div class="err" id="er" role="alert">${esc(msg)}</div>
    <button class="btn block" id="go">Sign in</button>
  </form></main>`
  $('#sh').onclick = () => { const p = $('#pw'); p.type = p.type === 'password' ? 'text' : 'password'; $('#sh').textContent = p.type === 'password' ? 'Show' : 'Hide' }
  $('#lf').onsubmit = async (e) => {
    e.preventDefault()
    const b = $('#go'); b.disabled = true; b.textContent = 'Signing in…'
    try {
      await api('/login', { method: 'POST', body: { phone: $('#ph').value, password: $('#pw').value } })
      await boot()
    } catch (err) {
      $('#er').textContent = err.message; b.disabled = false; b.textContent = 'Sign in'
    }
  }
  $('#ph').focus()
}

function renderShell() {
  $('#app').innerHTML = `
  <div class="shell">
    <aside class="side">
      <div class="logo"><i>₿</i> ${esc(BRAND)}</div>
      <nav class="nav" id="nav">${PAGES.map(([k, l]) => `<button data-p="${k}">${icon(k === 'cash' ? 'cash' : k)}<span>${l}</span>${k === 'cash' ? '<span class="badge" id="bd" hidden></span>' : ''}</button>`).join('')}</nav>
      <div class="foot"><b>Owner ${esc(S.owner)}</b><button class="btn ghost small" id="lo" style="width:100%">Log out</button></div>
    </aside>
    <main class="main"><header class="top" id="top"></header><div id="view"></div></main>
  </div>`
  $('#nav').onclick = (e) => { const b = e.target.closest('button'); if (b) go(b.dataset.p) }
  $('#lo').onclick = async () => { try { await api('/logout', { method: 'POST' }) } catch (_) { /* ignore */ } renderLogin() }
  window.addEventListener('resize', debounce(() => { if (S.cur) renderPage() }, 200))
}

const debounce = (fn, ms) => { let t; return (...a) => { clearTimeout(t); t = setTimeout(() => fn(...a), ms) } }

function renderTop() {
  const title = PAGES.find((p) => p[0] === S.page)[1]
  const showRange = S.page !== 'cash'
  $('#top').innerHTML = `<h2>${title}</h2><div class="grow"></div>` + (showRange ? `
    <div class="chips" id="chips">${[[7, '7 days'], [30, '30 days'], [90, '90 days'], [365, 'Year']].map(([n, l]) => `<button data-n="${n}" class="${S.preset === n ? 'on' : ''}">${l}</button>`).join('')}<button data-n="0" class="${S.preset === 0 ? 'on' : ''}">Custom</button></div>
    ${S.preset === 0 ? `<div class="dates"><input class="in" type="date" id="df" value="${S.from}" max="${today()}"> to <input class="in" type="date" id="dt" value="${S.to}" max="${today()}"></div>` : ''}` : '')
    + `<button class="btn ghost small" id="rf">↻ Refresh</button>`
  $('#rf').onclick = () => refresh(true)
  const ch = $('#chips')
  if (ch) ch.onclick = (e) => {
    const b = e.target.closest('button'); if (!b) return
    const n = Number(b.dataset.n)
    if (n) setPreset(n); else S.preset = 0
    refresh()
  }
  const df = $('#df'), dt = $('#dt')
  if (df) df.onchange = dt.onchange = () => { if (df.value && dt.value && df.value <= dt.value) { S.from = df.value; S.to = dt.value; refresh() } }
}

function go(p) { S.page = p; $$('#nav button').forEach((b) => b.classList.toggle('on', b.dataset.p === p)); renderTop(); renderPage(); window.scrollTo({ top: 0 }) }

async function refresh(manual) {
  const view = $('#view')
  if (!S.cur) view.innerHTML = '<div class="grid kpis">' + '<div class="card glass"><div class="skel" style="height:90px"></div></div>'.repeat(8) + '</div>'
  try {
    const len = span(S.from, S.to)
    const pFrom = addDays(S.from, -len), pTo = addDays(S.from, -1)
    const [cur, prev] = await Promise.all([api(`/summary?from=${S.from}&to=${S.to}`), api(`/summary?from=${pFrom}&to=${pTo}`).catch(() => null)])
    S.cur = cur; S.prev = prev; S.stamp = Date.now()
    const pend = cur.now.pendingDeposits.count + cur.now.pendingWithdrawals.count
    const bd = $('#bd'); if (bd) { bd.hidden = !pend; bd.textContent = pend }
    renderTop(); renderPage()
    if (manual) toast('Updated')
  } catch (e) { if (e.message !== 'login') view.innerHTML = `<div class="card glass empty">${esc(e.message)}</div>` }
}

function renderPage() {
  if (!S.cur) return
  const fn = { overview: viewOverview, revenue: viewRevenue, people: viewPeople, cash: viewCash, expenses: viewExpenses }[S.page]
  fn()
  const st = $('#stamp'); if (st) st.textContent = 'Updated ' + new Date(S.stamp).toLocaleTimeString('en-US', { timeZone: TZ, hour: '2-digit', minute: '2-digit' })
}

// ---------- overview ----------
function kpi(label, value, sub, ico, cls = '') {
  return `<div class="card glass kpi ${cls}"><h3>${label}<span class="ico">${ico}</span></h3><div class="v num">${value}</div><div class="s">${sub}</div></div>`
}

function viewOverview() {
  const d = S.cur, t = totals(d.days), p = S.prev ? totals(S.prev.days) : null
  const pend = d.now.pendingDeposits.count + d.now.pendingWithdrawals.count
  const cash = d.now.depositsAllTime - d.now.withdrawnAllTime - d.now.expensesAllTime
  const owed = d.now.mainWallets + d.now.pendingWithdrawals.amount
  $('#view').innerHTML = `
  ${pend ? `<div class="alert">⏳ ${d.now.pendingDeposits.count} deposit(s) and ${d.now.pendingWithdrawals.count} withdrawal(s) are waiting for you in the Admin panel.</div>` : ''}
  <div class="grid kpis">
    ${kpi('Net profit', `<span class="${sign(t.net)}">${fmt0(t.net)}</span><small>ETB</small>`, `${p ? delta(t.net, p.net) : ''}<span>after ${fmt0(t.expenses)} expenses</span>`, '💎', 'hero')}
    ${kpi('House revenue', `${fmt0(t.ggr)}<small>ETB</small>`, `${p ? delta(t.ggr, p.ggr) : ''}<span>${t.margin.toFixed(1)}% of bets kept</span>`, '🏦')}
    ${kpi('Total bets', `${fmt0(t.turnover)}<small>ETB</small>`, `${p ? delta(t.turnover, p.turnover) : ''}<span>${int(t.entries)} cartelas</span>`, '🎟️')}
    ${kpi('Prizes paid', `${fmt0(t.prizes)}<small>ETB</small>`, `${p ? delta(t.prizes, p.prizes, false) : ''}<span>to winners</span>`, '🏆')}
    ${kpi('Deposits', `${fmt0(t.deposits)}<small>ETB</small>`, `${p ? delta(t.deposits, p.deposits) : ''}<span>${int(t.depositCount)} deposits</span>`, '📥')}
    ${kpi('Withdrawals paid', `${fmt0(t.withdrawn)}<small>ETB</small>`, `${p ? delta(t.withdrawn, p.withdrawn, false) : ''}<span>${int(t.withdrawCount)} payouts</span>`, '📤')}
    ${kpi('New players', int(t.newUsers), `${p ? delta(t.newUsers, p.newUsers) : ''}<span>${int(d.now.users)} total · ${int(d.now.online)} online</span>`, '👥')}
    ${kpi('Cash vs. owed', `<span class="${sign(cash - owed)}">${fmt0(cash - owed)}</span><small>ETB</small>`, `<span>cash ${compact(cash)} − owed ${compact(owed)}</span>`, '⚖️')}
  </div>
  <div class="grid two" style="margin-top:16px">
    <section class="card glass"><h3>Revenue &amp; profit <small id="stamp"></small></h3><div class="chart" id="ch1"></div>
      <div class="legend"><span><i style="background:#2ee6a6"></i>House revenue</span><span><i style="background:#8b7bff"></i>Net profit</span></div></section>
    <section class="card glass"><h3>Bets by stake</h3><div id="dn"></div>
      <p class="note">House keeps the difference between bets and prizes. Bonus credits are not counted as a separate cost: their cost shows up when they win prizes.</p></section>
  </div>
  <div class="grid two eq" style="margin-top:16px">
    <section class="card glass"><h3>Deposits vs. withdrawals</h3><div class="chart" id="ch2"></div>
      <div class="legend"><span><i style="background:#4dabff"></i>Deposits</span><span><i style="background:#ffc53d"></i>Withdrawals paid</span></div></section>
    <div class="grid" style="gap:16px">
      <section class="card glass"><h3>Top winners</h3>${topList(d.topWinners)}</section>
      <section class="card glass"><h3>Top depositors</h3>${topList(d.topDepositors)}</section>
    </div>
  </div>`
  const labels = d.days.map((x) => x.day)
  drawChart($('#ch1'), { labels, title: 'Revenue and profit', series: [
    { name: 'House revenue', color: '#2ee6a6', kind: 'area', values: d.days.map(dayGgr) },
    { name: 'Net profit', color: '#8b7bff', kind: 'line', values: d.days.map((x) => dayGgr(x) - x.expenses) }] })
  drawChart($('#ch2'), { labels, title: 'Deposits and withdrawals', height: 360, series: [
    { name: 'Deposits', color: '#4dabff', kind: 'bar', values: d.days.map((x) => x.deposits) },
    { name: 'Withdrawals paid', color: '#ffc53d', kind: 'bar', values: d.days.map((x) => x.withdrawn) }] })
  $('#dn').innerHTML = donut(d.byStake.map((s) => ({ label: `Play ${fmt0(s.fee)} · ${int(s.entries)}`, value: s.stakes / 100, text: fmt0(s.stakes) })))
}

function topList(rows) {
  if (!rows || !rows.length) return '<p class="empty">Nothing yet.</p>'
  return '<div class="list">' + rows.map((r) => `<div class="row">${avatar(r.name, r.id)}<div>${esc(r.name || 'Player ' + r.id)}<small>${int(r.count)}×</small></div><span class="amt num">${fmt(r.amount)}</span></div>`).join('') + '</div>'
}

// ---------- revenue ----------
const REV_COLS = [['Day', null], ['Cartelas', 'entries'], ['Bets', 'turn'], ['Prizes', 'prizes'], ['Refunds', 'gameRefunds'], ['House revenue', 'ggr'], ['Expenses', 'expenses'], ['Net', 'net'], ['Deposits', 'deposits'], ['Withdrawn', 'withdrawn'], ['Bonus credits', 'promo']]
function revRow(d) {
  const ggr = dayGgr(d)
  return { day: d.day, entries: d.entries, turn: d.stakes - d.gameRefunds, prizes: d.prizes, gameRefunds: d.gameRefunds, ggr, expenses: d.expenses, net: ggr - d.expenses, deposits: d.deposits, withdrawn: d.withdrawn, promo: d.signup + d.referral + d.cashback }
}

function viewRevenue() {
  const d = S.cur, t = totals(d.days)
  const rows = d.days.map(revRow).reverse()
  const best = rows.reduce((a, b) => (b.ggr > a.ggr ? b : a), rows[0])
  const avg = t.ggr / d.days.length
  const sum = revRow({ day: 'Total', stakes: t.stakes, gameRefunds: t.gameRefunds, prizes: t.prizes, entries: t.entries, expenses: t.expenses, deposits: t.deposits, withdrawn: t.withdrawn, signup: t.signup, referral: t.referral, cashback: t.cashback })
  const cell = (k, r) => (k === 'entries' ? int(r[k]) : k === 'net' || k === 'ggr' ? `<span class="${sign(r[k])}">${fmt(r[k])}</span>` : fmt(r[k]))
  $('#view').innerHTML = `
  <div class="grid three">
    ${kpi('House revenue', `${fmt0(t.ggr)}<small>ETB</small>`, `<span>${t.margin.toFixed(1)}% margin</span>`, '🏦')}
    ${kpi('Average per day', `${fmt0(avg)}<small>ETB</small>`, `<span>${d.days.length} days</span>`, '📈')}
    ${kpi('Best day', `${fmt0(best.ggr)}<small>ETB</small>`, `<span>${shortDay(best.day)}</span>`, '⭐')}
  </div>
  <section class="card glass" style="margin-top:16px"><h3>Daily breakdown <small>${shortDay(d.from)} – ${shortDay(d.to)}</small></h3>
    <div class="toolbar" style="margin-top:8px"><div class="grow" style="flex:1"></div><button class="btn ghost small" id="csv">⬇ Export CSV</button></div>
    <div class="tablewrap"><table><thead><tr>${REV_COLS.map(([l]) => `<th>${l}</th>`).join('')}</tr></thead>
      <tbody>${rows.map((r) => `<tr><td>${shortDay(r.day)}</td>${REV_COLS.slice(1).map(([, k]) => `<td class="num">${cell(k, r)}</td>`).join('')}</tr>`).join('')}</tbody>
      <tfoot><tr><td>Total</td>${REV_COLS.slice(1).map(([, k]) => `<td class="num">${cell(k, sum)}</td>`).join('')}</tr></tfoot></table></div>
    <p class="note">House revenue = bets − given-back bets − prizes. Net = house revenue − expenses. Bonus credits (signup, invites, cashback) are play money; their cost appears when they win prizes.</p>
  </section>
  <section class="card glass" style="margin-top:16px"><h3>Money activity</h3>
    <div class="toolbar" style="margin-top:8px"><select class="in" id="lk"><option value="">All activity</option>${Object.entries(KINDS).map(([k, v]) => `<option value="${k}" ${S.ledgerKind === k ? 'selected' : ''}>${v[0]}</option>`).join('')}</select></div>
    <div id="led"><div class="skel" style="height:120px;margin-top:12px"></div></div></section>`
  $('#csv').onclick = () => csv(`revenue_${d.from}_${d.to}.csv`, [REV_COLS.map((c) => c[0]), ...rows.map((r) => REV_COLS.map(([l, k]) => (k === null ? r.day : k === 'entries' ? r[k] : (r[k] / 100).toFixed(2))))])
  $('#lk').onchange = (e) => { S.ledgerKind = e.target.value; loadLedger() }
  loadLedger()
}

async function loadLedger() {
  const box = $('#led'); if (!box) return
  try {
    const j = await api('/ledger?limit=60' + (S.ledgerKind ? '&kind=' + S.ledgerKind : ''))
    box.innerHTML = j.rows.length ? '<div class="list">' + j.rows.map((r) => `<div class="row">${avatar(r.name, r.uid)}<div>${esc(r.name || 'Player ' + r.uid)}<small>${when(r.at)}</small></div>${kindChip(r.kind)}<span class="amt num ${sign(r.amount)}">${r.amount > 0 ? '+' : ''}${fmt(r.amount)}</span></div>`).join('') + '</div>' : '<p class="empty">No activity.</p>'
  } catch (e) { if (e.message !== 'login') box.innerHTML = `<p class="empty">${esc(e.message)}</p>` }
}

// ---------- people ----------
const SORTS = [['joined', 'Newest'], ['balance', 'Highest prize balance'], ['bonus', 'Highest play wallet'], ['deposits', 'Most deposited'], ['withdrawn', 'Most withdrawn'], ['stakes', 'Most bets'], ['prizes', 'Most won'], ['houseProfit', 'Most profitable for you']]

function viewPeople() {
  const P = S.people
  $('#view').innerHTML = `
  <section class="card glass"><div class="toolbar">
    <input class="in" id="q" type="search" placeholder="Search name, phone or Telegram ID" value="${esc(P.q)}">
    <select class="in" id="so">${SORTS.map(([k, l]) => `<option value="${k}" ${P.sort === k ? 'selected' : ''}>${l}</option>`).join('')}</select>
    <button class="btn ghost small" id="od">${P.order === 'desc' ? '↓ High to low' : '↑ Low to high'}</button>
    <button class="btn ghost small" id="ex">⬇ Export all</button></div>
    <div id="pt"><div class="skel" style="height:240px;margin-top:14px"></div></div></section>`
  $('#q').oninput = debounce((e) => { P.q = e.target.value; P.page = 1; loadPeople() }, 300)
  $('#so').onchange = (e) => { P.sort = e.target.value; P.page = 1; loadPeople() }
  $('#od').onclick = () => { P.order = P.order === 'desc' ? 'asc' : 'desc'; viewPeople() }
  $('#ex').onclick = exportPeople
  loadPeople()
}

const peopleQS = (P, page, size) => `/people?q=${encodeURIComponent(P.q)}&sort=${P.sort}&order=${P.order}&page=${page}&size=${size}`

async function loadPeople() {
  const P = S.people, box = $('#pt'); if (!box) return
  try {
    const j = await api(peopleQS(P, P.page, 25)); P.data = j
    const pages = Math.max(1, Math.ceil(j.total / j.size))
    box.innerHTML = `
    <div class="tablewrap"><table><thead><tr><th>Player</th><th>Phone</th><th>Prize wallet</th><th>Play wallet</th><th>Deposited</th><th>Withdrawn</th><th>Bets</th><th>Prizes</th><th>Your profit</th></tr></thead>
    <tbody>${j.rows.map((r) => `<tr class="click" data-id="${r.id}"><td><div class="who">${avatar(r.name, r.id)}<div>${esc(r.name || 'Player')}<small>ID ${r.id} · ${joinDate(r.joined)}</small></div></div></td>
      <td>${r.phone ? esc(r.phone) : '<span class="chip">not registered</span>'}</td><td class="num">${fmt(r.balance)}</td><td class="num">${fmt(r.bonus)}</td><td class="num">${fmt(r.deposits)}</td>
      <td class="num">${fmt(r.withdrawn)}</td><td class="num">${fmt(r.stakes)}</td><td class="num">${fmt(r.prizes)}</td><td class="num ${sign(r.houseProfit)}">${fmt(r.houseProfit)}</td></tr>`).join('') || '<tr><td colspan="9" class="empty">No players found.</td></tr>'}</tbody>
    <tfoot><tr><td>All ${int(j.total)} players</td><td></td><td class="num">${fmt(j.sums.balance)}</td><td class="num">${fmt(j.sums.bonus)}</td><td class="num">${fmt(j.sums.deposits)}</td><td class="num">${fmt(j.sums.withdrawn)}</td><td class="num">${fmt(j.sums.stakes)}</td><td class="num">${fmt(j.sums.prizes)}</td><td class="num ${sign(j.sums.houseProfit)}">${fmt(j.sums.houseProfit)}</td></tr></tfoot></table></div>
    <div class="pager"><button class="btn ghost small" id="pv" ${j.page <= 1 ? 'disabled' : ''}>← Prev</button><span>Page ${j.page} / ${pages}</span><button class="btn ghost small" id="nx" ${j.page >= pages ? 'disabled' : ''}>Next →</button></div>`
    $('#pv').onclick = () => { P.page--; loadPeople() }
    $('#nx').onclick = () => { P.page++; loadPeople() }
    $$('tr.click', box).forEach((tr) => { tr.onclick = () => openPerson(tr.dataset.id) })
  } catch (e) { if (e.message !== 'login') box.innerHTML = `<p class="empty">${esc(e.message)}</p>` }
}

async function exportPeople() {
  toast('Preparing…')
  const rows = [['ID', 'Name', 'Phone', 'Prize wallet', 'Play wallet', 'Deposited', 'Withdrawn', 'Bets', 'Prizes', 'Your profit', 'Cartelas', 'Invited', 'Joined']]
  try {
    for (let page = 1; page <= 100; page++) {
      const j = await api(peopleQS(S.people, page, 100))
      for (const r of j.rows) rows.push([r.id, r.name, r.phone, r.balance / 100, r.bonus / 100, r.deposits / 100, r.withdrawn / 100, r.stakes / 100, r.prizes / 100, r.houseProfit / 100, r.entries, r.refs, new Date(r.joined).toISOString().slice(0, 10)])
      if (page * j.size >= j.total) break
    }
    csv('players.csv', rows)
  } catch (e) { if (e.message !== 'login') toast(e.message) }
}

async function openPerson(id) {
  const shade = document.createElement('div'); shade.className = 'shade'
  const dr = document.createElement('aside'); dr.className = 'drawer'; dr.setAttribute('role', 'dialog')
  dr.innerHTML = '<div class="skel" style="height:200px"></div>'
  const close = () => { shade.remove(); dr.remove() }
  shade.onclick = close
  document.body.append(shade, dr)
  try {
    const j = await api('/people/' + id), u = j.user
    dr.innerHTML = `
    <div class="hd">${avatar(u.name, u.id)}<div><h3>${esc(u.name || 'Player')}</h3><span class="mut">${u.phone ? esc(u.phone) : 'not registered'} · ID ${u.id}</span></div><button class="x" aria-label="Close">✕</button></div>
    <div class="mini">
      <div><small>Prize wallet</small><b class="num">${fmt(u.balance)}</b></div><div><small>Play wallet</small><b class="num">${fmt(u.bonus)}</b></div>
      <div><small>Deposited</small><b class="num">${fmt(u.deposits)}</b></div><div><small>Withdrawn</small><b class="num">${fmt(u.withdrawn)}</b></div>
      <div><small>Total bets</small><b class="num">${fmt(u.stakes)}</b></div><div><small>Prizes won</small><b class="num">${fmt(u.prizes)}</b></div>
      <div><small>Your profit</small><b class="num ${sign(u.houseProfit)}">${fmt(u.houseProfit)}</b></div><div><small>Joined</small><b>${joinDate(u.joined)}</b></div>
      <div><small>Cartelas · wins</small><b class="num">${int(u.entries)} · ${int(u.gamesWon)}</b></div><div><small>Invited · paying</small><b class="num">${int(u.refs)} · ${int(u.refQualified)}</b></div>
    </div>
    <h4 class="sec">Deposits</h4>${payList(j.deposits, 'deposit')}
    <h4 class="sec">Withdrawals</h4>${payList(j.withdrawals, 'withdraw')}
    <h4 class="sec">Latest activity</h4><div class="list">${(j.txs || []).map((r) => `<div class="row"><div>${kindChip(r.kind)}<small>${when(r.at)}</small></div><span class="amt num ${sign(r.amount)}">${r.amount > 0 ? '+' : ''}${fmt(r.amount)}</span></div>`).join('') || '<p class="empty">No activity.</p>'}</div>`
    $('.x', dr).onclick = close
  } catch (e) { close(); if (e.message !== 'login') toast(e.message) }
}

const statusChip = (s) => `<span class="chip ${s === 'approved' || s === 'paid' ? 'g' : s === 'rejected' ? 'r' : 'a'}">${esc(s)}</span>`
function payList(rows, kind) {
  if (!rows || !rows.length) return '<p class="mut" style="font-size:14px">None.</p>'
  return '<div class="list">' + rows.map((r) => `<div class="row"><div>${statusChip(r.status)}<small>${when(r.createdAt)}${r.phone ? ' · ' + esc(r.phone) : ''}${r.txnId ? ' · ' + esc(r.txnId) : ''}</small></div><span class="amt num">${fmt(r.amount)}</span></div>`).join('') + '</div>'
}

// ---------- cash & wallets ----------
function viewCash() {
  const n = S.cur.now
  const cash = n.depositsAllTime - n.withdrawnAllTime - n.expensesAllTime
  const owed = n.mainWallets + n.pendingWithdrawals.amount
  const net = cash - owed
  $('#view').innerHTML = `
  <div class="grid kpis">
    ${kpi('Cash in hand (estimate)', `${fmt0(cash)}<small>ETB</small>`, '<span>deposits − payouts − expenses</span>', '💵', 'hero')}
    ${kpi('Owed to players', `${fmt0(owed)}<small>ETB</small>`, `<span>${compact(n.mainWallets)} prize wallets + ${compact(n.pendingWithdrawals.amount)} requested</span>`, '🧾')}
    ${kpi('Safe to keep', `<span class="${sign(net)}">${fmt0(net)}</span><small>ETB</small>`, `<span>${net >= 0 ? 'cash covers every prize balance' : 'cash is lower than what players can withdraw'}</span>`, net >= 0 ? '✅' : '⚠️')}
    ${kpi('Play credits out', `${fmt0(n.playWallets)}<small>ETB</small>`, `<span>${fmt0(n.cashbackOwed)} cashback still to pay</span>`, '🎮')}
  </div>
  <div class="grid two eq" style="margin-top:16px">
    <section class="card glass"><h3>Waiting deposits <small>${int(n.pendingDeposits.count)} · ${fmt(n.pendingDeposits.amount)}</small></h3><div id="pdp"><div class="skel" style="height:90px;margin-top:12px"></div></div></section>
    <section class="card glass"><h3>Waiting withdrawals <small>${int(n.pendingWithdrawals.count)} · ${fmt(n.pendingWithdrawals.amount)}</small></h3><div id="pwd"><div class="skel" style="height:90px;margin-top:12px"></div></div></section>
  </div>
  <p class="note">Approve deposits and pay withdrawals in your Admin panel; this dashboard only shows the numbers. "Cash in hand" assumes every approved deposit reached your Telebirr and every paid withdrawal left it.</p>`
  api('/pending').then((j) => {
    const lst = (rows, k) => rows && rows.length ? '<div class="list">' + rows.map((r) => `<div class="row">${avatar(r.name, r.uid)}<div>${esc(r.name || 'Player ' + r.uid)}<small>${when(r.createdAt)}${r.phone ? ' · ' + esc(r.phone) : ''}${r.txnId ? ' · ' + esc(r.txnId) : ''}</small></div><span class="amt num">${fmt(r.amount)}</span></div>`).join('') + '</div>' : '<p class="empty">Nothing waiting 🎉</p>'
    $('#pdp').innerHTML = lst(j.deposits); $('#pwd').innerHTML = lst(j.withdrawals)
  }).catch((e) => { if (e.message !== 'login') toast(e.message) })
}

// ---------- expenses ----------
const CATS = ['Verify.ET credits', 'Hosting', 'Marketing', 'Telebirr fees', 'Salaries', 'Other']
async function viewExpenses() {
  const d = S.cur
  $('#view').innerHTML = `
  <section class="card glass"><h3>Add an expense</h3>
    <form class="form" id="ef" style="margin-top:12px">
      <div><label class="f" for="ed">Date</label><input class="in" id="ed" type="date" value="${today()}" max="${today()}" required></div>
      <div><label class="f" for="ec">Category</label><input class="in" id="ec" list="cats" placeholder="Hosting" maxlength="40" required><datalist id="cats">${CATS.map((c) => `<option value="${c}">`).join('')}</datalist></div>
      <div><label class="f" for="en">Note (optional)</label><input class="in" id="en" maxlength="200" placeholder="What was it for?"></div>
      <div><label class="f" for="ea">Amount (ETB)</label><input class="in" id="ea" type="number" min="0.01" step="0.01" inputmode="decimal" required></div>
      <button class="btn">Add</button></form></section>
  <div class="grid two eq" style="margin-top:16px">
    <section class="card glass"><h3>By category <small>${shortDay(d.from)} – ${shortDay(d.to)}</small></h3><div id="cb"></div></section>
    <section class="card glass"><h3>Expenses</h3><div id="el"><div class="skel" style="height:120px;margin-top:12px"></div></div></section>
  </div>`
  $('#ef').onsubmit = async (e) => {
    e.preventDefault()
    try {
      await api('/expenses', { method: 'POST', body: { date: $('#ed').value, category: $('#ec').value, note: $('#en').value, amount: Number($('#ea').value) } })
      toast('Expense added'); await refresh()
    } catch (err) { if (err.message !== 'login') toast(err.message) }
  }
  try {
    const j = await api(`/expenses?from=${S.from}&to=${S.to}`)
    const cats = {}; let sum = 0
    for (const r of j.rows) { cats[r.category] = (cats[r.category] || 0) + r.amount; sum += r.amount }
    const top = Math.max(1, ...Object.values(cats))
    $('#cb').innerHTML = Object.keys(cats).length ? '<div class="bars">' + Object.entries(cats).sort((a, b) => b[1] - a[1]).map(([c, v]) => `<div class="bar"><span>${esc(c)}</span><div class="track"><div class="fill" style="width:${(v / top) * 100}%"></div></div><b class="num">${fmt(v)}</b></div>`).join('') + `</div><p class="note">Total ${fmt(sum)} ETB</p>` : '<p class="empty">No expenses in this period.</p>'
    $('#el').innerHTML = j.rows.length ? '<div class="list">' + j.rows.map((r) => `<div class="row"><div>${esc(r.category)}<small>${shortDay(r.date)}${r.note ? ' · ' + esc(r.note) : ''}</small></div><span class="amt num">${fmt(r.amount)}</span><button class="xbtn" data-id="${esc(r.id)}" aria-label="Delete expense">✕</button></div>`).join('') + '</div>' : '<p class="empty">Nothing here yet.</p>'
    $$('.xbtn', $('#el')).forEach((b) => { b.onclick = async () => { if (!confirm('Delete this expense?')) return; try { await api('/expenses/' + b.dataset.id, { method: 'DELETE' }); toast('Deleted'); await refresh() } catch (err) { if (err.message !== 'login') toast(err.message) } } })
  } catch (e) { if (e.message !== 'login') $('#el').innerHTML = `<p class="empty">${esc(e.message)}</p>` }
}

// ---------- csv ----------
function csv(name, rows) {
  const body = rows.map((r) => r.map((v) => { const s = String(v ?? ''); return /[",\n]/.test(s) || /^[=+\-@]/.test(s) ? '"' + s.replace(/"/g, '""').replace(/^([=+\-@])/, "'$1") + '"' : s }).join(',')).join('\n')
  const a = document.createElement('a')
  a.href = URL.createObjectURL(new Blob(['\ufeff' + body], { type: 'text/csv;charset=utf-8' }))
  a.download = name; document.body.append(a); a.click(); a.remove()
  setTimeout(() => URL.revokeObjectURL(a.href), 2000)
}

// ---------- start ----------
async function boot() {
  try {
    const me = await api('/me')
    S.owner = me.phone
  } catch (e) { return }
  setPreset(30)
  renderShell()
  go('overview')
  await refresh()
  clearInterval(S.timer)
  S.timer = setInterval(() => { if (!document.hidden && S.page !== 'expenses') refresh() }, 60000)
}
boot()
