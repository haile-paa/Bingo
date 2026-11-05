// frontend/src/App.jsx
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import Admin from "./Admin.jsx";

const tg = window.Telegram?.WebApp;
const AUTH = tg?.initData
  ? `tma ${tg.initData}`
  : `dev ${new URLSearchParams(location.search).get("dev") || "1"}`;
const birr = (s) =>
  (s / 100).toLocaleString("en-US", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
const BRAND = "Big Bingo"; // change the app name shown on the first page here
const letterOf = (n) => "BINGO"[Math.floor((n - 1) / 15)];
const LETTER_BG = [
  "bg-[#3b82f6]",
  "bg-[#6366f1]",
  "bg-[#a855f7]",
  "bg-[#10b981]",
  "bg-[#f97316]",
];
const ballBg = (n) => LETTER_BG[Math.floor((n - 1) / 15)];

const DICT = {
  am: {
    game: "ጨዋታ",
    history: "ታሪክ",
    wallet: "ዋሌት",
    profile: "መገለጫ",
    prize: "ደራሽ",
    birr: "ብር",
    main: "ዋና ዋሌት",
    bonus: "የጨዋታ ዋሌት",
    round: "ዙር",
    time: "ሰዓት",
    price: "ዋጋ",
    mineCount: "የእኔ",
    called: "የተጠሩ",
    cartelas: "ካርቴላ",
    pickHint: "አንድ ካርቴላ {fee} ብር ነው። ከአንድ በላይ መያዝ ይችላሉ። እንደገና ሲነኩ ይሰረዛል።",
    legendMine: "የእርስዎ",
    legendTaken: "የተያዘ",
    myCards: "የእኔ ካርቴላዎች",
    noCards: "እስካሁን ካርቴላ አልመረጡም።",
    need: "ጨዋታው ለመጀመር ቢያንስ {min} ካርቴላ ያስፈልጋል። አሁን {n} ተይዟል።",
    ready: "ተዘጋጁ…",
    watching: "ተመልካች ብቻ",
    watchingMsg: "የዚህ ዙር ጨዋታ ተጀምሯል። አዲስ ዙር እስኪጀምር እዚሁ ይጠብቁ።",
    bingo: "ቢንጎ!",
    youWon: "እርስዎ አሸነፉ! 🎉",
    winCartela: "አሸናፊ ካርቴላ",
    noWinner: "አሸናፊ የለም",
    refunded: "ክፍያዎ ወደ ዋሌትዎ ተመልሷል።",
    nextIn: "ቀጣይ ጨዋታ በ{n} ሰከንድ ይጀምራል",
    rules: "ደንቦች",
    r1: "ካርቴላ ለመምረጥ {pick} ሰከንድ አለዎት። ዋጋው 5 ወይም 10 ብር ነው (በመረጡት ጨዋታ)።",
    r2: "ከዚያ ወደ ጨዋታው ይገባሉ፤ ቁጥሮች አንድ በአንድ ይጠራሉ። የተጠራ ቁጥር በካርቴላዎ ላይ አረንጓዴ ይሆናል።",
    r3: "መስመር (አግድም፣ ቁልቁል ወይም ሰያፍ) የሞላ የመጀመሪያው ካርቴላ ያሸንፋል፤ አሸናፊ ሲገኝ ቁጥር መጥራት ይቆማል።",
    r4: "በእያንዳንዱ ጨዋታ {total} ቦታ አለ። ከተከፈለው ጠቅላላ ገንዘብ {pct}% ለአሸናፊ ይሰጣል፤ ቀሪው {cut}% የአገልግሎት ክፍያ ነው።",
    r5: "ሁለት ካርቴላ በአንድ ጊዜ ቢያሸንፉ ሽልማቱ እኩል ይካፈላል።",
    mtDays: "ቀን",
    mtHours: "ሰዓት",
    mtMin: "ደቂቃ",
    mtSec: "ሰከንድ",
    mtSoon: "በቅርቡ እንመለሳለን…",
    mtAlmost: "ልንጨርስ ነው… ገጹ በራሱ ይከፈታል",
    mtBot: "ገንዘብ ማስገባት እና ሂሳብ ማየት በቦቱ ላይ ይቻላል።",
    err_maintenance: "ጨዋታው ለጥገና ዝግ ነው።",
    err_closed: "መምረጫ ጊዜው አልፏል። ቀጣዩን ዙር ይጠብቁ።",
    err_taken: "ይህ ካርቴላ ተይዟል።",
    err_funds: "በቂ ሂሳብ የለዎትም። በቦቱ ውስጥ ገንዘብ ያስገቡ።",
    err_register: "መጀመሪያ በቦቱ ውስጥ «ተመዝገብ» ን ተጭነው ይመዝገቡ።",
    err_limit: "አንድ ሰው እስከ {max} ካርቴላ ብቻ መያዝ ይችላል።",
    err_server: "ችግር ተፈጥሯል። እንደገና ይሞክሩ።",
    cbTitle: "ሳምንታዊ ካሽባክ {pct}%",
    cbDeposit: "በዚህ ሳምንት ያስገቡት",
    cbLost: "ያጡት ገንዘብ",
    cbSoFar: "እስካሁን የሚያገኙት",
    cbDays: "ሰኞ ይከፈላል · {n} ቀን ይቀራል",
    cbNote: "የተሸነፈ ገንዘብ ብቻ ይቆጠራል። ያሸነፈ ገንዘብ ከካሽባክ ይወጣል።",
    depositTitle: "ገንዘብ ማስገባት",
    depositBody:
      "ማስገባት በቦቱ ውስጥ ብቻ ነው። «ገንዘብ አስገባ» ን ተጭነው ከ10 ብር ጀምሮ በቴሌብር ይላኩ፣ ከዚያ የደረሰዎትን SMS ወይም የደረሰኝ ሊንክ ለቦቱ ይላኩ።",
    toBot: "ወደ ቦቱ ሂድ",
    withdrawTitle: "ገንዘብ ማውጣት",
    tele: "የቴሌብር ስልክ ቁጥር",
    amount: "መጠን (ብር፣ ዝቅተኛ {min})",
    send: "ጥያቄ ላክ",
    sent: "ጥያቄዎ ተልኳል። አድሚን በቴሌብር ይልካል።",
    withdrawable: "የሚወጣ",
    playOnly: "ለጨዋታ ብቻ",
    noTx: "እስካሁን እንቅስቃሴ የለም።",
    wins: "ያሸነፉ ጨዋታዎች",
    invites: "የጋበዟቸው",
    earned: "ጠቅላላ ገቢ",
    language: "ቋንቋ",
    inviteTitle: "ጓደኞችን ይጋብዙ",
    inviteBody:
      "ጓደኛዎ ሲመዘገብ 10 ብር ያገኛል። ጓደኛዎ ቢያንስ 10 ብር ሲያስገባ እርስዎ 10 ብር ያገኛሉ። ገቢ ያደረጉ 20 ጓደኞች ሲጋብዙ 150 ብር ያገኛሉ።",
    copy: "ኮፒ",
    copied: "ተቀድቷል",
    shareBtn: "አጋራ",
    shareText: "ከእኔ ጋር Big Bingo ይጫወቱ! 20 ሰው ይጋብዙ እና 150 ብር ያግኙ! 🎁",
    connecting: "ከጨዋታው ጋር በመገናኘት ላይ…",
    reconnecting: "እንደገና በመገናኘት ላይ…",
    loading: "በመጫን ላይ…",
    waking: "ሰርቨሩ እየተነሳ ነው… እስከ 1 ደቂቃ ሊወስድ ይችላል",
    tagline: "ቁጥር ይምረጡ፣ ቢንጎ ይጮሁ፣ ያሸንፉ!",
    welcomeTo: "እንኳን ደህና መጡ ወደ",
    chooseStake: "ዋጋዎን ይምረጡ",
    play10: "ተጫወት 10",
    play5: "ተጫወት 5",
    statPct: "ለአሸናፊ የሚከፈል",
    statSpots: "ቦታ በአንድ ጨዋታ",
    statSold: "አሁን የተያዙ ካርቴላዎች",
    prizeNow: "ደራሽ",
    players: "ተጫዋቾች",
    gameId: "የጨዋታ ቁጥር",
    bet: "ውርርድ",
    derash: "ደራሽ",
    stake: "ዋጋ",
    back: "ተመለስ",
    refresh: "አድስ",
    leave: "ውጣ",
    auto: "አውቶማቲክ",
    lobbyRound: "አሁን ያለው ዙር",
    lobbyPlayers: "የተመዘገቡ ተጫዋቾች",
    lobbyPrize: "አሁን ያለው ደራሽ",
    k_signup_bonus: "የምዝገባ ስጦታ",
    k_deposit: "ገቢ",
    k_referral: "የግብዣ ሽልማት",
    k_entry: "የካርቴላ ክፍያ",
    k_win: "ሽልማት",
    k_cashback: "ሳምንታዊ ካሽባክ",
    k_withdraw: "ወጪ",
    k_refund: "ተመላሽ",
  },
  en: {
    game: "Game",
    history: "History",
    wallet: "Wallet",
    profile: "Profile",
    prize: "Prize",
    birr: "birr",
    main: "Main wallet",
    bonus: "Play wallet",
    round: "Round",
    time: "Time",
    price: "Price",
    mineCount: "Mine",
    called: "Called",
    cartelas: "cartelas",
    pickHint:
      "One cartela costs {fee} birr. You can hold several. Tap again to release.",
    legendMine: "Yours",
    legendTaken: "Taken",
    myCards: "My cartelas",
    noCards: "You have not picked a cartela yet.",
    need: "At least {min} cartelas are needed to start. {n} taken so far.",
    ready: "Get ready…",
    watching: "Watching only",
    watchingMsg: "This round has started. Wait here for the next one.",
    bingo: "BINGO!",
    youWon: "You won! 🎉",
    winCartela: "Winning cartela",
    noWinner: "No winner",
    refunded: "Your payment was returned to your wallet.",
    nextIn: "Next game starts in {n}s",
    rules: "Rules",
    r1: "You have {pick} seconds to pick cartelas. Each costs 5 or 10 birr, depending on the game you choose.",
    r2: "Then the game starts and numbers are called one by one. Called numbers turn green on your cartela.",
    r3: "The first cartela to complete a line (row, column or diagonal) wins. Calling stops as soon as there is a winner.",
    r4: "Each game has {total} spots. Winners get {pct}% of all stakes paid in; the other {cut}% is the service fee.",
    r5: "If several cartelas win together the prize is split equally.",
    mtDays: "Days",
    mtHours: "Hours",
    mtMin: "Min",
    mtSec: "Sec",
    mtSoon: "We will be back soon…",
    mtAlmost: "Almost done… this page opens by itself",
    mtBot: "Deposits and balance still work in the bot.",
    err_maintenance: "The game is closed for maintenance.",
    err_closed: "Picking is closed. Wait for the next round.",
    err_taken: "That cartela is taken.",
    err_funds: "Not enough balance. Deposit in the bot.",
    err_register: "Register in the bot first (tap Register).",
    err_limit: "You can hold at most {max} cartelas.",
    err_server: "Something went wrong. Try again.",
    cbTitle: "Weekly cashback {pct}%",
    cbDeposit: "Deposited this week",
    cbLost: "Money lost",
    cbSoFar: "Cashback so far",
    cbDays: "Paid Monday · {n} d left",
    cbNote:
      "Only money you lose counts. Money that wins a prize is removed from the cashback.",
    depositTitle: "Deposit",
    depositBody:
      "Deposits happen in the bot only. Tap Deposit, send at least 10 birr by Telebirr, then paste the SMS or the receipt link to the bot.",
    toBot: "Go to the bot",
    withdrawTitle: "Withdraw",
    tele: "Telebirr phone number",
    amount: "Amount (birr, min {min})",
    send: "Send request",
    sent: "Request sent. An admin will pay you by Telebirr.",
    withdrawable: "Withdrawable",
    playOnly: "Play only",
    noTx: "No activity yet.",
    wins: "Games won",
    invites: "Invited",
    earned: "Total earned",
    language: "Language",
    inviteTitle: "Invite friends",
    inviteBody:
      "Your friend gets 10 birr when they join. You get 10 birr when their first deposit of at least 10 birr is approved. Invite 20 friends who deposit and get 150 birr.",
    copy: "Copy",
    copied: "Copied",
    shareBtn: "Share",
    shareText: "Play Big Bingo with me! Invite 20 people and get 150 birr! 🎁",
    connecting: "Connecting to the game…",
    reconnecting: "Reconnecting…",
    loading: "Loading…",
    waking: "Waking up the server… this can take up to a minute",
    tagline: "Pick your numbers. Shout BINGO. Win.",
    welcomeTo: "Welcome to",
    chooseStake: "Choose Your Stake",
    play10: "Play 10",
    play5: "Play 5",
    statPct: "Paid to winners",
    statSpots: "Spots per game",
    statSold: "Cartelas taken now",
    prizeNow: "Prize",
    players: "Players",
    gameId: "Game ID",
    bet: "Bet",
    derash: "Derash",
    stake: "Stake",
    back: "Back",
    refresh: "Refresh",
    leave: "Leave",
    auto: "Automatic",
    lobbyRound: "Current round",
    lobbyPlayers: "Players this round",
    lobbyPrize: "Current prize",
    k_signup_bonus: "Welcome bonus",
    k_deposit: "Deposit",
    k_referral: "Invite reward",
    k_entry: "Cartela payment",
    k_win: "Prize won",
    k_cashback: "Weekly cashback",
    k_withdraw: "Withdrawal",
    k_refund: "Refund",
  },
};

const LangCtx = createContext(null);
const useL = () => useContext(LangCtx);

const API_URL = (import.meta.env.VITE_API_URL || "").replace(/\/$/, "");

async function api(path, opts = {}) {
  const res = await fetch(API_URL + path, {
    ...opts,
    headers: { "Content-Type": "application/json", Authorization: AUTH },
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || "Something went wrong");
  return data;
}

function useNow() {
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);
  return now;
}

// live numbers of every room, for the first page
function useRooms(active) {
  const [rooms, setRooms] = useState(null);
  useEffect(() => {
    if (!active) return undefined;
    let stop = false;
    const load = () =>
      fetch(API_URL + "/api/rooms")
        .then((r) => r.json())
        .then((d) => !stop && Array.isArray(d) && d.length === 2 && setRooms(d))
        .catch(() => {});
    load();
    const id = setInterval(load, 3000);
    return () => {
      stop = true;
      clearInterval(id);
    };
  }, [active]);
  return rooms;
}

function useGame(stake) {
  const [g, setG] = useState(null);
  const [conn, setConn] = useState(false);
  const [err, setErr] = useState("");
  const ws = useRef(null);

  useEffect(() => {
    setG(null);
    setConn(false);
    if (!stake) return undefined;
    let timer;
    let closed = false;
    const open = () => {
      const base = API_URL
        ? API_URL.replace(/^http/, "ws")
        : `${location.protocol === "https:" ? "wss" : "ws"}://${location.host}`;
      const sock = new WebSocket(
        `${base}/ws?a=${encodeURIComponent(AUTH)}&s=${stake}`,
      );
      ws.current = sock;
      sock.onopen = () => setConn(true);
      sock.onclose = () => {
        setConn(false);
        if (!closed) timer = setTimeout(open, 1500);
      };
      sock.onmessage = (e) => {
        const m = JSON.parse(e.data);
        if (m.type === "state") setG(m);
        else if (m.type === "mine")
          setG((p) => p && { ...p, mine: m.mine, cards: m.cards });
        else if (m.type === "spot") {
          setG(
            (p) =>
              p && {
                ...p,
                prize: m.prize,
                taken: m.on
                  ? [...new Set([...p.taken, m.n])]
                  : p.taken.filter((x) => x !== m.n),
              },
          );
        } else if (m.type === "spots") {
          setG((p) => {
            if (!p) return p;
            const t = new Set(p.taken);
            const b = new Set(p.bots || []);
            for (const s of m.list) {
              if (s.on) {
                t.add(s.n);
                if (s.bot) b.add(s.n);
              } else {
                t.delete(s.n);
                b.delete(s.n);
              }
            }
            return { ...p, prize: m.prize, taken: [...t], bots: [...b] };
          });
        } else if (m.type === "sync") {
          setG(
            (p) =>
              p && { ...p, prize: m.prize, taken: m.taken, bots: m.bots || [] },
          );
        } else if (m.type === "call") {
          setG((p) => p && { ...p, phase: "run", called: m.called });
        } else if (m.type === "error") {
          setErr(m.code);
          setTimeout(() => setErr(""), 4000);
        }
      };
    };
    open();
    return () => {
      closed = true;
      clearTimeout(timer);
      ws.current?.close();
    };
  }, [stake]);

  const toggle = useCallback(
    (n) => ws.current?.send(JSON.stringify({ type: "toggle", n })),
    [],
  );
  // closing the socket makes it reconnect by itself and the server sends a fresh full state
  const reconnect = useCallback(() => ws.current?.close(), []);
  return { g, conn, err, toggle, reconnect };
}

function Cartela({ n, card, called, bg = "bg-panel", compact = false }) {
  const set = new Set(called);
  const last = called[called.length - 1];
  return (
    <figure
      className={`${compact ? "mt-2 rounded-2xl p-2" : "mt-3 rounded-3xl p-3"} ${bg}`}
    >
      <figcaption className='mb-2 text-sm font-semibold text-saffron'>
        #{n}
      </figcaption>
      <div
        className={`grid grid-cols-5 text-center ${compact ? "gap-1" : "gap-1.5"}`}
      >
        {[..."BINGO"].map((l, i) => (
          <span
            key={l}
            className={`rounded-lg py-1 text-sm font-bold ${LETTER_BG[i]}`}
          >
            {l}
          </span>
        ))}
        {card.flat().map((v, i) => (
          <span
            key={i}
            className={`grid place-items-center rounded-lg font-bold tabular-nums ${compact ? "h-8 text-sm" : "h-10 text-lg"} ${v === 0 || set.has(v) ? "bg-called text-white" : "bg-white text-ink"} ${v === last ? "ring-2 ring-saffron" : ""}`}
          >
            {v === 0 ? "★" : v}
          </span>
        ))}
      </div>
    </figure>
  );
}

function Rules({ v }) {
  const { t } = useL();
  return (
    <section className='glass mt-4 rounded-3xl p-4'>
      <ol className='list-decimal space-y-2 pl-5 text-sm text-white/80'>
        {["r1", "r2", "r3", "r4", "r5"].map((k) => (
          <li key={k}>{t(k, v)}</li>
        ))}
      </ol>
    </section>
  );
}

const RULE_VALUES = { pick: 30, total: 300, pct: 80, cut: 20 };

// ---------- skeleton (shown while the server is waking up) ----------
const Sk = ({ className = "" }) => (
  <div className={`skel ${className}`} aria-hidden />
);

function useAfter(ms) {
  const [done, setDone] = useState(false);
  useEffect(() => {
    const id = setTimeout(() => setDone(true), ms);
    return () => clearTimeout(id);
  }, [ms]);
  return done;
}

function Waking() {
  const { t } = useL();
  const late = useAfter(4000);
  return (
    <p
      role='status'
      className={`mt-4 text-center text-sm text-white/70 ${late ? "" : "invisible"}`}
    >
      {t("waking")}
    </p>
  );
}

function GameSkeleton() {
  return (
    <div aria-busy='true'>
      <div className='flex justify-between'>
        <Sk className='h-10 w-24' />
        <Sk className='h-10 w-24' />
      </div>
      <div className='mt-3 flex gap-2'>
        {[0, 1, 2, 3].map((i) => (
          <Sk key={i} className='h-12 flex-1' />
        ))}
      </div>
      <div className='mt-4 grid grid-cols-8 gap-1.5'>
        {Array.from({ length: 64 }, (_, i) => (
          <Sk key={i} className='h-10' />
        ))}
      </div>
      <Waking />
    </div>
  );
}

// ---------- page 1: choose the 5 or 10 birr game ----------
function Lobby({ rooms, onPlay }) {
  const { t } = useL();
  const [rules, setRules] = useState(false);
  const sold = rooms ? rooms.reduce((n, r) => n + r.players, 0) : 0;
  return (
    <>
      <div className='flex items-center justify-between'>
        <div className='flex items-center gap-3'>
          <span className='grid size-12 place-items-center rounded-full bg-white text-2xl font-bold text-purple-700 shadow-lg'>
            {BRAND[0]}
          </span>
          <h1 className='text-2xl font-bold'>{BRAND}</h1>
        </div>
        <button
          onClick={() => setRules((r) => !r)}
          aria-expanded={rules}
          className='glass rounded-xl px-4 py-2 font-semibold'
        >
          ⓘ {t("rules")}
        </button>
      </div>
      {rules && <Rules v={RULE_VALUES} />}
      <h2 className='mt-12 text-center text-4xl font-bold leading-tight'>
        {t("welcomeTo")}{" "}
        <span className='bg-gradient-to-r from-yellow-300 to-orange-500 bg-clip-text text-transparent'>
          {BRAND}
        </span>
      </h2>
      <section className='glass glass-gold mt-8 rounded-3xl p-5'>
        <h3 className='text-center font-semibold'>▷ {t("chooseStake")}</h3>
        <button
          onClick={() => onPlay(5)}
          className='gloss mt-6 w-full rounded-2xl bg-gradient-to-r from-[#00d58f] to-[#009a8b] py-4 text-xl font-bold'
        >
          ▷ {t("play5")}
          {rooms && (
            <span className='block text-xs font-medium text-white/80'>
              {t("prizeNow")} {birr(rooms[0].prize)} · {rooms[0].players}/
              {rooms[0].total}
            </span>
          )}
        </button>
        <button
          onClick={() => onPlay(10)}
          className='gloss mt-4 w-full rounded-2xl bg-gradient-to-r from-[#0a9bff] to-[#5a3cf5] py-4 text-xl font-bold'
        >
          ▷ {t("play10")}
          {rooms && (
            <span className='block text-xs font-medium text-white/80'>
              {t("prizeNow")} {birr(rooms[1].prize)} · {rooms[1].players}/
              {rooms[1].total}
            </span>
          )}
        </button>
      </section>
      <section className='glass mt-6 space-y-6 rounded-3xl px-5 py-8 text-center'>
        {[
          [RULE_VALUES.total, t("statSpots")],
          [rooms ? sold : null, t("statSold")],
        ].map(([n, label]) => (
          <div key={label}>
            {n === null ? (
              <Sk className='mx-auto h-9 w-24' />
            ) : (
              <p className='text-4xl font-bold tabular-nums'>{n}</p>
            )}
            <p className='mt-1 text-purple-200'>{label}</p>
          </div>
        ))}
      </section>
      {!rooms && <Waking />}
    </>
  );
}

const Pill = ({ label, value, hi }) => (
  <div
    className={`glass min-w-0 flex-1 rounded-xl px-2 py-1 ${hi ? "text-center" : ""}`}
  >
    <p className='truncate text-[11px] font-semibold text-purple-200'>
      {label}
    </p>
    <p
      className={`truncate font-bold tabular-nums ${hi ? "text-xl text-yellow-300" : "text-sm"}`}
    >
      {value}
    </p>
  </div>
);

// ---------- page 2: only the numbers of this game (no cartela here) ----------
function Pick({ g, me, toggle, now, onBack, onRefresh }) {
  const { t } = useL();
  const left = Math.max(0, Math.ceil((g.deadline - now) / 1000));
  const grid = useMemo(() => {
    const taken = new Set(g.taken);
    const bots = new Set(g.bots || []);
    const mine = new Set(g.mine);
    return Array.from({ length: g.total }, (_, i) => i + 1).map((n) => {
      const isMine = mine.has(n);
      const other = taken.has(n) && !isMine;
      return (
        <button
          key={n}
          onClick={() => toggle(n)}
          disabled={other}
          aria-pressed={isMine}
          className={`relative h-10 rounded-lg border text-sm font-bold tabular-nums ${isMine ? "border-called bg-called text-white" : bots.has(n) ? "border-sky-400 bg-sky-700 text-white" : other ? "border-orange-500 bg-orange-600 text-white" : "border-white/25 bg-white/10 text-white"}`}
        >
          {n}
          {bots.has(n) && !isMine && (
            <span
              className='absolute -right-0.5 -top-1 text-[9px] leading-none'
              aria-label=''
            ></span>
          )}
        </button>
      );
    });
  }, [g.taken, g.bots, g.mine, g.total, toggle]);

  return (
    <>
      <div className='flex items-center justify-between gap-2'>
        <button
          onClick={onBack}
          className='glass rounded-xl px-4 py-2 font-semibold'
        >
          ← {t("back")}
        </button>
        <button
          onClick={onRefresh}
          className='glass rounded-xl px-4 py-2 font-semibold'
        >
          ↻ {t("refresh")}
        </button>
      </div>
      <section className='mt-3 flex gap-2'>
        <Pill label={t("main")} value={me ? birr(me.balance) : "—"} />
        <Pill label={t("bonus")} value={me ? birr(me.bonus) : "—"} />
        <Pill label={t("stake")} value={birr(g.fee)} />
        <Pill label={`${g.mine.length}/${g.cfg.max}`} value={`${left} s`} hi />
      </section>
      {g.taken.length < g.cfg.min && (
        <p className='mt-3 rounded-xl bg-white/10 px-3 py-2 text-sm'>
          {t("need", { min: g.cfg.min, n: g.taken.length })}
        </p>
      )}
      <p className='mt-3 text-sm text-white/70'>
        {t("pickHint", { fee: birr(g.fee) })}
      </p>
      <div className='glass mt-3 max-h-[68dvh] overflow-y-auto rounded-2xl p-2'>
        <div className='grid grid-cols-8 gap-1.5'>{grid}</div>
      </div>
      <p className='mt-2 flex items-center gap-2 text-xs text-white/70'>
        <i className='inline-block size-3 rounded bg-called' />{" "}
        {t("legendMine")}
        <i className='ml-3 inline-block size-3 rounded bg-orange-600' />{" "}
        {t("legendTaken")}
        <i className='ml-3 inline-block size-3 rounded bg-sky-700' />
      </p>
    </>
  );
}

// ---------- page 3: the bingo section (opens by itself when the 30 s are over) ----------
function Bingo({ g, onLeave, onRefresh }) {
  const { t } = useL();
  const set = new Set(g.called);
  const last = g.called[g.called.length - 1];
  const rows = Array.from({ length: 15 }, (_, r) => r);
  return (
    <>
      <section className='flex gap-1.5'>
        <Pill label={t("gameId")} value={g.gid} />
        <Pill label={t("players")} value={g.taken.length} />
        <Pill label={t("bet")} value={birr(g.fee)} />
        <Pill label={t("derash")} value={birr(g.prize)} />
        <Pill label={t("called")} value={g.called.length} />
      </section>
      <div className='mt-3 grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)] items-start gap-2'>
        <section aria-label='1-75' className='glass rounded-2xl p-2'>
          <div className='grid grid-cols-5 gap-1 text-center'>
            {[..."BINGO"].map((l, i) => (
              <span
                key={l}
                className={`rounded-md py-1 text-xs font-bold ${LETTER_BG[i]}`}
              >
                {l}
              </span>
            ))}
            {rows.flatMap((r) =>
              [0, 1, 2, 3, 4].map((c) => {
                const n = c * 15 + r + 1;
                return (
                  <span
                    key={n}
                    className={`grid h-8 place-items-center rounded-md text-xs font-bold tabular-nums ${n === last ? "bg-saffron text-ink" : set.has(n) ? "bg-called text-white" : "bg-white/10 text-white/80"}`}
                  >
                    {n}
                  </span>
                );
              }),
            )}
          </div>
        </section>
        <div className='min-w-0'>
          <section className='glass flex items-center justify-between rounded-xl p-2'>
            {last ? (
              <span
                className={`rounded-full px-3 py-1 text-sm font-bold tabular-nums ${ballBg(last)}`}
              >
                {letterOf(last)}-{last}
              </span>
            ) : (
              <span />
            )}
          </section>
          <section className='glass mt-2 grid place-items-center rounded-xl py-4'>
            {last ? (
              <div
                className={`grid size-24 place-items-center rounded-full ring-4 ring-saffron ${ballBg(last)}`}
              >
                <p className='text-2xl font-bold tabular-nums'>
                  {letterOf(last)}-{last}
                </p>
              </div>
            ) : (
              <p className='py-6 text-lg font-bold'>{t("ready")}</p>
            )}
          </section>
          <section className='glass mt-2 flex items-center justify-between rounded-full px-3 py-1.5 text-sm'>
            <span>{t("auto")}</span>
            <span
              aria-hidden
              className='relative h-5 w-9 rounded-full bg-called'
            >
              <i className='absolute right-0.5 top-0.5 size-4 rounded-full bg-white' />
            </span>
          </section>
          <section className='glass mt-2 rounded-xl p-2'>
            {g.mine.length === 0 ? (
              <>
                <p className='mt-2 text-center text-lg font-bold'>
                  {t("watching")}
                </p>
                <p className='my-3 text-center text-sm text-white/80'>
                  {t("watchingMsg")}
                </p>
              </>
            ) : (
              g.mine.map((n) => (
                <Cartela
                  key={n}
                  n={n}
                  card={g.cards[n]}
                  called={g.called}
                  bg='bg-black/20'
                  compact
                />
              ))
            )}
          </section>
        </div>
      </div>
      <div className='mt-4 grid grid-cols-2 gap-3'>
        <button
          onClick={onLeave}
          className='rounded-2xl bg-gradient-to-r from-rose to-orange-600 py-3 font-bold'
        >
          {t("leave")}
        </button>
        <button
          onClick={onRefresh}
          className='glass rounded-2xl py-3 font-bold'
        >
          ↻ {t("refresh")}
        </button>
      </div>
    </>
  );
}

function Result({ g, me, now }) {
  const { t } = useL();
  const r = g.result;
  const left = Math.max(0, Math.ceil((g.deadline - now) / 1000));
  const next = (
    <p className='mt-4 text-center text-sm text-white/70'>
      {t("nextIn", { n: left })}
    </p>
  );
  if (!r || r.winners.length === 0) {
    return (
      <section className='rounded-3xl bg-panel p-6 text-center'>
        <p className='text-3xl font-bold'>{t("noWinner")}</p>
        <p className='mt-2 text-white/70'>{t("refunded")}</p>
        {next}
      </section>
    );
  }
  const won = r.winners.some((w) => w.uid === me?.id);
  return (
    <section>
      <div className='rounded-3xl bg-panel p-6 text-center'>
        <p className='mx-auto grid size-16 place-items-center rounded-full bg-saffron text-3xl text-ink'>
          ♛
        </p>
        <p className='mt-2 text-3xl font-bold text-saffron'>{t("bingo")}</p>
        {won && <p className='mt-1 font-semibold'>{t("youWon")}</p>}
        <p className='mt-2 text-sm text-white/70'>
          {t("prize")} {birr(r.prize)} {t("birr")}
        </p>
      </div>
      {r.winners.map((w) => (
        <article key={w.cartela} className='mt-3 rounded-3xl bg-panel p-4'>
          <p className='text-xl font-bold'>{w.name}</p>
          <p className='text-sm tabular-nums text-white/70'>
            {w.phone} · {t("winCartela")} #{w.cartela}
          </p>
          <p className='mt-1 text-lg font-bold text-called'>
            {birr(w.share)} {t("birr")}
          </p>
          <Cartela n={w.cartela} card={w.card} called={g.called} bg='bg-ink' />
        </article>
      ))}
      {next}
    </section>
  );
}

function CashbackCard({ c }) {
  const { t } = useL();
  const now = useNow();
  const pct = Math.min(
    100,
    Math.max(0, ((now - c.start) / (c.end - c.start)) * 100),
  );
  const days = Math.max(1, Math.ceil((c.end - now) / 86400000));
  return (
    <section className='rounded-3xl bg-panel p-4'>
      <h2 className='font-semibold'>🎁 {t("cbTitle", { pct: c.percent })}</h2>
      <p className='mt-3 flex justify-between text-sm'>
        <span className='text-white/70'>{t("cbDeposit")}</span>
        <b className='tabular-nums'>
          {birr(c.deposited)} {t("birr")}
        </b>
      </p>
      <p className='mt-1 flex justify-between text-sm'>
        <span className='text-white/70'>{t("cbLost")}</span>
        <b className='tabular-nums'>
          {birr(c.lost)} {t("birr")}
        </b>
      </p>
      <div
        role='progressbar'
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(pct)}
        className='mt-3 h-3 overflow-hidden rounded-full bg-white/15'
      >
        <div
          className='h-full rounded-full bg-gradient-to-r from-mint to-saffron transition-all'
          style={{ width: `${pct}%` }}
        />
      </div>
      <p className='mt-2 flex justify-between text-sm'>
        <span>
          {t("cbSoFar")}:{" "}
          <b className='text-saffron'>
            {birr(c.amount)} {t("birr")}
          </b>
        </span>
        <span className='text-white/60'>{t("cbDays", { n: days })}</span>
      </p>
      <p className='mt-2 text-xs text-white/60'>{t("cbNote")}</p>
    </section>
  );
}

