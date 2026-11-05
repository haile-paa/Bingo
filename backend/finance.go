// backend/finance.go - owner-only finance dashboard (served from ../finance at /finance)
//
// Login: OWNER_PHONE + OWNER_PASSWORD (environment variables). Nothing else can open it.
// Everything here is read-only except the expenses list the owner keeps by hand.
package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	finCookie  = "fin_session"
	finIter    = 150000
	finSession = 12 * time.Hour
)

var (
	finOn    bool
	finPhone string
	finSalt  []byte
	finHash  []byte
	finKey   []byte

	finMu    sync.Mutex
	finFails []time.Time // recent wrong logins (all visitors together)
)

// pbkdf2 is PBKDF2-HMAC-SHA256 (RFC 8018), kept here so no extra dependency is needed.
func pbkdf2(password, salt []byte, iter, keyLen int) []byte {
	prf := hmac.New(sha256.New, password)
	var dk []byte
	for block := 1; len(dk) < keyLen; block++ {
		prf.Reset()
		prf.Write(salt)
		prf.Write([]byte{byte(block >> 24), byte(block >> 16), byte(block >> 8), byte(block)})
		u := prf.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < iter; i++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		dk = append(dk, t...)
	}
	return dk[:keyLen]
}

func finToken(exp int64) string {
	msg := strconv.FormatInt(exp, 10)
	return msg + "." + hex.EncodeToString(hmacSHA256(finKey, []byte(msg)))
}

func finTokenOK(tok string) bool {
	msg, sig, ok := strings.Cut(tok, ".")
	if !ok {
		return false
	}
	exp, err := strconv.ParseInt(msg, 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}
	return hmac.Equal([]byte(sig), []byte(hex.EncodeToString(hmacSHA256(finKey, []byte(msg)))))
}

// registerFinance mounts the dashboard. It stays off unless OWNER_PHONE and OWNER_PASSWORD are set.
func registerFinance(r *gin.Engine) {
	phone := normPhone(os.Getenv("OWNER_PHONE"))
	pass := os.Getenv("OWNER_PASSWORD")
	if phone == "" || len(pass) < 10 {
		log.Println("finance dashboard is OFF: set OWNER_PHONE (09...) and OWNER_PASSWORD (at least 10 characters) to turn it on")
		return
	}
	finOn, finPhone = true, phone
	finSalt = make([]byte, 16)
	if _, err := rand.Read(finSalt); err != nil {
		log.Fatal(err)
	}
	finHash = pbkdf2([]byte(pass), finSalt, finIter, 32)
	// sessions die when the password, bot token or webhook secret changes
	finKey = hmacSHA256([]byte("finance-session"), []byte(cfg.BotToken+"|"+cfg.WebhookSecret+"|"+pass))

	fin := r.Group("/finance", finHeaders())
	file := func(name string) gin.HandlerFunc {
		return func(c *gin.Context) { c.File("../finance/" + name) }
	}
	fin.GET("", file("index.html"))
	fin.GET("/style.css", file("style.css"))
	fin.GET("/app.js", file("app.js"))

	api := fin.Group("/api", finCSRF())
	api.POST("/login", finLogin)
	auth := api.Group("", finAuth())
	auth.POST("/logout", func(c *gin.Context) {
		http.SetCookie(c.Writer, &http.Cookie{Name: finCookie, Value: "", Path: "/finance", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		c.JSON(200, gin.H{"ok": true})
	})
	auth.GET("/me", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true, "phone": finPhone[:3] + "****" + finPhone[len(finPhone)-3:]})
	})
	auth.GET("/summary", finSummary)
	auth.GET("/people", finPeople)
	auth.GET("/people/:id", finPerson)
	auth.GET("/ledger", finLedger)
	auth.GET("/pending", finPending)
	auth.GET("/expenses", finExpenses)
	auth.POST("/expenses", finAddExpense)
	auth.DELETE("/expenses/:id", finDelExpense)
	auth.GET("/announce/info", annInfo)
	auth.GET("/announce/audience", annAudience)
	auth.GET("/announce", annList)
	auth.POST("/announce/send", annSend)
	auth.POST("/announce_cancel/:id", annCancelH)
	annRecover()
	log.Println("finance dashboard is ON at /finance")
}

func finHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Cache-Control", "no-store")
		h.Set("X-Robots-Tag", "noindex, nofollow")
		c.Next()
	}
}

// finCSRF: every request that changes something must come from our own page.
func finCSRF() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet {
			if c.GetHeader("X-Finance") != "1" {
				c.AbortWithStatusJSON(403, gin.H{"error": "forbidden"})
				return
			}
			if o := c.GetHeader("Origin"); o != "" {
				if u, err := url.Parse(o); err != nil || !strings.EqualFold(u.Host, c.Request.Host) {
					c.AbortWithStatusJSON(403, gin.H{"error": "forbidden"})
					return
				}
			}
		}
		c.Next()
	}
}

func finAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		tok, err := c.Cookie(finCookie)
		if err != nil || !finTokenOK(tok) {
			c.AbortWithStatusJSON(401, gin.H{"error": "login required"})
			return
		}
		c.Next()
	}
}

func finBlocked() bool {
	finMu.Lock()
	defer finMu.Unlock()
	cut := time.Now().Add(-10 * time.Minute)
	i := 0
	for i < len(finFails) && finFails[i].Before(cut) {
		i++
	}
	finFails = finFails[i:]
	return len(finFails) >= 8 // 8 wrong tries in 10 minutes locks login for everybody until the window passes
}

func finLogin(c *gin.Context) {
	if finBlocked() {
		c.JSON(429, gin.H{"error": "Too many wrong attempts. Try again in 10 minutes."})
		return
	}
	var in struct {
		Phone    string `json:"phone"`
		Password string `json:"password"`
	}
	if c.ShouldBindJSON(&in) != nil || len(in.Password) > 200 {
		c.JSON(400, gin.H{"error": "Enter your phone number and password."})
		return
	}
	phone := normPhone(in.Phone)
	got := pbkdf2([]byte(in.Password), finSalt, finIter, 32) // always computed, so timing does not reveal which part was wrong
	okPass := subtle.ConstantTimeCompare(got, finHash) == 1
	okPhone := phone != "" && subtle.ConstantTimeCompare([]byte(phone), []byte(finPhone)) == 1
	if !okPass || !okPhone {
		finMu.Lock()
		finFails = append(finFails, time.Now())
		finMu.Unlock()
		time.Sleep(500 * time.Millisecond)
		c.JSON(401, gin.H{"error": "Wrong phone number or password."})
		return
	}
	exp := time.Now().Add(finSession)
	secure := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
	http.SetCookie(c.Writer, &http.Cookie{Name: finCookie, Value: finToken(exp.Unix()), Path: "/finance", Expires: exp,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode})
	c.JSON(200, gin.H{"ok": true})
}

// ---------- helpers ----------

func num(v any) int64 {
	switch x := v.(type) {
	case int32:
		return int64(x)
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	}
	return 0
}

func dayExpr(field string) bson.M {
	return bson.M{"$dateToString": bson.M{"format": "%Y-%m-%d", "date": "$" + field, "timezone": "Africa/Addis_Ababa"}}
}

func finAgg(ctx context.Context, coll string, pipe []bson.D) []bson.M {
	cur, err := db.Collection(coll).Aggregate(ctx, pipe)
	if err != nil {
		log.Println("finance agg:", coll, err)
		return nil
	}
	var out []bson.M
	if err := cur.All(ctx, &out); err != nil {
		log.Println("finance agg read:", coll, err)
		return nil
	}
	return out
}

func stage(op string, v any) bson.D { return bson.D{{Key: op, Value: v}} }

// finRange reads ?from=YYYY-MM-DD&to=YYYY-MM-DD (Addis Ababa days, both included); default: last 30 days.
func finRange(c *gin.Context) (time.Time, time.Time, []string) {
	now := time.Now().In(eat)
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, eat).AddDate(0, 0, 1) // exclusive
	start := end.AddDate(0, 0, -30)
	if t, err := time.ParseInLocation("2006-01-02", c.Query("from"), eat); err == nil {
		start = t
	}
	if t, err := time.ParseInLocation("2006-01-02", c.Query("to"), eat); err == nil {
		end = t.AddDate(0, 0, 1)
	}
	if end.Sub(start) > 400*24*time.Hour {
		start = end.AddDate(0, 0, -400)
	}
	if !end.After(start) {
		start = end.AddDate(0, 0, -1)
	}
	var days []string
	for d := start; d.Before(end); d = d.AddDate(0, 0, 1) {
		days = append(days, d.Format("2006-01-02"))
	}
	return start, end, days
}

