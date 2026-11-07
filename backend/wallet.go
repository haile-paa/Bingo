// backend/wallet.go
package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// All money is stored as integer santim (1 birr = 100).
const (
	minDeposit    int64 = 1000
	minWithdraw   int64 = 10000
	signupBonus   int64 = 1000
	refMinDeposit int64 = 1000  // an invited friend counts only after a first deposit of at least 10 birr (stops fake-account farming)
	referralBonus int64 = 1000  // per invited friend whose first deposit is approved (set 0 to turn off)
	inviteGoal    int64 = 20    // every 20 such friends ...
	inviteReward  int64 = 15000 // ... pay 150 birr into the play wallet
	payoutPercent int64 = 80 // players share 80% of the stakes, the company keeps 20%
	totalSpots          = 300 // cartelas per room
	minSpots            = 30 // fewest cartelas needed to start a round
	maxPerUser          = 2  // cartelas one person can hold per round
)

var (
	amtRe   = regexp.MustCompile(`(?i)ETB\s*([\d,]+(?:\.\d{1,2})?)`)
	txnRe   = regexp.MustCompile(`(?i)transaction\s*(?:number|no\.?|id)\s*(?:is|:)?\s*([A-Z0-9]{8,14})`)
	// the receipt link Telebirr sends: https://transactioninfo.ethiotelecom.et/receipt/DJU7HRHTVL
	linkRe  = regexp.MustCompile(`(?i)transactioninfo\.ethiotelecom\.et/receipt/([A-Z0-9]{8,14})`)
	bareRe  = regexp.MustCompile(`^[A-Z0-9]{8,14}$`) // just the transaction number, e.g. DJU7HRHTVL
	phoneRe = regexp.MustCompile(`^0[79]\d{8}$`)
)

type User struct {
	ID         int64     `bson:"_id,omitempty"`
	Name       string    `bson:"name"`
	Balance    int64     `bson:"balance"`
	Bonus      int64     `bson:"bonus"`
	ReferredBy int64     `bson:"referredBy"`
	RefPaid    bool      `bson:"refPaid"`
	RefQualified int64   `bson:"refQualified"` // invited friends whose first deposit was approved
	State      string    `bson:"state,omitempty"`
	Phone      string    `bson:"phone,omitempty"`
	GamesWon   int64     `bson:"gamesWon"`
	Earned     int64     `bson:"earned"`
	CreatedAt  time.Time `bson:"createdAt"`
	Blocked     bool      `bson:"blocked,omitempty"`     // blocked from the whole game by the owner (see blocklist.go)
	BlockedAt   time.Time `bson:"blockedAt,omitempty"`
	BlockReason string    `bson:"blockReason,omitempty"`
}

type Deposit struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	UID       int64              `bson:"uid"`
	Amount    int64              `bson:"amount"`
	TxnID     string             `bson:"txnId"`
	SMS       string             `bson:"sms"`
	ReceiverOK bool              `bson:"receiverOk"`
	Status    string             `bson:"status"`
	CreatedAt time.Time          `bson:"createdAt"`
}

type Withdrawal struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	UID       int64              `bson:"uid"`
	Amount    int64              `bson:"amount"`
	Phone     string             `bson:"phone"`
	Status    string             `bson:"status"`
	CreatedAt time.Time          `bson:"createdAt"`
}

type Tx struct {
	Kind   string    `bson:"kind" json:"kind"`
	Amount int64     `bson:"amount" json:"amount"`
	At     time.Time `bson:"at" json:"at"`
}

func birr(a int64) string { return fmt.Sprintf("%.2f", float64(a)/100) }

func ensureIndexes(ctx context.Context) {
	add := func(coll string, keys bson.D, opts *options.IndexOptions) {
		if _, err := db.Collection(coll).Indexes().CreateOne(ctx, mongo.IndexModel{Keys: keys, Options: opts}); err != nil {
			log.Fatal("index ", coll, ": ", err)
		}
	}
	byStatus := bson.D{{Key: "status", Value: 1}, {Key: "createdAt", Value: 1}}
	add("deposits", bson.D{{Key: "txnId", Value: 1}}, options.Index().SetUnique(true))
	add("deposits", byStatus, nil)
	add("withdrawals", byStatus, nil)
	add("txs", bson.D{{Key: "uid", Value: 1}, {Key: "at", Value: -1}}, nil)
	add("spots", bson.D{{Key: "tok", Value: 1}}, options.Index().SetUnique(true))
	add("spots", bson.D{{Key: "at", Value: 1}}, nil)
	add("expenses", bson.D{{Key: "date", Value: -1}}, nil)
	add("withdrawals", bson.D{{Key: "status", Value: 1}, {Key: "decidedAt", Value: -1}}, nil)
	add("cashback", bson.D{{Key: "uid", Value: 1}, {Key: "week", Value: 1}}, options.Index().SetUnique(true))
	add("users", bson.D{{Key: "referredBy", Value: 1}}, nil)
	add("users", bson.D{{Key: "phone", Value: 1}}, options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"phone": bson.M{"$exists": true}}))
}