function Wallet({ me, refresh }) {
  const { t } = useL();
  const [phone, setPhone] = useState("");
  const [amount, setAmount] = useState("");
  const [msg, setMsg] = useState("");
  const submit = async (e) => {
    e.preventDefault();
    try {
      await api("/api/withdraw", {
        method: "POST",
        body: JSON.stringify({ phone, amount: Number(amount) }),
      });
      setMsg(t("sent"));
      setAmount("");
      refresh();
    } catch (x) {
      setMsg(x.message);
    }
  };
  const input =
    "mt-1 w-full rounded-xl bg-ink px-3 py-3 outline-none focus:ring-2 focus:ring-saffron";
  return (
    <div className='space-y-5'>
      <section className='grid grid-cols-2 gap-3'>
        <article className='rounded-3xl bg-panel p-4'>
          <p className='text-sm text-white/70'>{t("main")}</p>
          <p className='text-3xl font-bold tabular-nums'>
            {me ? birr(me.balance) : "—"}
          </p>
          <p className='text-xs text-white/60'>{t("withdrawable")}</p>
        </article>
        <article className='rounded-3xl bg-panel p-4'>
          <p className='text-sm text-white/70'>{t("bonus")}</p>
          <p className='text-3xl font-bold tabular-nums text-called'>
            {me ? birr(me.bonus) : "—"}
          </p>
          <p className='text-xs text-white/60'>{t("playOnly")}</p>
        </article>
      </section>
      {me?.cashback && <CashbackCard c={me.cashback} />}
      <section className='rounded-3xl bg-panel p-4'>
        <h2 className='font-semibold'>{t("depositTitle")}</h2>
        <p className='mt-1 text-sm text-white/70'>{t("depositBody")}</p>
        <button
          onClick={() => tg?.close()}
          className='mt-3 w-full rounded-2xl bg-mint py-3 font-bold text-ink'
        >
          {t("toBot")}
        </button>
      </section>
      <form onSubmit={submit} className='rounded-3xl bg-panel p-4'>
        <h2 className='font-semibold'>{t("withdrawTitle")}</h2>
        <label className='mt-3 block text-sm text-white/70' htmlFor='phone'>
          {t("tele")}
        </label>
        <input
          id='phone'
          value={phone}
          onChange={(e) => setPhone(e.target.value)}
          inputMode='tel'
          placeholder='09…'
          required
          className={input}
        />
        <label className='mt-3 block text-sm text-white/70' htmlFor='amount'>
          {t("amount", { min: me ? me.minWithdraw / 100 : 100 })}
        </label>
        <input
          id='amount'
          value={amount}
          onChange={(e) => setAmount(e.target.value)}
          inputMode='numeric'
          required
          className={input}
        />
        <button className='mt-4 w-full rounded-2xl bg-saffron py-3 font-bold text-ink'>
          {t("send")}
        </button>
        {msg && (
          <p role='status' className='mt-2 text-sm text-white/80'>
            {msg}
          </p>
        )}
      </form>
    </div>
  );
}