func finNames(ctx context.Context, ids []int64) map[int64]string {
	out := map[int64]string{}
	if len(ids) == 0 {
		return out
	}
	cur, err := db.Collection("users").Find(ctx, bson.M{"_id": bson.M{"$in": ids}}, options.Find().SetProjection(bson.M{"name": 1}))
	if err != nil {
		return out
	}
	var us []User
	if cur.All(ctx, &us) == nil {
		for _, u := range us {
			out[u.ID] = u.Name
		}
	}
	return out
}

type dayRow struct {
	Day           string `json:"day"`
	Stakes        int64  `json:"stakes"`
	Prizes        int64  `json:"prizes"`
	GameRefunds   int64  `json:"gameRefunds"`
	Entries       int64  `json:"entries"`
	Deposits      int64  `json:"deposits"`
	DepositCount  int64  `json:"depositCount"`
	Withdrawn     int64  `json:"withdrawn"`
	WithdrawCount int64  `json:"withdrawCount"`
	Signup        int64  `json:"signup"`
	Referral      int64  `json:"referral"`
	Cashback      int64  `json:"cashback"`
	NewUsers      int64  `json:"newUsers"`
	Expenses      int64  `json:"expenses"`
}

// ---------- /summary ----------

func finSummary(c *gin.Context) {
	ctx := c.Request.Context()
	from, to, days := finRange(c)
	rows := make(map[string]*dayRow, len(days))
	list := make([]*dayRow, 0, len(days))
	for _, d := range days {
		r := &dayRow{Day: d}
		rows[d] = r
		list = append(list, r)
	}
	inRange := bson.M{"$gte": from, "$lt": to}

	// money moving through the game, per day
	for _, g := range finAgg(ctx, "txs", []bson.D{
		stage("$match", bson.M{"at": inRange}),
		stage("$group", bson.M{
			"_id": bson.M{"day": dayExpr("at"), "kind": "$kind",
				"long": bson.M{"$gt": bson.A{bson.M{"$strLenCP": bson.M{"$ifNull": bson.A{"$ref", ""}}}, 6}}},
			"sum": bson.M{"$sum": "$amount"}, "n": bson.M{"$sum": 1}}),
	}) {
		id := asDoc(g["_id"])
		r := rows[asString(id["day"])]
		if r == nil {
			continue
		}
		kind, long := asString(id["kind"]), id["long"] == true
		sum, n := num(g["sum"]), num(g["n"])
		switch kind {
		case "entry":
			r.Stakes += -sum
			r.Entries += n
		case "win":
			r.Prizes += sum
		case "refund":
			if !long { // a stake given back (the long ones are rejected withdrawals)
				r.GameRefunds += sum
			}
		case "deposit":
			r.Deposits += sum
			r.DepositCount += n
		case "signup_bonus":
			r.Signup += sum
		case "referral":
			r.Referral += sum
		case "cashback":
			r.Cashback += sum
		}
	}
	// withdrawals the owner has paid out
	for _, g := range finAgg(ctx, "withdrawals", []bson.D{
		stage("$match", bson.M{"status": "paid", "decidedAt": inRange}),
		stage("$group", bson.M{"_id": dayExpr("decidedAt"), "sum": bson.M{"$sum": "$amount"}, "n": bson.M{"$sum": 1}}),
	}) {
		if r := rows[asString(g["_id"])]; r != nil {
			r.Withdrawn, r.WithdrawCount = num(g["sum"]), num(g["n"])
		}
	}
	for _, g := range finAgg(ctx, "users", []bson.D{
		stage("$match", bson.M{"createdAt": inRange}),
		stage("$group", bson.M{"_id": dayExpr("createdAt"), "n": bson.M{"$sum": 1}}),
	}) {
		if r := rows[asString(g["_id"])]; r != nil {
			r.NewUsers = num(g["n"])
		}
	}
	expCats := map[string]int64{}
	for _, g := range finAgg(ctx, "expenses", []bson.D{
		stage("$match", bson.M{"date": bson.M{"$gte": days[0], "$lte": days[len(days)-1]}}),
		stage("$group", bson.M{"_id": bson.M{"day": "$date", "cat": "$category"}, "sum": bson.M{"$sum": "$amount"}}),
	}) {
		id := asDoc(g["_id"])
		if r := rows[asString(id["day"])]; r != nil {
			r.Expenses += num(g["sum"])
		}
		expCats[asString(id["cat"])] += num(g["sum"])
	}

	// how many bets of each size
	byStake := []gin.H{}
	for _, g := range finAgg(ctx, "txs", []bson.D{
		stage("$match", bson.M{"at": inRange, "kind": "entry"}),
		stage("$group", bson.M{"_id": "$amount", "n": bson.M{"$sum": 1}}),
		stage("$sort", bson.M{"_id": 1}),
	}) {
		fee := -num(g["_id"])
		byStake = append(byStake, gin.H{"fee": fee, "entries": num(g["n"]), "stakes": fee * num(g["n"])})
	}

	top := func(kind string) []gin.H {
		out := []gin.H{}
		var ids []int64
		res := finAgg(ctx, "txs", []bson.D{
			stage("$match", bson.M{"at": inRange, "kind": kind}),
			stage("$group", bson.M{"_id": "$uid", "sum": bson.M{"$sum": "$amount"}, "n": bson.M{"$sum": 1}}),
			stage("$sort", bson.M{"sum": -1}),
			stage("$limit", 5),
		})
		for _, g := range res {
			ids = append(ids, num(g["_id"]))
		}
		names := finNames(ctx, ids)
		for _, g := range res {
			id := num(g["_id"])
			out = append(out, gin.H{"id": id, "name": names[id], "amount": num(g["sum"]), "count": num(g["n"])})
		}
		return out
	}

	// where things stand right now (not limited to the dates)
	one := func(coll string, match bson.M, field string) (int64, int64) {
		res := finAgg(ctx, coll, []bson.D{stage("$match", match), stage("$group", bson.M{"_id": nil, "sum": bson.M{"$sum": "$" + field}, "n": bson.M{"$sum": 1}})})
		if len(res) == 0 {
			return 0, 0
		}
		return num(res[0]["sum"]), num(res[0]["n"])
	}
	depAll, _ := one("deposits", bson.M{"status": "approved"}, "amount")
	wdAll, _ := one("withdrawals", bson.M{"status": "paid"}, "amount")
	expAll, _ := one("expenses", bson.M{}, "amount")
	pendDepSum, pendDepN := one("deposits", bson.M{"status": "pending"}, "amount")
	pendWdSum, pendWdN := one("withdrawals", bson.M{"status": "pending"}, "amount")
	var mainSum, playSum int64
	if res := finAgg(ctx, "users", []bson.D{stage("$group", bson.M{"_id": nil, "main": bson.M{"$sum": "$balance"}, "play": bson.M{"$sum": "$bonus"}})}); len(res) > 0 {
		mainSum, playSum = num(res[0]["main"]), num(res[0]["play"])
	}
	usersN, _ := db.Collection("users").CountDocuments(ctx, bson.M{})
	regN, _ := db.Collection("users").CountDocuments(ctx, bson.M{"phone": bson.M{"$exists": true}})
	var cashbackOwed int64
	if cur, err := db.Collection("cashback").Find(ctx, bson.M{"paid": bson.M{"$ne": true}}); err == nil {
		var ws []CashbackWeek
		if cur.All(ctx, &ws) == nil {
			for _, w := range ws {
				cashbackOwed += w.cashback()
			}
		}
	}

	c.JSON(200, gin.H{
		"from": days[0], "to": days[len(days)-1], "days": list,
		"byStake": byStake, "topWinners": top("win"), "topDepositors": top("deposit"),
		"expenseByCategory": expCats,
		"now": gin.H{
			"users": usersN, "registered": regN, "online": onlineTotal(),
			"mainWallets": mainSum, "playWallets": playSum,
			"depositsAllTime": depAll, "withdrawnAllTime": wdAll, "expensesAllTime": expAll,
			"pendingDeposits":    gin.H{"amount": pendDepSum, "count": pendDepN},
			"pendingWithdrawals": gin.H{"amount": pendWdSum, "count": pendWdN},
			"cashbackOwed":       cashbackOwed,
		},
	})
}