func addTx(ctx context.Context, uid int64, kind string, amount int64, ref string) {
	_, err := db.Collection("txs").InsertOne(ctx, bson.M{"uid": uid, "kind": kind, "amount": amount, "ref": ref, "at": time.Now()})
	if err != nil {
		log.Println("tx log:", err)
	}
}

func credit(ctx context.Context, uid int64, field string, amt int64) error {
	_, err := db.Collection("users").UpdateOne(ctx, bson.M{"_id": uid}, bson.M{"$inc": bson.M{field: amt}})
	return err
}

// spend takes the entry fee from bonus first, then cash balance. It returns the wallet used ("" if too poor).
func spend(ctx context.Context, uid, amt int64) (string, error) {
	for _, field := range []string{"bonus", "balance"} {
		res, err := db.Collection("users").UpdateOne(ctx, bson.M{"_id": uid, field: bson.M{"$gte": amt}}, bson.M{"$inc": bson.M{field: -amt}})
		if err != nil {
			return "", err
		}
		if res.ModifiedCount == 1 {
			return field, nil
		}
	}
	return "", nil
}

func ensureUser(ctx context.Context, id int64, name string, ref int64) (bool, error) {
	users := db.Collection("users")
	if ref == id {
		ref = 0
	}
	if ref != 0 {
		if n, err := users.CountDocuments(ctx, bson.M{"_id": ref}); err != nil || n == 0 {
			ref = 0
		}
	}
	res, err := users.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$setOnInsert": bson.M{
		"name": name, "balance": int64(0), "bonus": int64(0), "gamesWon": int64(0), "earned": int64(0), "referredBy": ref, "refPaid": false, "createdAt": time.Now(),
	}}, options.Update().SetUpsert(true))
	if err != nil {
		return false, err
	}
	return res.UpsertedCount == 1, nil
}

// txnFrom finds the Telebirr transaction number in what the user pasted:
// the full SMS, the receipt link, or the bare number.
func txnFrom(text string) string {
	if m := txnRe.FindStringSubmatch(text); m != nil {
		return strings.ToUpper(m[1])
	}
	if m := linkRe.FindStringSubmatch(text); m != nil {
		return strings.ToUpper(m[1])
	}
	if t := strings.ToUpper(strings.TrimSpace(text)); bareRe.MatchString(t) && strings.ContainsAny(t, "0123456789") {
		return t
	}
	return ""
}

const (
	msgBadDeposit = "መልዕክቱን ማንበብ አልተቻለም። የቴሌብር SMS ሙሉውን፣ የደረሰኝ ሊንኩን ወይም የትራንዛክሽን ቁጥሩን paste ያድርጉ።"
	msgDupDeposit = "ይህ ትራንዛክሽን ቀድሞ ተልኳል።"
)

// createDeposit handles what the user pasted after choosing an amount.
// With VERIFY_ET_API_KEY set the receipt is checked by Verify.ET and credited at once;
// otherwise (or when Verify.ET is unavailable) the deposit waits for admin approval.
// declared is the amount the user typed earlier; it is only a fallback for manual review.
func createDeposit(ctx context.Context, uid int64, text string, declared int64) (string, bool) {
	if len(text) > 1000 {
		return msgBadDeposit, false
	}
	tx := txnFrom(text)
	if tx == "" {
		return msgBadDeposit, false
	}
	if n, err := db.Collection("deposits").CountDocuments(ctx, bson.M{"txnId": tx}); err == nil && n > 0 {
		return msgDupDeposit, false
	}
	if !rl.allow("dep:"+strconv.FormatInt(uid, 10), 15, time.Hour) {
		return "ብዙ ሙከራ አድርገዋል። ከጥቂት ጊዜ በኋላ እንደገና ይሞክሩ።", false
	}
	if cfg.VerifyETKey != "" {
		res := verifyTelebirr(ctx, uid, tx)
		switch res.Outcome {
		case verifyOK:
			return autoDeposit(ctx, uid, text, tx, declared, res.Item)
		case verifyInvalid:
			return res.Message, false
		case verifyBusy:
			return "እባክዎ ጥቂት ሰከንዶች ቆይተው እንደገና ይሞክሩ።", false
		}
		// verifyDown (Verify.ET unreachable, out of credits, still queued): the admin decides by hand
	}
	return queueDeposit(ctx, uid, text, tx, declared)
}