function History({ me }) {
  const { t } = useL();
  const [txs, setTxs] = useState([]);
  useEffect(() => {
    api("/api/txs")
      .then(setTxs)
      .catch(() => {});
  }, [me?.balance, me?.bonus]);
  if (txs.length === 0)
    return <p className='py-16 text-center text-white/60'>{t("noTx")}</p>;
  return (
    <ul className='divide-y divide-line'>
      {txs.map((x, i) => (
        <li key={i} className='flex items-center justify-between py-3 text-sm'>
          <span>
            {t("k_" + x.kind)}
            <span className='block text-xs text-white/50'>
              {new Date(x.at).toLocaleString()}
            </span>
          </span>
          <span
            className={`font-bold tabular-nums ${x.amount < 0 ? "text-rose" : "text-called"}`}
          >
            {x.amount < 0 ? "-" : "+"}
            {birr(Math.abs(x.amount))}
          </span>
        </li>
      ))}
    </ul>
  );
}

function Profile({ me }) {
  const { t, lang, set } = useL();
  const [copied, setCopied] = useState(false);
  if (!me)
    return <p className='py-16 text-center text-white/60'>{t("loading")}</p>;
  const link = `https://t.me/${me.bot}?start=ref_${me.id}`;
  const copy = async () => {
    await navigator.clipboard.writeText(link).catch(() => {});
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };
  const share = () => {
    const url = `https://t.me/share/url?url=${encodeURIComponent(link)}&text=${encodeURIComponent(t("shareText"))}`;
    if (tg) tg.openTelegramLink(url);
    else window.open(url, "_blank");
  };
  const cards = [
    [t("main"), birr(me.balance)],
    [t("bonus"), birr(me.bonus)],
    [t("wins"), me.gamesWon],
    [t("invites"), me.referrals],
    [t("earned"), birr(me.earned)],
  ];
  return (
    <div className='space-y-5'>
      <section className='flex items-center justify-between rounded-3xl bg-panel p-4'>
        <span className='font-semibold'>{t("language")}</span>
        <div className='flex gap-2'>
          {[
            ["am", "አማርኛ"],
            ["en", "English"],
          ].map(([id, label]) => (
            <button
              key={id}
              onClick={() => set(id)}
              aria-pressed={lang === id}
              className={`rounded-full px-4 py-2 text-sm font-semibold ${lang === id ? "bg-saffron text-ink" : "bg-ink"}`}
            >
              {label}
            </button>
          ))}
        </div>
      </section>
      <section className='text-center'>
        <p className='mx-auto grid size-20 place-items-center rounded-full bg-panel text-3xl font-bold'>
          {me.name?.[0]}
        </p>
        <p className='mt-2 text-xl font-bold'>{me.name}</p>
        <p className='text-sm tabular-nums text-white/60'>{me.phone}</p>
      </section>
      <section className='grid grid-cols-2 gap-3'>
        {cards.map(([label, v]) => (
          <article key={label} className='rounded-3xl bg-panel p-4'>
            <p className='text-sm text-white/70'>{label}</p>
            <p className='text-2xl font-bold tabular-nums'>{v}</p>
          </article>
        ))}
      </section>
      <section className='rounded-3xl bg-panel p-4'>
        <h2 className='font-semibold'>{t("inviteTitle")}</h2>
        <p className='mt-1 text-sm text-white/70'>{t("inviteBody")}</p>
        {me.inviteGoal > 0 && (
          <p className='mt-2 font-semibold text-saffron'>
            {me.refQualified % me.inviteGoal}/{me.inviteGoal} ·{" "}
            {birr(me.inviteReward)} {t("birr")}
          </p>
        )}
        <p className='mt-3 break-all rounded-xl bg-ink p-3 text-xs'>{link}</p>
        <div className='mt-3 grid grid-cols-2 gap-3'>
          <button onClick={copy} className='rounded-2xl bg-line py-3 font-bold'>
            {copied ? t("copied") : t("copy")}
          </button>
          <button
            onClick={share}
            className='rounded-2xl bg-saffron py-3 font-bold text-ink'
          >
            {t("shareBtn")}
          </button>
        </div>
      </section>
    </div>
  );
}

