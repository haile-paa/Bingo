// frontend/src/App.jsx
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import Admin from './Admin.jsx'

const tg = window.Telegram?.WebApp
const AUTH = tg?.initData ? `tma ${tg.initData}` : `dev ${new URLSearchParams(location.search).get('dev') || '1'}`
const birr = (s) => (s / 100).toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
const letterOf = (n) => 'BINGO'[Math.floor((n - 1) / 15)]
const LETTER_BG = ['bg-[#3b82f6]', 'bg-[#6366f1]', 'bg-[#a855f7]', 'bg-[#10b981]', 'bg-[#f97316]']
const ballBg = (n) => LETTER_BG[Math.floor((n - 1) / 15)]

const DICT = {
  am: {
    game: 'ጨዋታ', history: 'ታሪክ', wallet: 'ዋሌት', profile: 'መገለጫ',
    prize: 'ደራሽ', birr: 'ብር', main: 'ዋና ዋሌት', bonus: 'የጨዋታ ዋሌት',
    round: 'ዙር', time: 'ሰዓት', price: 'ዋጋ', mineCount: 'የእኔ', called: 'የተጠሩ', cartelas: 'ካርቴላ',
    pickHint: 'አንድ ካርቴላ {fee} ብር ነው። ከአንድ በላይ መያዝ ይችላሉ። እንደገና ሲነኩ ይሰረዛል።',
    legendMine: 'የእርስዎ', legendTaken: 'የተያዘ', myCards: 'የእኔ ካርቴላዎች', noCards: 'እስካሁን ካርቴላ አልመረጡም።',
    need: 'ጨዋታው ለመጀመር ቢያንስ {min} ካርቴላ ያስፈልጋል። አሁን {n} ተይዟል።',
    ready: 'ተዘጋጁ…', watching: 'ተመልካች ብቻ',
    watchingMsg: 'የዚህ ዙር ጨዋታ ተጀምሯል። አዲስ ዙር እስኪጀምር እዚሁ ይጠብቁ።',
    bingo: 'ቢንጎ!', youWon: 'እርስዎ አሸነፉ! 🎉', winCartela: 'አሸናፊ ካርቴላ',
    noWinner: 'አሸናፊ የለም', refunded: 'ክፍያዎ ወደ ዋሌትዎ ተመልሷል።', nextIn: 'ቀጣይ ጨዋታ በ{n} ሰከንድ ይጀምራል',
    rules: 'ደንቦች',
    r1: 'ካርቴላ ለመምረጥ {pick} ሰከንድ አለዎት። እያንዳንዱ ካርቴላ {fee} ብር ነው።',
    r2: 'ከዚያ በ{run} ሰከንድ ውስጥ ቁጥሮች ይጠራሉ። የተጠራ ቁጥር በካርቴላዎ ላይ አረንጓዴ ይሆናል።',
    r3: 'መስመር (አግድም፣ ቁልቁል ወይም ሰያፍ) የሞላ የመጀመሪያው ካርቴላ ያሸንፋል።',
    r4: '{total} ካርቴላ ሙሉ ሲሸጥ ሽልማቱ {full} ብር ነው። ባነሰ ካርቴላ ሽልማቱ በተመጣጣኝ ይሰላል።',
    r5: 'ሁለት ካርቴላ በአንድ ጊዜ ቢያሸንፉ ሽልማቱ እኩል ይካፈላል። ማንም ካላሸነፈ ክፍያ ይመለሳል።',
    err_closed: 'መምረጫ ጊዜው አልፏል። ቀጣዩን ዙር ይጠብቁ።', err_taken: 'ይህ ካርቴላ ተይዟል።',
    err_funds: 'በቂ ሂሳብ የለዎትም። በቦቱ ውስጥ ገንዘብ ያስገቡ።',
    err_register: 'መጀመሪያ በቦቱ ውስጥ «ተመዝገብ» ን ተጭነው ይመዝገቡ።',
    err_limit: 'አንድ ሰው እስከ {max} ካርቴላ ብቻ መያዝ ይችላል።', err_server: 'ችግር ተፈጥሯል። እንደገና ይሞክሩ።',
    depositTitle: 'ገንዘብ ማስገባት',
    depositBody: 'ማስገባት በቦቱ ውስጥ ብቻ ነው። «ገንዘብ አስገባ» ን ተጭነው ከ50 ብር ጀምሮ በቴሌብር ይላኩ፣ ከዚያ የደረሰዎትን SMS ለቦቱ ይላኩ።',
    toBot: 'ወደ ቦቱ ሂድ', withdrawTitle: 'ገንዘብ ማውጣት', tele: 'የቴሌብር ስልክ ቁጥር',
    amount: 'መጠን (ብር፣ ዝቅተኛ {min})', send: 'ጥያቄ ላክ', sent: 'ጥያቄዎ ተልኳል። አድሚን በቴሌብር ይልካል።',
    withdrawable: 'የሚወጣ', playOnly: 'ለጨዋታ ብቻ', noTx: 'እስካሁን እንቅስቃሴ የለም።',
    wins: 'ያሸነፉ ጨዋታዎች', invites: 'የጋበዟቸው', earned: 'ጠቅላላ ገቢ', language: 'ቋንቋ',
    inviteTitle: 'ጓደኞችን ይጋብዙ', inviteBody: 'ጓደኛዎ ሲመዘገብ 10 ብር ያገኛል። የመጀመሪያ ገቢው ሲጸድቅ እርስዎ 10 ብር ያገኛሉ።',
    copy: 'ኮፒ', copied: 'ተቀድቷል', shareBtn: 'አጋራ', shareText: 'ከእኔ ጋር Bingo ይጫወቱ! ሲመዘገቡ 10 ብር ያገኛሉ።',
    connecting: 'ከጨዋታው ጋር በመገናኘት ላይ…', reconnecting: 'እንደገና በመገናኘት ላይ…', loading: 'በመጫን ላይ…',
    voiceOn: '🔊 ድምፅ', voiceOff: '🔇 ድምፅ',
    k_signup_bonus: 'የምዝገባ ስጦታ', k_deposit: 'ገቢ', k_referral: 'የግብዣ ሽልማት', k_entry: 'የካርቴላ ክፍያ',
    k_win: 'ሽልማት', k_withdraw: 'ወጪ', k_refund: 'ተመላሽ',
  },
  en: {
    game: 'Game', history: 'History', wallet: 'Wallet', profile: 'Profile',
    prize: 'Prize', birr: 'birr', main: 'Main wallet', bonus: 'Play wallet',
    round: 'Round', time: 'Time', price: 'Price', mineCount: 'Mine', called: 'Called', cartelas: 'cartelas',
    pickHint: 'One cartela costs {fee} birr. You can hold several. Tap again to release.',
    legendMine: 'Yours', legendTaken: 'Taken', myCards: 'My cartelas', noCards: 'You have not picked a cartela yet.',
    need: 'At least {min} cartelas are needed to start. {n} taken so far.',
    ready: 'Get ready…', watching: 'Watching only',
    watchingMsg: 'This round has started. Wait here for the next one.',
    bingo: 'BINGO!', youWon: 'You won! 🎉', winCartela: 'Winning cartela',
    noWinner: 'No winner', refunded: 'Your payment was returned to your wallet.', nextIn: 'Next game starts in {n}s',
    rules: 'Rules',
    r1: 'You have {pick} seconds to pick cartelas. Each costs {fee} birr.',
    r2: 'Then numbers are called for {run} seconds. Called numbers turn green on your cartela.',
    r3: 'The first cartela to complete a line (row, column or diagonal) wins.',
    r4: 'With all {total} cartelas sold the prize is {full} birr. With fewer, it scales down proportionally.',
    r5: 'If several cartelas win together the prize is split equally. If nobody wins, stakes are refunded.',
    err_closed: 'Picking is closed. Wait for the next round.', err_taken: 'That cartela is taken.',
    err_funds: 'Not enough balance. Deposit in the bot.',
    err_register: 'Register in the bot first (tap Register).',
    err_limit: 'You can hold at most {max} cartelas.', err_server: 'Something went wrong. Try again.',
    depositTitle: 'Deposit',
    depositBody: 'Deposits happen in the bot only. Tap Deposit, send at least 50 birr by Telebirr, then paste the SMS to the bot.',
    toBot: 'Go to the bot', withdrawTitle: 'Withdraw', tele: 'Telebirr phone number',
    amount: 'Amount (birr, min {min})', send: 'Send request', sent: 'Request sent. An admin will pay you by Telebirr.',
    withdrawable: 'Withdrawable', playOnly: 'Play only', noTx: 'No activity yet.',
    wins: 'Games won', invites: 'Invited', earned: 'Total earned', language: 'Language',
    inviteTitle: 'Invite friends', inviteBody: 'Your friend gets 10 birr when they join. You get 10 birr when their first deposit is approved.',
    copy: 'Copy', copied: 'Copied', shareBtn: 'Share', shareText: 'Play Bingo with me! You get 10 birr when you join.',
    connecting: 'Connecting to the game…', reconnecting: 'Reconnecting…', loading: 'Loading…',
    voiceOn: '🔊 Voice', voiceOff: '🔇 Voice',
    k_signup_bonus: 'Welcome bonus', k_deposit: 'Deposit', k_referral: 'Invite reward', k_entry: 'Cartela payment',
    k_win: 'Prize won', k_withdraw: 'Withdrawal', k_refund: 'Refund',
  },
}