// storedText is what the admin sees: the SMS, or the receipt link when only a link/number was sent.
func storedText(text, tx string) string {
	if amtRe.MatchString(text) {
		return text
	}
	return "https://transactioninfo.ethiotelecom.et/receipt/" + tx
}

// maxReceiptAge: a receipt older than this is not accepted automatically.
const maxReceiptAge = 24 * time.Hour

// autoDeposit credits a receipt that Verify.ET confirmed. The amount comes from Verify.ET, not from the user.
func autoDeposit(ctx context.Context, uid int64, text, tx string, declared int64, it *verifyItem) (string, bool) {
	amount := int64(math.Round(float64(it.Amount) * 100))
	if amount < minDeposit {
		return "ዝቅተኛው ገቢ " + birr(minDeposit) + " ብር ነው።", false
	}
	switch it.receiverCheck() {
	case recvWrong:
		return "ይህ ክፍያ ወደ ሌላ አካውንት ነው የተላከው። ክፍያው ወደ " + cfg.TelebirrNumber + " መሆን አለበት።", false
	case recvUnknown:
		return queueDeposit(ctx, uid, text, tx, declared) // cannot read the receiver: the admin decides
	}
	if ts, err := time.Parse(time.RFC3339, it.Timestamp); err == nil && time.Since(ts) > maxReceiptAge {
		return "ይህ ትራንዛክሽን ከ24 ሰዓት በላይ ሆኖታል። ድጋፍን ያነጋግሩ።", false
	}
	if it.History != nil && it.History.ConfirmedBefore {
		return queueDeposit(ctx, uid, text, tx, declared) // this receipt was already checked by someone else: the admin decides
	}
	d := Deposit{UID: uid, Amount: amount, TxnID: tx, SMS: storedText(text, tx), ReceiverOK: true, Status: "pending", CreatedAt: time.Now()}
	res, err := db.Collection("deposits").InsertOne(ctx, d)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return msgDupDeposit, false
		}
		log.Println("deposit:", err)
		return "ችግር ተፈጥሯል። እባክዎ እንደገና ይሞክሩ።", false
	}
	id, _ := res.InsertedID.(primitive.ObjectID)
	if err := approveDeposit(ctx, id); err != nil {
		log.Println("auto approve:", err)
		return "✅ " + birr(amount) + " ብር ተቀብለናል (ref " + tx + ")። አድሚን በቅርቡ ያረጋግጣል።", true
	}
	return "✅ " + birr(amount) + " ብር ተረጋግጦ ወደ የጨዋታ ዋሌትዎ ገብቷል (ref " + tx + ")።", true
}

// queueDeposit saves the deposit for the admin to approve by hand.
func queueDeposit(ctx context.Context, uid int64, text, tx string, declared int64) (string, bool) {
	amount := declared
	if am := amtRe.FindStringSubmatch(text); am != nil {
		if v, err := strconv.ParseFloat(strings.ReplaceAll(am[1], ",", ""), 64); err == nil {
			amount = int64(math.Round(v * 100))
		}
	}
	if amount < minDeposit {
		return "ዝቅተኛው ገቢ " + birr(minDeposit) + " ብር ነው። መጠኑ አልታወቀም፤ ከ«ገንዘብ አስገባ» እንደገና ይጀምሩ።", false
	}
	tail := cfg.TelebirrNumber[max(0, len(cfg.TelebirrNumber)-4):]
	name := strings.ToLower(cfg.TelebirrName)
	name = name[:min(6, len(name))]
	okRecv := (tail != "" && strings.Contains(text, tail)) || (name != "" && strings.Contains(strings.ToLower(text), name))
	d := Deposit{UID: uid, Amount: amount, TxnID: tx, SMS: storedText(text, tx), ReceiverOK: okRecv, Status: "pending", CreatedAt: time.Now()}
	if _, err := db.Collection("deposits").InsertOne(ctx, d); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return msgDupDeposit, false
		}
		log.Println("deposit:", err)
		return "ችግር ተፈጥሯል። እባክዎ እንደገና ይሞክሩ።", false
	}
	return "✅ " + birr(amount) + " ብር ተቀብለናል (ref " + tx + ")። አድሚን በቅርቡ ያረጋግጣል።", true
}