// asDoc reads a nested document whether the driver returned it as bson.M or bson.D.
func asDoc(v any) bson.M {
	switch x := v.(type) {
	case bson.M:
		return x
	case bson.D:
		m := bson.M{}
		for _, e := range x {
			m[e.Key] = e.Value
		}
		return m
	}
	return bson.M{}
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

// ---------- /people ----------

type pRow struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Phone        string `json:"phone"`
	Balance      int64  `json:"balance"`
	Bonus        int64  `json:"bonus"`
	Earned       int64  `json:"earned"`
	GamesWon     int64  `json:"gamesWon"`
	Refs         int64  `json:"refs"`
	RefQualified int64  `json:"refQualified"`
	Joined       int64  `json:"joined"`
	Deposits     int64  `json:"deposits"`
	Withdrawn    int64  `json:"withdrawn"`
	Stakes       int64  `json:"stakes"`
	Prizes       int64  `json:"prizes"`
	Entries      int64  `json:"entries"`
	HouseProfit  int64  `json:"houseProfit"` // stakes - prizes - given-back stakes
}

var (
	peopleMu    sync.Mutex
	peopleCache []pRow
	peopleAt    time.Time
)

func loadPeople(ctx context.Context, fresh bool) []pRow {
	peopleMu.Lock()
	defer peopleMu.Unlock()
	if !fresh && peopleCache != nil && time.Since(peopleAt) < time.Minute {
		return peopleCache
	}
	cur, err := db.Collection("users").Find(ctx, bson.M{})
	if err != nil {
		log.Println("finance people:", err)
		return peopleCache
	}
	var us []User
	if err := cur.All(ctx, &us); err != nil {
		return peopleCache
	}
	idx := make(map[int64]*pRow, len(us))
	out := make([]pRow, len(us))
	for i, u := range us {
		out[i] = pRow{ID: u.ID, Name: u.Name, Phone: u.Phone, Balance: u.Balance, Bonus: u.Bonus, Earned: u.Earned,
			GamesWon: u.GamesWon, RefQualified: u.RefQualified, Joined: u.CreatedAt.UnixMilli()}
		idx[u.ID] = &out[i]
	}
	for _, g := range finAgg(ctx, "txs", []bson.D{
		stage("$match", bson.M{"kind": bson.M{"$in": bson.A{"entry", "win", "refund", "deposit"}}}),
		stage("$group", bson.M{"_id": bson.M{"uid": "$uid", "kind": "$kind",
			"long": bson.M{"$gt": bson.A{bson.M{"$strLenCP": bson.M{"$ifNull": bson.A{"$ref", ""}}}, 6}}},
			"sum": bson.M{"$sum": "$amount"}, "n": bson.M{"$sum": 1}}),
	}) {
		id := asDoc(g["_id"])
		p := idx[num(id["uid"])]
		if p == nil {
			continue
		}
		sum := num(g["sum"])
		switch asString(id["kind"]) {
		case "entry":
			p.Stakes += -sum
			p.Entries += num(g["n"])
		case "win":
			p.Prizes += sum
		case "refund":
			if id["long"] != true {
				p.HouseProfit += -sum // a given-back stake lowers what the house kept
			}
		case "deposit":
			p.Deposits += sum
		}
	}
	for _, g := range finAgg(ctx, "withdrawals", []bson.D{
		stage("$match", bson.M{"status": "paid"}),
		stage("$group", bson.M{"_id": "$uid", "sum": bson.M{"$sum": "$amount"}}),
	}) {
		if p := idx[num(g["_id"])]; p != nil {
			p.Withdrawn = num(g["sum"])
		}
	}
	for _, g := range finAgg(ctx, "users", []bson.D{
		stage("$match", bson.M{"referredBy": bson.M{"$gt": 0}}),
		stage("$group", bson.M{"_id": "$referredBy", "n": bson.M{"$sum": 1}}),
	}) {
		if p := idx[num(g["_id"])]; p != nil {
			p.Refs = num(g["n"])
		}
	}
	for i := range out {
		out[i].HouseProfit += out[i].Stakes - out[i].Prizes
	}
	peopleCache, peopleAt = out, time.Now()
	return out
}