const LangCtx = createContext(null)
const useL = () => useContext(LangCtx)

async function api(path, opts = {}) {
  const res = await fetch(path, { ...opts, headers: { 'Content-Type': 'application/json', Authorization: AUTH } })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(data.error || 'Something went wrong')
  return data
}

function speak(n) {
  if (!('speechSynthesis' in window)) return
  speechSynthesis.cancel()
  const u = new SpeechSynthesisUtterance(`${letterOf(n)}. ${n}`)
  u.rate = 0.85
  speechSynthesis.speak(u)
}

function useNow() {
  const [now, setNow] = useState(Date.now())
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(id)
  }, [])
  return now
}

function useGame(voiceOn) {
  const [g, setG] = useState(null)
  const [conn, setConn] = useState(false)
  const [err, setErr] = useState('')
  const ws = useRef(null)
  const voice = useRef(voiceOn)
  voice.current = voiceOn

  useEffect(() => {
    let timer
    let closed = false
    const open = () => {
      const proto = location.protocol === 'https:' ? 'wss' : 'ws'
      const sock = new WebSocket(`${proto}://${location.host}/ws?a=${encodeURIComponent(AUTH)}`)
      ws.current = sock
      sock.onopen = () => setConn(true)
      sock.onclose = () => {
        setConn(false)
        if (!closed) timer = setTimeout(open, 1500)
      }
      sock.onmessage = (e) => {
        const m = JSON.parse(e.data)
        if (m.type === 'state') setG(m)
        else if (m.type === 'mine') setG((p) => p && { ...p, mine: m.mine, cards: m.cards })
        else if (m.type === 'spot') {
          setG((p) => p && {
            ...p,
            prize: m.prize,
            taken: m.on ? [...new Set([...p.taken, m.n])] : p.taken.filter((x) => x !== m.n),
          })
        } else if (m.type === 'call') {
          setG((p) => p && { ...p, phase: 'run', called: m.called })
          if (voice.current) speak(m.n)
        } else if (m.type === 'error') {
          setErr(m.code)
          setTimeout(() => setErr(''), 4000)
        }
      }
    }
    open()
    return () => {
      closed = true
      clearTimeout(timer)
      ws.current?.close()
    }
  }, [])

  const toggle = useCallback((n) => ws.current?.send(JSON.stringify({ type: 'toggle', n })), [])
  return { g, conn, err, toggle }
}