func payReferrer(ctx context.Context, uid, deposit int64) {
	if deposit < refMinDeposit {
		return // too small to count; a later first-size deposit still can
	}
	var u User
	err := db.Collection("users").FindOneAndUpdate(ctx,
		bson.M{"_id": uid, "refPaid": false, "referredBy": bson.M{"$gt": 0}},
		bson.M{"$set": bson.M{"refPaid": true}}).Decode(&u)
	if err != nil {
		return
	}
	if referralBonus > 0 {
		if err := credit(ctx, u.ReferredBy, "bonus", referralBonus); err != nil {
			log.Println("referral:", err)
		} else {
			addTx(ctx, u.ReferredBy, "referral", referralBonus, strconv.FormatInt(uid, 10))
			go notify(u.ReferredBy, "🎁 የጋበዙት ጓደኛ የመጀመሪያ ገቢ አድርጓል። +"+birr(referralBonus)+" ብር ተጨምሯል!")
		}
	}
	// goal: every inviteGoal friends -> inviteReward. $inc returns the new count atomically, so a goal can't pay twice.
	var ref User
	if err := db.Collection("users").FindOneAndUpdate(ctx, bson.M{"_id": u.ReferredBy},
		bson.M{"$inc": bson.M{"refQualified": 1}},
		options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&ref); err != nil {
		log.Println("referral count:", err)
		return
	}
	if ref.RefQualified%inviteGoal == 0 {
		if err := credit(ctx, u.ReferredBy, "bonus", inviteReward); err != nil {
			log.Println("invite goal:", err)
			return
		}
		addTx(ctx, u.ReferredBy, "referral", inviteReward, "goal"+strconv.FormatInt(ref.RefQualified, 10))
		go notify(u.ReferredBy, "🎉 "+strconv.FormatInt(inviteGoal, 10)+" ጓደኞች ጋብዘዋል! +"+birr(inviteReward)+" ብር ወደ የጨዋታ ዋሌትዎ ተጨምሯል።")
	}
}

func approveDeposit(ctx context.Context, id primitive.ObjectID) error {
	deposits := db.Collection("deposits")
	var d Deposit
	err := deposits.FindOneAndUpdate(ctx, bson.M{"_id": id, "status": "pending"},
		bson.M{"$set": bson.M{"status": "approved", "decidedAt": time.Now()}}).Decode(&d)
	if err != nil {
		return err
	}
	// deposits go to the play wallet (bets only, never withdrawable); only prizes reach the main wallet
	if err := credit(ctx, d.UID, "bonus", d.Amount); err != nil {
		deposits.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"status": "pending"}})
		return err
	}
	addTx(ctx, d.UID, "deposit", d.Amount, d.TxnID)
	cbInc(ctx, d.UID, "deposited", d.Amount)
	go notify(d.UID, "✅ የ"+birr(d.Amount)+" ብር ገቢዎ ጸድቋል።")
	payReferrer(ctx, d.UID, d.Amount)
	return nil
}

func rejectDeposit(ctx context.Context, id primitive.ObjectID) error {
	var d Deposit
	err := db.Collection("deposits").FindOneAndUpdate(ctx, bson.M{"_id": id, "status": "pending"},
		bson.M{"$set": bson.M{"status": "rejected", "decidedAt": time.Now()}}).Decode(&d)
	if err != nil {
		return err
	}
	go notify(d.UID, "❌ የ"+birr(d.Amount)+" ብር ገቢዎ (ref "+d.TxnID+") ሊረጋገጥ አልቻለም። ስህተት ከሆነ ድጋፍን ያነጋግሩ።")
	return nil
}

