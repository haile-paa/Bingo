// backend/cashback.go - weekly cashback on money the player LOST
//
// Per user and week (Monday 00:00 -> next Monday 00:00, Addis Ababa time) we count:
//   deposited - money deposited this week
//   wagered   - stakes paid this week (given back stakes are subtracted again)
//   wonStake  - stakes of the cartelas that WON a prize this week
// Money that was never played is not part of the cashback, and money that won a prize is removed from it:
//   lost     = max(0, min(deposited, wagered) - wonStake)
//   cashback = lost * cashbackPercent / 100, paid into the play wallet after the week ends.
package main

import (
	"context"
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const cashbackPercent int64 = 10

var eat = time.FixedZone("EAT", 3*60*60) // Addis Ababa

func weekStart(t time.Time) time.Time {
	t = t.In(eat)
	off := (int(t.Weekday()) + 6) % 7 // Monday = 0
	return time.Date(t.Year(), t.Month(), t.Day()-off, 0, 0, 0, 0, eat)
}

func weekKey(t time.Time) string { return weekStart(t).Format("2006-01-02") }

type CashbackWeek struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	UID       int64              `bson:"uid"`
	Week      string             `bson:"week"`
	Deposited int64              `bson:"deposited"`
	Wagered   int64              `bson:"wagered"`
	WonStake  int64              `bson:"wonStake"`
	Paid      bool               `bson:"paid"`
	Amount    int64              `bson:"amount"`
}

func (w CashbackWeek) lost() int64 {
	l := min(w.Deposited, w.Wagered) - w.WonStake
	if l < 0 {
		return 0
	}
	return l
}

func (w CashbackWeek) cashback() int64 { return w.lost() * cashbackPercent / 100 }

// cbInc adds amt to one counter of this week's record (created on first use).
func cbInc(ctx context.Context, uid int64, field string, amt int64) {
	if amt == 0 {
		return
	}
	_, err := db.Collection("cashback").UpdateOne(ctx,
		bson.M{"uid": uid, "week": weekKey(time.Now())},
		bson.M{"$inc": bson.M{field: amt}},
		options.Update().SetUpsert(true))
	if err != nil {
		log.Println("cashback:", field, err)
	}
}

// cashbackInfo is what /api/me sends to the wallet screen.
func cashbackInfo(ctx context.Context, uid int64) gin.H {
	var w CashbackWeek
	_ = db.Collection("cashback").FindOne(ctx, bson.M{"uid": uid, "week": weekKey(time.Now())}).Decode(&w) // zero values when nothing yet
	start := weekStart(time.Now())
	return gin.H{"percent": cashbackPercent, "deposited": w.Deposited, "lost": w.lost(), "amount": w.cashback(),
		"start": start.UnixMilli(), "end": start.AddDate(0, 0, 7).UnixMilli()}
}

// startCashback pays finished weeks (checked every 10 minutes, so payouts arrive right after Monday 00:00).
func startCashback() {
	for {
		payCashback()
		time.Sleep(10 * time.Minute)
	}
}

func payCashback() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cur, err := db.Collection("cashback").Find(ctx, bson.M{"week": bson.M{"$lt": weekKey(time.Now())}, "paid": bson.M{"$ne": true}})
	if err != nil {
		log.Println("cashback find:", err)
		return
	}
	var weeks []CashbackWeek
	if err := cur.All(ctx, &weeks); err != nil {
		log.Println("cashback read:", err)
		return
	}
	for _, w := range weeks {
		amt := w.cashback()
		// mark as paid first, so two servers can never pay the same week twice
		res, err := db.Collection("cashback").UpdateOne(ctx, bson.M{"_id": w.ID, "paid": bson.M{"$ne": true}},
			bson.M{"$set": bson.M{"paid": true, "amount": amt}})
		if err != nil || res.ModifiedCount != 1 || amt <= 0 {
			continue
		}
		if err := credit(ctx, w.UID, "bonus", amt); err != nil {
			log.Println("CASHBACK FAILED", w.UID, amt, err)
			db.Collection("cashback").UpdateOne(ctx, bson.M{"_id": w.ID}, bson.M{"$set": bson.M{"paid": false}})
			continue
		}
		addTx(ctx, w.UID, "cashback", amt, w.Week)
		go notify(w.UID, "🎁 ሳምንታዊ ካሽባክ! +"+birr(amt)+" ብር ወደ የጨዋታ ዋሌትዎ ተጨምሯል።")
	}
}