function Player() {
  const { t } = useL();
  const [tab, setTab] = useState("game");
  const [view, setView] = useState("lobby"); // lobby (Play 5 / Play 10) -> play (numbers, then bingo, then winner)
  const [stake, setStake] = useState(null);
  const [me, setMe] = useState(null);
  const now = useNow();
  const rooms = useRooms(tab === "game" && view === "lobby");
  const { g, conn, err, toggle, reconnect } = useGame(
    view === "play" ? stake : null,
  );
  const refresh = useCallback(
    () =>
      api("/api/me")
        .then(setMe)
        .catch(() => {}),
    [],
  );

  useEffect(() => {
    tg?.ready();
    tg?.expand();
  }, []);
  useEffect(() => {
    refresh();
  }, [refresh, g?.phase, g?.mine?.length]);

  const play = (st) => {
    setStake(st);
    setView("play");
  };
  const toLobby = () => setView("lobby");
  const reload = () => {
    reconnect();
    refresh();
  };

  return (
    <div className='mx-auto flex min-h-dvh max-w-md flex-col'>
      {view === "play" && !conn && (
        <p
          role='status'
          className='mx-4 mt-3 rounded-xl bg-white/10 px-3 py-2 text-sm'
        >
          {t("reconnecting")}
        </p>
      )}
      {err && (
        <p
          role='alert'
          className='mx-4 mt-3 rounded-xl bg-rose px-3 py-2 text-sm font-semibold text-ink'
        >
          {t("err_" + err, { max: g?.cfg.max })}
        </p>
      )}
      <main className='flex-1 px-3 pb-24 pt-[max(1rem,env(safe-area-inset-top))]'>
        {tab === "game" &&
          (view === "lobby" ? (
            <Lobby rooms={rooms} onPlay={play} />
          ) : !g ? (
            <GameSkeleton />
          ) : g.phase === "pick" ? (
            <Pick
              g={g}
              me={me}
              toggle={toggle}
              now={now}
              onBack={toLobby}
              onRefresh={reload}
            />
          ) : g.phase === "run" ? (
            <Bingo g={g} onLeave={toLobby} onRefresh={reload} />
          ) : (
            <Result g={g} me={me} now={now} />
          ))}
        {tab === "history" && <History me={me} />}
        {tab === "wallet" && <Wallet me={me} refresh={refresh} />}
        {tab === "profile" && <Profile me={me} />}
      </main>
      <nav className='fixed inset-x-0 bottom-0 mx-auto flex max-w-md border-t border-white/10 bg-[#16123a] pb-[env(safe-area-inset-bottom)]'>
        {["game", "history", "wallet", "profile"].map((id) => (
          <button
            key={id}
            onClick={() => setTab(id)}
            aria-current={tab === id ? "page" : undefined}
            className={`flex-1 py-3 text-sm font-semibold ${tab === id ? "text-saffron" : "text-white/60"}`}
          >
            {t(id)}
          </button>
        ))}
      </nav>
    </div>
  );
}