func meHandler(c *gin.Context) {
	ctx, uid := c.Request.Context(), c.GetInt64("uid")
	var u User
	if err := db.Collection("users").FindOne(ctx, bson.M{"_id": uid}).Decode(&u); err != nil {
		c.JSON(500, gin.H{"error": "server error"})
		return
	}
	refs, _ := db.Collection("users").CountDocuments(ctx, bson.M{"referredBy": uid})
	c.JSON(200, gin.H{"id": u.ID, "name": u.Name, "balance": u.Balance, "bonus": u.Bonus,
		"referrals": refs, "refQualified": u.RefQualified, "inviteGoal": inviteGoal, "inviteReward": inviteReward, "bot": cfg.BotUsername, "minWithdraw": minWithdraw, "phone": u.Phone,
		"gamesWon": u.GamesWon, "earned": u.Earned, "telebirr": cfg.TelebirrNumber, "telebirrName": cfg.TelebirrName,
		"cashback": cashbackInfo(ctx, uid)})
}

func txsHandler(c *gin.Context) {
	ctx := c.Request.Context()
	cur, err := db.Collection("txs").Find(ctx, bson.M{"uid": c.GetInt64("uid")},
		options.Find().SetSort(bson.D{{Key: "at", Value: -1}}).SetLimit(30))
	if err != nil {
		c.JSON(500, gin.H{"error": "server error"})
		return
	}
	out := []Tx{}
	if err := cur.All(ctx, &out); err != nil {
		c.JSON(500, gin.H{"error": "server error"})
		return
	}
	c.JSON(200, out)
}

// withdrawTo holds the money and queues the request for an admin: "ok", "funds" or "err".
func withdrawTo(ctx context.Context, uid int64, phone string, amount int64) string {
	if n, err := db.Collection("withdrawals").CountDocuments(ctx, bson.M{"uid": uid, "status": "pending"}); err != nil || n >= 3 {
		return "limit" // at most 3 open requests per person
	}
	res, err := db.Collection("users").UpdateOne(ctx, bson.M{"_id": uid, "balance": bson.M{"$gte": amount}},
		bson.M{"$inc": bson.M{"balance": -amount}})
	if err != nil {
		return "err"
	}
	if res.ModifiedCount == 0 {
		return "funds"
	}
	w := Withdrawal{UID: uid, Amount: amount, Phone: phone, Status: "pending", CreatedAt: time.Now()}
	if _, err := db.Collection("withdrawals").InsertOne(ctx, w); err != nil {
		credit(ctx, uid, "balance", amount)
		return "err"
	}
	addTx(ctx, uid, "withdraw", -amount, phone)
	return "ok"
}

func withdrawHandler(c *gin.Context) {
	var in struct {
		Amount int64  `json:"amount"`
		Phone  string `json:"phone"`
	}
	uid := c.GetInt64("uid")
	if !rl.allow("wd:"+strconv.FormatInt(uid, 10), 5, time.Hour) {
		c.JSON(429, gin.H{"error": "Too many requests. Try again later."})
		return
	}
	if u, err := loadUser(c.Request.Context(), uid); err != nil || (u.Phone == "" && !cfg.Dev) {
		c.JSON(403, gin.H{"error": "Please register in the bot first."})
		return
	}
	phone := ""
	if err := c.ShouldBindJSON(&in); err == nil {
		phone = normPhone(in.Phone)
	}
	if phone == "" || in.Amount < 1 || in.Amount > 1000000 || in.Amount*100 < minWithdraw {
		c.JSON(400, gin.H{"error": "Enter a valid Telebirr phone number and an amount of at least " + birr(minWithdraw) + " birr."})
		return
	}
	switch withdrawTo(c.Request.Context(), uid, phone, in.Amount*100) {
	case "ok":
		c.JSON(200, gin.H{"ok": true})
	case "limit":
		c.JSON(429, gin.H{"error": "You already have open withdrawal requests. Wait until they are paid."})
	case "funds":
		c.JSON(402, gin.H{"error": "Not enough withdrawable balance. Bonus money can only be used to play."})
	default:
		c.JSON(500, gin.H{"error": "server error"})
	}
}

func statsHandler(c *gin.Context) {
	ctx := c.Request.Context()
	users, _ := db.Collection("users").EstimatedDocumentCount(ctx)
	pd, _ := db.Collection("deposits").CountDocuments(ctx, bson.M{"status": "pending"})
	pw, _ := db.Collection("withdrawals").CountDocuments(ctx, bson.M{"status": "pending"})
	c.JSON(200, gin.H{"users": users, "pendingDeposits": pd, "pendingWithdrawals": pw, "online": onlineTotal()})
}