const Stat = ({ label, value, hi }) => (
  <div className={`rounded-2xl p-2 text-center ${hi ? 'bg-saffron text-ink' : 'bg-panel'}`}>
    <p className={`text-[11px] ${hi ? 'text-ink/70' : 'text-white/60'}`}>{label}</p>
    <p className="text-base font-bold tabular-nums">{value}</p>
  </div>
)

function Cartela({ n, card, called, bg = 'bg-panel' }) {
  const set = new Set(called)
  const last = called[called.length - 1]
  return (
    <figure className={`mt-3 rounded-3xl p-3 ${bg}`}>
      <figcaption className="mb-2 text-sm font-semibold text-saffron">#{n}</figcaption>
      <div className="grid grid-cols-5 gap-1.5 text-center">
        {[...'BINGO'].map((l, i) => <span key={l} className={`rounded-lg py-1 text-sm font-bold ${LETTER_BG[i]}`}>{l}</span>)}
        {card.flat().map((v, i) => (
          <span
            key={i}
            className={`grid h-10 place-items-center rounded-lg text-lg font-bold tabular-nums ${v === 0 || set.has(v) ? 'bg-called text-white' : 'bg-white text-ink'} ${v === last ? 'ring-2 ring-saffron' : ''}`}
          >
            {v === 0 ? '★' : v}
          </span>
        ))}
      </div>
    </figure>
  )
}