func finPeople(c *gin.Context) {
	rows := loadPeople(c.Request.Context(), c.Query("fresh") == "1")
	q := strings.ToLower(strings.TrimSpace(c.Query("q")))
	filtered := make([]pRow, 0, len(rows))
	for _, r := range rows {
		if q == "" || strings.Contains(strings.ToLower(r.Name), q) || strings.Contains(r.Phone, q) || strconv.FormatInt(r.ID, 10) == q {
			filtered = append(filtered, r)
		}
	}
	key := map[string]func(p pRow) int64{
		"joined": func(p pRow) int64 { return p.Joined }, "balance": func(p pRow) int64 { return p.Balance },
		"bonus": func(p pRow) int64 { return p.Bonus }, "deposits": func(p pRow) int64 { return p.Deposits },
		"withdrawn": func(p pRow) int64 { return p.Withdrawn }, "stakes": func(p pRow) int64 { return p.Stakes },
		"prizes": func(p pRow) int64 { return p.Prizes }, "houseProfit": func(p pRow) int64 { return p.HouseProfit },
		"entries": func(p pRow) int64 { return p.Entries },
	}[c.DefaultQuery("sort", "joined")]
	if key == nil {
		key = func(p pRow) int64 { return p.Joined }
	}
	desc := c.DefaultQuery("order", "desc") != "asc"
	sort.SliceStable(filtered, func(i, j int) bool {
		if desc {
			return key(filtered[i]) > key(filtered[j])
		}
		return key(filtered[i]) < key(filtered[j])
	})
	size, _ := strconv.Atoi(c.DefaultQuery("size", "25"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, page = max(1, min(size, 100)), max(1, page)
	var sum pRow
	for _, r := range filtered {
		sum.Balance += r.Balance
		sum.Bonus += r.Bonus
		sum.Deposits += r.Deposits
		sum.Withdrawn += r.Withdrawn
		sum.Stakes += r.Stakes
		sum.Prizes += r.Prizes
		sum.HouseProfit += r.HouseProfit
	}
	lo := min((page-1)*size, len(filtered))
	hi := min(lo+size, len(filtered))
	c.JSON(200, gin.H{"total": len(filtered), "page": page, "size": size, "rows": filtered[lo:hi], "sums": sum})
}

type txRow struct {
	UID    int64     `bson:"uid" json:"uid"`
	Name   string    `bson:"-" json:"name"`
	Kind   string    `bson:"kind" json:"kind"`
	Amount int64     `bson:"amount" json:"amount"`
	Ref    string    `bson:"ref" json:"ref"`
	At     time.Time `bson:"at" json:"at"`
}

type payRow struct {
	Amount    int64     `bson:"amount" json:"amount"`
	Status    string    `bson:"status" json:"status"`
	Phone     string    `bson:"phone,omitempty" json:"phone,omitempty"`
	TxnID     string    `bson:"txnId,omitempty" json:"txnId,omitempty"`
	UID       int64     `bson:"uid" json:"uid"`
	Name      string    `bson:"-" json:"name"`
	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
}

func finPerson(c *gin.Context) {
	ctx := c.Request.Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "bad id"})
		return
	}
	var row *pRow
	for _, r := range loadPeople(ctx, false) {
		if r.ID == id {
			r := r
			row = &r
			break
		}
	}
	if row == nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	var txs []txRow
	if cur, err := db.Collection("txs").Find(ctx, bson.M{"uid": id}, options.Find().SetSort(bson.D{{Key: "at", Value: -1}}).SetLimit(100)); err == nil {
		_ = cur.All(ctx, &txs)
	}
	get := func(coll string) []payRow {
		var out []payRow
		if cur, err := db.Collection(coll).Find(ctx, bson.M{"uid": id}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(20).
			SetProjection(bson.M{"sms": 0})); err == nil {
			_ = cur.All(ctx, &out)
		}
		return out
	}
	c.JSON(200, gin.H{"user": row, "txs": txs, "deposits": get("deposits"), "withdrawals": get("withdrawals")})
}

