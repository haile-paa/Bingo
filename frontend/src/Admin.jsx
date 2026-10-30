// frontend/src/Admin.jsx
import { useCallback, useEffect, useState } from 'react'

const birr = (s) => (s / 100).toFixed(2)

export default function Admin() {
  const [key, setKey] = useState(sessionStorage.getItem('akey') || '')
  const [input, setInput] = useState('')
  const [tab, setTab] = useState('deposits')
  const [rows, setRows] = useState([])
  const [stats, setStats] = useState(null)
  const [error, setError] = useState('')

  const call = useCallback(async (path, method = 'GET') => {
    const res = await fetch(path, { method, headers: { 'X-Admin-Key': key } })
    const data = await res.json().catch(() => ({}))
    if (!res.ok) throw new Error(data.error || 'Request failed')
    return data
  }, [key])

  const load = useCallback(async () => {
    if (!key) return
    try {
      setRows(await call(`/admin/${tab}?status=pending`))
      setStats(await call('/admin/stats'))
      setError('')
    } catch (e) {
      setError(e.message)
      if (e.message === 'unauthorized') {
        sessionStorage.removeItem('akey')
        setKey('')
      }
    }
  }, [call, key, tab])

  useEffect(() => {
    load()
    const id = setInterval(load, 10000)
    return () => clearInterval(id)
  }, [load])

  const act = async (id, action) => {
    if (!confirm(`Confirm: ${action}?`)) return
    try {
      await call(`/admin/${tab}/${id}/${action}`, 'POST')
      load()
    } catch (e) {
      setError(e.message)
    }
  }

  if (!key) {
    return (
      <main className="mx-auto grid min-h-dvh max-w-sm place-items-center px-4">
        <form
          onSubmit={(e) => {
            e.preventDefault()
            sessionStorage.setItem('akey', input)
            setKey(input)
          }}
          className="w-full rounded-3xl bg-panel p-6"
        >
          <h1 className="text-xl font-bold">Admin sign in</h1>
          <label htmlFor="k" className="mt-4 block text-sm text-white/70">Admin key</label>
          <input id="k" type="password" value={input} onChange={(e) => setInput(e.target.value)} className="mt-1 w-full rounded-xl bg-ink px-3 py-3 outline-none focus:ring-2 focus:ring-saffron" />
          <button className="mt-4 w-full rounded-2xl bg-saffron py-3 font-bold text-ink">Sign in</button>
          {error && <p role="alert" className="mt-2 text-sm text-rose">{error}</p>}
        </form>
      </main>
    )
  }

  const isDep = tab === 'deposits'
  return (
    <main className="mx-auto max-w-3xl px-4 py-6">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-bold">Bingo admin</h1>
        {stats && (
          <p className="text-sm text-white/70">
            {stats.users} users · {stats.online} online · {stats.pendingDeposits} deposits · {stats.pendingWithdrawals} withdrawals waiting
          </p>
        )}
      </header>
      <nav className="mt-4 flex gap-2">
        {['deposits', 'withdrawals'].map((t) => (
          <button key={t} onClick={() => setTab(t)} aria-pressed={tab === t} className={`rounded-full px-4 py-2 text-sm font-semibold ${tab === t ? 'bg-saffron text-ink' : 'bg-panel'}`}>
            {t === 'deposits' ? 'Deposits' : 'Withdrawals'}
          </button>
        ))}
      </nav>
      {error && <p role="alert" className="mt-3 rounded-xl bg-rose px-3 py-2 text-sm font-semibold text-ink">{error}</p>}
      {rows.length === 0 && <p className="mt-8 text-center text-white/60">Nothing waiting. This list refreshes every 10 seconds.</p>}
      <ul className="mt-4 space-y-3">
        {rows.map((r) => (
          <li key={r._id} className="rounded-3xl bg-panel p-4">
            <div className="flex items-start justify-between gap-3">
              <div>
                <p className="text-2xl font-bold tabular-nums">{birr(r.amount)} birr</p>
                <p className="text-sm text-white/70">User {r.uid} · {new Date(r.createdAt).toLocaleString()}</p>
                {isDep ? <p className="text-sm">Ref <b>{r.txnId}</b> · <span className={r.receiverOk ? 'text-mint' : 'text-rose'}>{r.receiverOk ? 'receiver matches' : 'receiver NOT matched: check the Telebirr app'}</span></p> : <p className="text-sm">Send to <b>{r.phone}</b></p>}
              </div>
              <div className="flex gap-2">
                <button onClick={() => act(r._id, isDep ? 'approve' : 'paid')} className="rounded-xl bg-mint px-3 py-2 text-sm font-bold text-ink">{isDep ? 'Approve' : 'Mark paid'}</button>
                <button onClick={() => act(r._id, 'reject')} className="rounded-xl bg-rose px-3 py-2 text-sm font-bold text-ink">Reject</button>
              </div>
            </div>
            {isDep && <pre className="mt-3 whitespace-pre-wrap rounded-xl bg-ink p-3 text-xs text-white/80">{r.sms}</pre>}
          </li>
        ))}
      </ul>
    </main>
  )
}
