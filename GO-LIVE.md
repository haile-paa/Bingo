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