// ---------- /ledger and /pending ----------

func finLedger(c *gin.Context) {
	ctx := c.Request.Context()
	filter := bson.M{}
	if k := c.Query("kind"); k != "" {
		filter["kind"] = k
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	limit = max(1, min(limit, 300))
	var txs []txRow
	cur, err := db.Collection("txs").Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "at", Value: -1}}).SetLimit(int64(limit)))
	if err != nil || cur.All(ctx, &txs) != nil {
		c.JSON(500, gin.H{"error": "server error"})
		return
	}
	ids := make([]int64, 0, len(txs))
	for _, t := range txs {
		ids = append(ids, t.UID)
	}
	names := finNames(ctx, ids)
	for i := range txs {
		txs[i].Name = names[txs[i].UID]
	}
	c.JSON(200, gin.H{"rows": txs})
}

func finPending(c *gin.Context) {
	ctx := c.Request.Context()
	get := func(coll string) []payRow {
		var out []payRow
		cur, err := db.Collection(coll).Find(ctx, bson.M{"status": "pending"}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}).SetLimit(50).
			SetProjection(bson.M{"sms": 0}))
		if err != nil || cur.All(ctx, &out) != nil {
			return nil
		}
		ids := make([]int64, 0, len(out))
		for _, p := range out {
			ids = append(ids, p.UID)
		}
		names := finNames(ctx, ids)
		for i := range out {
			out[i].Name = names[out[i].UID]
		}
		return out
	}
	c.JSON(200, gin.H{"deposits": get("deposits"), "withdrawals": get("withdrawals")})
}