func listHandler(coll string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		cur, err := db.Collection(coll).Find(ctx, bson.M{"status": c.DefaultQuery("status", "pending")},
			options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}).SetLimit(100))
		if err != nil {
			c.JSON(500, gin.H{"error": "server error"})
			return
		}
		out := []bson.M{}
		if err := cur.All(ctx, &out); err != nil {
			c.JSON(500, gin.H{"error": "server error"})
			return
		}
		c.JSON(200, out)
	}
}

func depositAction(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"error": "bad id"})
		return
	}
	ctx := c.Request.Context()
	switch c.Param("action") {
	case "approve":
		err = approveDeposit(ctx, id)
	case "reject":
		err = rejectDeposit(ctx, id)
	default:
		c.JSON(400, gin.H{"error": "bad action"})
		return
	}
	if err != nil {
		c.JSON(409, gin.H{"error": "Already handled or not found."})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func withdrawalAction(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	status := map[string]string{"paid": "paid", "reject": "rejected"}[c.Param("action")]
	if err != nil || status == "" {
		c.JSON(400, gin.H{"error": "bad request"})
		return
	}
	ctx := c.Request.Context()
	var w Withdrawal
	err = db.Collection("withdrawals").FindOneAndUpdate(ctx, bson.M{"_id": id, "status": "pending"},
		bson.M{"$set": bson.M{"status": status, "decidedAt": time.Now()}}).Decode(&w)
	if err != nil {
		c.JSON(409, gin.H{"error": "Already handled or not found."})
		return
	}
	if status == "paid" {
		go notify(w.UID, "💸 የ"+birr(w.Amount)+" ብር ጥያቄዎ ወደ "+w.Phone+" በቴሌብር ተልኳል።")
		c.JSON(200, gin.H{"ok": true})
		return
	}
	if err := credit(ctx, w.UID, "balance", w.Amount); err != nil {
		c.JSON(500, gin.H{"error": "refund failed, check logs"})
		return
	}
	addTx(ctx, w.UID, "refund", w.Amount, id.Hex())
	go notify(w.UID, "↩️ የ"+birr(w.Amount)+" ብር የማውጣት ጥያቄዎ ተቀባይነት አላገኘም። ወደ ዋሌትዎ ተመልሷል።")
	c.JSON(200, gin.H{"ok": true})
}

func normPhone(raw string) string {
	d := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, raw)
	d = strings.TrimPrefix(d, "251")
	if len(d) == 9 {
		d = "0" + d
	}
	if !phoneRe.MatchString(d) {
		return ""
	}
	return d
}

// maskPhone turns 0985241404 into 09****1404.
func maskPhone(p string) string {
	if len(p) < 7 {
		return p
	}
	return p[:2] + strings.Repeat("*", len(p)-6) + p[len(p)-4:]
}

// registerPhone links a phone to the account (one phone per account) and gives the welcome bonus.
func registerPhone(ctx context.Context, uid int64, raw string) string {
	phone := normPhone(raw)
	if phone == "" {
		return "bad"
	}
	res, err := db.Collection("users").UpdateOne(ctx,
		bson.M{"_id": uid, "phone": bson.M{"$exists": false}},
		bson.M{"$set": bson.M{"phone": phone}, "$inc": bson.M{"bonus": signupBonus}})
	if mongo.IsDuplicateKeyError(err) {
		return "dup"
	}
	if err != nil {
		log.Println("register:", err)
		return "err"
	}
	if res.ModifiedCount == 0 {
		return "already"
	}
	addTx(ctx, uid, "signup_bonus", signupBonus, "")
	return "ok"
}

// refundSpot returns a cartela's price to the wallet it was paid from.
func refundSpot(ctx context.Context, s Spot, n int, fee int64) {
	field := "balance"
	if s.Bonus {
		field = "bonus"
	}
	if err := credit(ctx, s.UID, field, fee); err != nil {
		log.Println("REFUND FAILED", s.UID, n, err)
		return
	}
	addTx(ctx, s.UID, "refund", fee, strconv.Itoa(n))
	cbInc(ctx, s.UID, "wagered", -fee) // a given-back stake was not played
}

func payWinner(ctx context.Context, uid, amt, round int64) error {
	_, err := db.Collection("users").UpdateOne(ctx, bson.M{"_id": uid},
		bson.M{"$inc": bson.M{"balance": amt, "earned": amt, "gamesWon": 1}})
	if err == nil {
		addTx(ctx, uid, "win", amt, strconv.FormatInt(round, 10))
	}
	return err
}