function Master({ called }) {
  const set = new Set(called)
  return (
    <section aria-label="1-75" className="mt-4 rounded-3xl bg-panel p-3">
      {[0, 1, 2, 3, 4].map((row) => (
        <div key={row} className="flex items-center gap-[2px] py-[1px]">
          <span className={`grid size-5 place-items-center rounded text-xs font-bold ${LETTER_BG[row]}`}>{'BINGO'[row]}</span>
          {Array.from({ length: 15 }, (_, i) => row * 15 + i + 1).map((n) => (
            <span key={n} className={`grid h-6 flex-1 place-items-center rounded text-[10px] tabular-nums ${set.has(n) ? 'bg-saffron font-bold text-ink' : 'text-white/40'}`}>{n}</span>
          ))}
        </div>
      ))}
    </section>
  )
}

function Rules({ g }) {
  const { t } = useL()
  const v = { pick: g.cfg.pick, run: g.cfg.run, fee: birr(g.fee), total: g.total, full: birr(g.cfg.full) }
  return (
    <details className="mt-5 rounded-3xl bg-panel p-4">
      <summary className="cursor-pointer font-semibold">{t('rules')}</summary>
      <ol className="mt-3 list-decimal space-y-2 pl-5 text-sm text-white/80">
        {['r1', 'r2', 'r3', 'r4', 'r5'].map((k) => <li key={k}>{t(k, v)}</li>)}
      </ol>
    </details>
  )
}