// ---------- expenses (kept by hand by the owner) ----------

type Expense struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Date      string             `bson:"date" json:"date"`
	Category  string             `bson:"category" json:"category"`
	Note      string             `bson:"note" json:"note"`
	Amount    int64              `bson:"amount" json:"amount"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
}

func finExpenses(c *gin.Context) {
	ctx := c.Request.Context()
	from, to, days := finRange(c)
	_, _ = from, to
	var out []Expense
	cur, err := db.Collection("expenses").Find(ctx, bson.M{"date": bson.M{"$gte": days[0], "$lte": days[len(days)-1]}},
		options.Find().SetSort(bson.D{{Key: "date", Value: -1}, {Key: "createdAt", Value: -1}}).SetLimit(500))
	if err != nil || cur.All(ctx, &out) != nil {
		c.JSON(500, gin.H{"error": "server error"})
		return
	}
	if out == nil {
		out = []Expense{}
	}
	c.JSON(200, gin.H{"rows": out})
}

func finAddExpense(c *gin.Context) {
	var in struct {
		Date     string  `json:"date"`
		Category string  `json:"category"`
		Note     string  `json:"note"`
		Amount   float64 `json:"amount"` // birr
	}
	if c.ShouldBindJSON(&in) != nil {
		c.JSON(400, gin.H{"error": "bad request"})
		return
	}
	if _, err := time.Parse("2006-01-02", in.Date); err != nil {
		c.JSON(400, gin.H{"error": "Pick a valid date."})
		return
	}
	cat, note := strings.TrimSpace(in.Category), strings.TrimSpace(in.Note)
	if cat == "" || len(cat) > 40 || len(note) > 200 || in.Amount <= 0 || in.Amount > 100000000 {
		c.JSON(400, gin.H{"error": "Check the category and the amount."})
		return
	}
	e := Expense{Date: in.Date, Category: cat, Note: note, Amount: int64(in.Amount*100 + 0.5), CreatedAt: time.Now()}
	res, err := db.Collection("expenses").InsertOne(c.Request.Context(), e)
	if err != nil {
		c.JSON(500, gin.H{"error": "server error"})
		return
	}
	e.ID, _ = res.InsertedID.(primitive.ObjectID)
	c.JSON(200, e)
}

func finDelExpense(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"error": "bad id"})
		return
	}
	if _, err := db.Collection("expenses").DeleteOne(c.Request.Context(), bson.M{"_id": id}); err != nil {
		c.JSON(500, gin.H{"error": "server error"})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
