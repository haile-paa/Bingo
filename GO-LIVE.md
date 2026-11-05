# Big Bingo - go-live checklist

## 1. Environment variables (Render -> Environment)
| Name | What |
|---|---|
| MONGO_URI | MongoDB Atlas connection string (paid/backup-enabled cluster recommended) |
| BOT_TOKEN | from @BotFather |
| BOT_USERNAME | bot username without @ |
| WEBHOOK_SECRET | random, 40+ characters (part of the webhook URL) |
| ADMIN_KEY | random, 40+ characters - it opens the admin panel, keep it secret |
| MINIAPP_URL | your public https URL |
| TELEBIRR_NUMBER / TELEBIRR_NAME | the account players pay into |
| VERIFY_ET_API_KEY | Verify.ET key (permission: verification:write) |
| SUPPORT_USER | support Telegram username |
| OWNER_PHONE / OWNER_PASSWORD | login for the finance dashboard at `/finance` (password 10+ characters, ideally 16+) |
Do NOT set `DEV`. Generate random values with: `openssl rand -hex 24`

## 2. Before the first real player
- [ ] Rotate BOT_TOKEN, ADMIN_KEY and WEBHOOK_SECRET if they were ever pasted in chats, screenshots or git.
- [ ] Atlas: turn on backups, allow only the server's IP (or 0.0.0.0/0 with a strong password), use a database user with access to the `bingo` database only.
- [ ] Render: use a paid instance (a free one sleeps) or ping `/health` every 5 minutes.
- [ ] Make one test deposit, one game, one withdrawal and one admin approval end to end.
- [ ] Set the Telegram webhook to `https://YOUR-APP/bot/WEBHOOK_SECRET`.
- [ ] Verify.ET: keep credits topped up. When credits run out, deposits fall back to manual admin approval.

## 3. Money rules built in
- Deposits go to the Play wallet (bets only). Only prizes are withdrawable.
- Prize = 80% of stakes, company keeps 20%. Games start at 20 cartelas.
- Weekly cashback = 10% of deposit money that was played and lost (Mon-Mon, Addis time).
- Referral: friend must deposit at least 50 birr (`refMinDeposit`) before invite rewards count.
- Max 3 open withdrawal requests per player; admin pays by hand and marks them paid.

## 4. Every day
- Admin panel: approve/reject deposits that fell back to manual, pay withdrawals.
- Watch the Render logs for `PAYOUT FAILED`, `CASHBACK FAILED`, `REFUND FAILED`.

## 5. Finance dashboard (`/finance`)
- Owner-only login with `OWNER_PHONE` + `OWNER_PASSWORD`. Without both variables the page stays off.
- Shows revenue, profit, deposits, withdrawals, players, cash vs. owed, and your own expenses list. It cannot approve deposits or pay withdrawals - keep using the Admin panel for that.
- Wrong password 8 times in 10 minutes locks the login for 10 minutes. Sessions last 12 hours and end when you change the password.

## 6. If the server restarts during a round
Every paid cartela is also saved in the database. If the server stops mid-round, a cleanup job (runs every minute) gives those stakes back after about 6 minutes and sends the player a Telegram message. Prizes are never refunded this way once a round has finished.

## 7. Announcements (`/finance` -> Announce)
- Upload a poster (JPG/PNG up to 5 MB), write the description (up to 1000 characters with a poster, 4000 without) and add an optional button (Play opens the game, or any https link).
- Choose who gets it: all registered players, players who deposited, players not active for 7 days, or only you (test).
- Test first: your owner phone number (`OWNER_PHONE`) must be registered in the bot to receive the test.
- It is sent about 18 messages a second, with a progress bar. Press Stop to cancel. Only one announcement can send at a time, and at most 8 per hour.
- People who blocked the bot show as "blocked". Send announcements sparingly: too many may make players block the bot.

## 8. Maintenance mode (`/finance` -> Maint.)
- Write a title and message, optionally add a picture and a "back online at" time (shows a live countdown), then press **Close the game now**.
- Players see the maintenance screen; the game and the pick/play connection are closed. Rounds that have not started are cancelled and every stake is refunded; rounds already running finish normally and winners are paid.
- The Telegram bot keeps working (deposit, balance, withdraw).
- Press **Reopen the game** to switch it off; players' phones reload by themselves within about 10 seconds.
- If the server restarts while maintenance is on, it stays on (the setting is saved in the database).