function Pick({ g, toggle, now }) {
  const { t } = useL()
  const left = Math.max(0, Math.ceil((g.deadline - now) / 1000))
  const grid = useMemo(() => {
    const taken = new Set(g.taken)
    const mine = new Set(g.mine)
    return Array.from({ length: g.total }, (_, i) => i + 1).map((n) => {
      const isMine = mine.has(n)
      const other = taken.has(n) && !isMine
      return (
        <button
          key={n}
          onClick={() => toggle(n)}
          disabled={other}
          aria-pressed={isMine}
          className={`h-10 rounded-lg text-sm font-bold tabular-nums ${isMine ? 'bg-called text-white' : other ? 'bg-rose text-white/90' : 'bg-ink text-white'}`}
        >
          {n}
        </button>
      )
    })
  }, [g.taken, g.mine, g.total, toggle])

  return (
    <>
      <section className="grid grid-cols-4 gap-2">
        <Stat label={t('round')} value={g.round} />
        <Stat label={t('time')} value={`${left}s`} hi />
        <Stat label={t('price')} value={birr(g.fee)} />
        <Stat label={t('mineCount')} value={`${g.mine.length}/${g.cfg.max}`} />
      </section>
      {g.taken.length < g.cfg.min && (
        <p className="mt-3 rounded-xl bg-line px-3 py-2 text-sm">{t('need', { min: g.cfg.min, n: g.taken.length })}</p>
      )}
      <p className="mt-3 text-sm text-white/70">{t('pickHint', { fee: birr(g.fee) })}</p>
      <div className="mt-3 max-h-[42dvh] overflow-y-auto rounded-3xl bg-panel p-3">
        <div className="grid grid-cols-8 gap-1.5">{grid}</div>
      </div>
      <p className="mt-2 flex items-center gap-2 text-xs text-white/70">
        <i className="inline-block size-3 rounded bg-called" /> {t('legendMine')}
        <i className="ml-3 inline-block size-3 rounded bg-rose" /> {t('legendTaken')}
      </p>
      <h2 className="mt-5 font-semibold">{t('myCards')}</h2>
      {g.mine.length === 0 && <p className="mt-2 text-sm text-white/60">{t('noCards')}</p>}
      {g.mine.map((n) => <Cartela key={n} n={n} card={g.cards[n]} called={[]} />)}
      <Rules g={g} />
    </>
  )
}

function Run({ g }) {
  const { t } = useL()
  const last = g.called[g.called.length - 1]
  const max = g.cfg.run / g.cfg.call
  return (
    <>
      <section className="grid grid-cols-4 gap-2">
        <Stat label={t('round')} value={g.round} />
        <Stat label={t('cartelas')} value={g.taken.length} />
        <Stat label={t('prize')} value={birr(g.prize)} />
        <Stat label={t('called')} value={`${g.called.length}/${max}`} hi />
      </section>
      <section className="mt-4 rounded-3xl bg-panel p-5 text-center">
        {last ? (
          <div className={`mx-auto grid size-36 place-items-center rounded-full ring-8 ring-saffron/80 ${ballBg(last)}`}>
            <p className="text-5xl font-bold tabular-nums">{letterOf(last)}-{last}</p>
          </div>
        ) : (
          <p className="py-10 text-xl font-bold">{t('ready')}</p>
        )}
        <div className="mt-4 flex flex-wrap justify-center gap-2">
          {g.called.slice(-6, -1).reverse().map((n) => (
            <span key={n} className={`rounded-full px-3 py-1 text-sm font-bold tabular-nums ${ballBg(n)}`}>{letterOf(n)}-{n}</span>
          ))}
        </div>
      </section>
      {g.mine.length === 0 && (
        <section className="mt-4 rounded-3xl bg-panel p-4 text-center">
          <p className="text-lg font-bold">{t('watching')}</p>
          <p className="mt-1 text-sm text-white/70">{t('watchingMsg')}</p>
        </section>
      )}
      {g.mine.map((n) => <Cartela key={n} n={n} card={g.cards[n]} called={g.called} />)}
      <Master called={g.called} />
    </>
  )
}