// ---------- maintenance screen (the owner switches it on from /finance) ----------
function useStatus() {
  const [st, setSt] = useState(null);
  const wasOn = useRef(false);
  useEffect(() => {
    let stop = false;
    const load = () =>
      fetch(API_URL + "/api/status", { cache: "no-store" })
        .then((r) => r.json())
        .then((d) => {
          if (stop) return;
          if (wasOn.current && !d.on) {
            location.reload();
            return;
          } // the game is open again
          wasOn.current = !!d.on;
          setSt({ ...d, off: (d.now || Date.now()) - Date.now() }); // corrects a wrong clock on the phone
        })
        .catch(() => {});
    load();
    const id = setInterval(load, 10000);
    return () => {
      stop = true;
      clearInterval(id);
    };
  }, []);
  return st;
}

function Maintenance({ st }) {
  const { t } = useL();
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);
  const left = st.until ? Math.max(0, st.until - (now + st.off)) : 0;
  const parts = [
    [Math.floor(left / 864e5), "mtDays"],
    [Math.floor(left / 36e5) % 24, "mtHours"],
    [Math.floor(left / 6e4) % 60, "mtMin"],
    [Math.floor(left / 1e3) % 60, "mtSec"],
  ];
  return (
    <main className='mx-auto grid min-h-dvh max-w-md place-items-center p-4'>
      <section className='glass w-full overflow-hidden rounded-[2rem] p-5 text-center'>
        {st.image && (
          <img
            src={`${API_URL}/maintenance/image?v=${st.v}`}
            alt=''
            className='mb-5 max-h-72 w-full rounded-3xl object-cover shadow-lg'
          />
        )}
        <p className='text-4xl' aria-hidden>
          🛠️
        </p>
        <h1 className='mt-2 bg-gradient-to-r from-yellow-300 to-orange-500 bg-clip-text text-3xl font-extrabold leading-tight text-transparent'>
          {st.title}
        </h1>
        {st.message && (
          <p className='mt-3 whitespace-pre-wrap text-white/85'>{st.message}</p>
        )}
        {st.until > 0 && left > 0 ? (
          <div
            className='mt-6 grid grid-cols-4 gap-2'
            role='timer'
            aria-live='off'
          >
            {parts.map(([n, k]) => (
              <div key={k} className='glass rounded-2xl py-3'>
                <p className='text-3xl font-extrabold tabular-nums'>
                  {String(n).padStart(2, "0")}
                </p>
                <p className='text-[11px] text-purple-200'>{t(k)}</p>
              </div>
            ))}
          </div>
        ) : (
          <p className='mt-6 animate-pulse font-semibold text-saffron'>
            {st.until > 0 ? t("mtAlmost") : t("mtSoon")}
          </p>
        )}
        <p className='mt-6 text-sm text-white/60'>{t("mtBot")}</p>
      </section>
    </main>
  );
}

export default function App() {
  const [lang, setLang] = useState(() => {
    try {
      return localStorage.getItem("lang") || "am";
    } catch {
      return "am";
    }
  });
  const ctx = useMemo(
    () => ({
      lang,
      set: (l) => {
        setLang(l);
        try {
          localStorage.setItem("lang", l);
        } catch {
          /* storage unavailable */
        }
      },
      t: (k, v = {}) =>
        (DICT[lang][k] ?? DICT.en[k] ?? k).replace(
          /\{(\w+)\}/g,
          (_, x) => v[x] ?? "",
        ),
    }),
    [lang],
  );
  const st = useStatus();
  if (location.hash === "#admin") return <Admin />;
  if (st?.on)
    return (
      <LangCtx.Provider value={ctx}>
        <Maintenance st={st} />
      </LangCtx.Provider>
    );
  return (
    <LangCtx.Provider value={ctx}>
      <Player />
    </LangCtx.Provider>
  );
}