function Result({ g, me, now }) {
  const { t } = useL()
  const r = g.result
  const left = Math.max(0, Math.ceil((g.deadline - now) / 1000))
  const next = <p className="mt-4 text-center text-sm text-white/70">{t('nextIn', { n: left })}</p>
  if (!r || r.winners.length === 0) {
    return (
      <section className="rounded-3xl bg-panel p-6 text-center">
        <p className="text-3xl font-bold">{t('noWinner')}</p>
        <p className="mt-2 text-white/70">{t('refunded')}</p>
        {next}
      </section>
    )
  }
  const won = r.winners.some((w) => w.uid === me?.id)
  return (
    <section>
      <div className="rounded-3xl bg-panel p-6 text-center">
        <p className="mx-auto grid size-16 place-items-center rounded-full bg-saffron text-3xl text-ink">♛</p>
        <p className="mt-2 text-3xl font-bold text-saffron">{t('bingo')}</p>
        {won && <p className="mt-1 font-semibold">{t('youWon')}</p>}
        <p className="mt-2 text-sm text-white/70">{t('prize')} {birr(r.prize)} {t('birr')}</p>
      </div>
      {r.winners.map((w) => (
        <article key={w.cartela} className="mt-3 rounded-3xl bg-panel p-4">
          <p className="text-xl font-bold">{w.name}</p>
          <p className="text-sm tabular-nums text-white/70">{w.phone} · {t('winCartela')} #{w.cartela}</p>
          <p className="mt-1 text-lg font-bold text-called">{birr(w.share)} {t('birr')}</p>
          <Cartela n={w.cartela} card={w.card} called={g.called} bg="bg-ink" />
        </article>
      ))}
      {next}
    </section>
  )
}

function Wallet({ me, refresh }) {
  const { t } = useL()
  const [phone, setPhone] = useState('')
  const [amount, setAmount] = useState('')
  const [msg, setMsg] = useState('')
  const submit = async (e) => {
    e.preventDefault()
    try {
      await api('/api/withdraw', { method: 'POST', body: JSON.stringify({ phone, amount: Number(amount) }) })
      setMsg(t('sent'))
      setAmount('')
      refresh()
    } catch (x) {
      setMsg(x.message)
    }
  }
  const input = 'mt-1 w-full rounded-xl bg-ink px-3 py-3 outline-none focus:ring-2 focus:ring-saffron'
  return (
    <div className="space-y-5">
      <section className="grid grid-cols-2 gap-3">
        <article className="rounded-3xl bg-panel p-4">
          <p className="text-sm text-white/70">{t('main')}</p>
          <p className="text-3xl font-bold tabular-nums">{me ? birr(me.balance) : '—'}</p>
          <p className="text-xs text-white/60">{t('withdrawable')}</p>
        </article>
        <article className="rounded-3xl bg-panel p-4">
          <p className="text-sm text-white/70">{t('bonus')}</p>
          <p className="text-3xl font-bold tabular-nums text-called">{me ? birr(me.bonus) : '—'}</p>
          <p className="text-xs text-white/60">{t('playOnly')}</p>
        </article>
      </section>
      <section className="rounded-3xl bg-panel p-4">
        <h2 className="font-semibold">{t('depositTitle')}</h2>
        <p className="mt-1 text-sm text-white/70">{t('depositBody')}</p>
        <button onClick={() => tg?.close()} className="mt-3 w-full rounded-2xl bg-mint py-3 font-bold text-ink">{t('toBot')}</button>
      </section>
      <form onSubmit={submit} className="rounded-3xl bg-panel p-4">
        <h2 className="font-semibold">{t('withdrawTitle')}</h2>
        <label className="mt-3 block text-sm text-white/70" htmlFor="phone">{t('tele')}</label>
        <input id="phone" value={phone} onChange={(e) => setPhone(e.target.value)} inputMode="tel" placeholder="09…" required className={input} />
        <label className="mt-3 block text-sm text-white/70" htmlFor="amount">{t('amount', { min: me ? me.minWithdraw / 100 : 100 })}</label>
        <input id="amount" value={amount} onChange={(e) => setAmount(e.target.value)} inputMode="numeric" required className={input} />
        <button className="mt-4 w-full rounded-2xl bg-saffron py-3 font-bold text-ink">{t('send')}</button>
        {msg && <p role="status" className="mt-2 text-sm text-white/80">{msg}</p>}
      </form>
    </div>
  )
}

function History({ me }) {
  const { t } = useL()
  const [txs, setTxs] = useState([])
  useEffect(() => {
    api('/api/txs').then(setTxs).catch(() => {})
  }, [me?.balance, me?.bonus])
  if (txs.length === 0) return <p className="py-16 text-center text-white/60">{t('noTx')}</p>
  return (
    <ul className="divide-y divide-line">
      {txs.map((x, i) => (
        <li key={i} className="flex items-center justify-between py-3 text-sm">
          <span>{t('k_' + x.kind)}<span className="block text-xs text-white/50">{new Date(x.at).toLocaleString()}</span></span>
          <span className={`font-bold tabular-nums ${x.amount < 0 ? 'text-rose' : 'text-called'}`}>{x.amount < 0 ? '-' : '+'}{birr(Math.abs(x.amount))}</span>
        </li>
      ))}
    </ul>
  )
}

function Profile({ me }) {
  const { t, lang, set } = useL()
  const [copied, setCopied] = useState(false)
  if (!me) return <p className="py-16 text-center text-white/60">{t('loading')}</p>
  const link = `https://t.me/${me.bot}?start=ref_${me.id}`
  const copy = async () => {
    await navigator.clipboard.writeText(link).catch(() => {})
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }
  const share = () => {
    const url = `https://t.me/share/url?url=${encodeURIComponent(link)}&text=${encodeURIComponent(t('shareText'))}`
    if (tg) tg.openTelegramLink(url)
    else window.open(url, '_blank')
  }
  const cards = [[t('main'), birr(me.balance)], [t('bonus'), birr(me.bonus)], [t('wins'), me.gamesWon], [t('invites'), me.referrals], [t('earned'), birr(me.earned)]]
  return (
    <div className="space-y-5">
      <section className="text-center">
        <p className="mx-auto grid size-20 place-items-center rounded-full bg-panel text-3xl font-bold">{me.name?.[0]}</p>
        <p className="mt-2 text-xl font-bold">{me.name}</p>
        <p className="text-sm tabular-nums text-white/60">{me.phone}</p>
      </section>
      <section className="grid grid-cols-2 gap-3">
        {cards.map(([label, v]) => (
          <article key={label} className="rounded-3xl bg-panel p-4">
            <p className="text-sm text-white/70">{label}</p>
            <p className="text-2xl font-bold tabular-nums">{v}</p>
          </article>
        ))}
      </section>
      <section className="rounded-3xl bg-panel p-4">
        <h2 className="font-semibold">{t('inviteTitle')}</h2>
        <p className="mt-1 text-sm text-white/70">{t('inviteBody')}</p>
        <p className="mt-3 break-all rounded-xl bg-ink p-3 text-xs">{link}</p>
        <div className="mt-3 grid grid-cols-2 gap-3">
          <button onClick={copy} className="rounded-2xl bg-line py-3 font-bold">{copied ? t('copied') : t('copy')}</button>
          <button onClick={share} className="rounded-2xl bg-saffron py-3 font-bold text-ink">{t('shareBtn')}</button>
        </div>
      </section>
      <section className="flex items-center justify-between rounded-3xl bg-panel p-4">
        <span className="font-semibold">{t('language')}</span>
        <div className="flex gap-2">
          {[['am', 'አማርኛ'], ['en', 'English']].map(([id, label]) => (
            <button key={id} onClick={() => set(id)} aria-pressed={lang === id} className={`rounded-full px-4 py-2 text-sm font-semibold ${lang === id ? 'bg-saffron text-ink' : 'bg-ink'}`}>{label}</button>
          ))}
        </div>
      </section>
    </div>
  )
}

function Player() {
  const { t } = useL()
  const [tab, setTab] = useState('game')
  const [voice, setVoice] = useState(false)
  const [me, setMe] = useState(null)
  const now = useNow()
  const { g, conn, err, toggle } = useGame(voice)
  const refresh = useCallback(() => api('/api/me').then(setMe).catch(() => {}), [])

  useEffect(() => {
    tg?.ready()
    tg?.expand()
  }, [])
  useEffect(() => {
    refresh()
  }, [refresh, g?.phase, g?.mine?.length])

  const toggleVoice = () => {
    const next = !voice
    setVoice(next)
    if (next && 'speechSynthesis' in window) speechSynthesis.speak(new SpeechSynthesisUtterance('Voice on'))
  }

  return (
    <div className="mx-auto flex min-h-dvh max-w-md flex-col">
      <header className="sticky top-0 z-10 bg-ink px-4 pb-3 pt-4 pt-[max(1rem,env(safe-area-inset-top))]">
        <div className="flex items-center justify-between">
          <h1 className="text-2xl font-bold">Bingo</h1>
          <button onClick={toggleVoice} aria-pressed={voice} className="rounded-full bg-panel px-3 py-1 text-sm">{voice ? t('voiceOn') : t('voiceOff')}</button>
        </div>
        {g && (
          <div className="mt-3 flex items-center justify-between rounded-2xl bg-saffron px-4 py-2 text-ink">
            <span className="font-semibold">{t('prize')}</span>
            <span className="text-3xl font-bold tabular-nums">{birr(g.prize)} <small className="text-base">{t('birr')}</small></span>
            <span className="text-xs font-semibold tabular-nums">{g.taken.length}/{g.total}</span>
          </div>
        )}
        <div className="mt-2 grid grid-cols-2 gap-2 text-center text-sm">
          <p className="rounded-xl bg-panel py-1">{t('main')} <b className="tabular-nums">{me ? birr(me.balance) : '—'}</b></p>
          <p className="rounded-xl bg-panel py-1">{t('bonus')} <b className="tabular-nums text-called">{me ? birr(me.bonus) : '—'}</b></p>
        </div>
      </header>
      {!conn && <p role="status" className="mx-4 mt-2 rounded-xl bg-line px-3 py-2 text-sm">{t('reconnecting')}</p>}
      {err && <p role="alert" className="mx-4 mt-2 rounded-xl bg-rose px-3 py-2 text-sm font-semibold text-ink">{t('err_' + err, { max: g?.cfg.max })}</p>}
      <main className="flex-1 px-4 pb-24 pt-3">
        {tab === 'game' && (!g ? <p className="py-16 text-center text-white/60">{t('connecting')}</p>
          : g.phase === 'pick' ? <Pick g={g} toggle={toggle} now={now} />
          : g.phase === 'run' ? <Run g={g} />
          : <Result g={g} me={me} now={now} />)}
        {tab === 'history' && <History me={me} />}
        {tab === 'wallet' && <Wallet me={me} refresh={refresh} />}
        {tab === 'profile' && <Profile me={me} />}
      </main>
      <nav className="fixed inset-x-0 bottom-0 mx-auto flex max-w-md border-t border-line bg-ink pb-[env(safe-area-inset-bottom)]">
        {['game', 'history', 'wallet', 'profile'].map((id) => (
          <button key={id} onClick={() => setTab(id)} aria-current={tab === id ? 'page' : undefined} className={`flex-1 py-3 text-sm font-semibold ${tab === id ? 'text-saffron' : 'text-white/60'}`}>
            {t(id)}
          </button>
        ))}
      </nav>
    </div>
  )
}

export default function App() {
  const [lang, setLang] = useState(() => {
    try {
      return localStorage.getItem('lang') || 'am'
    } catch {
      return 'am'
    }
  })
  const ctx = useMemo(() => ({
    lang,
    set: (l) => {
      setLang(l)
      try {
        localStorage.setItem('lang', l)
      } catch { /* storage unavailable */ }
    },
    t: (k, v = {}) => (DICT[lang][k] ?? DICT.en[k] ?? k).replace(/\{(\w+)\}/g, (_, x) => v[x] ?? ''),
  }), [lang])
  if (location.hash === '#admin') return <Admin />
  return <LangCtx.Provider value={ctx}><Player /></LangCtx.Provider>
}
