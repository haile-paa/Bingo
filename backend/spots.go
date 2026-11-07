// backend/spots.go - paying for cartelas without freezing the game, and refunding after a crash
//
// Every paid cartela is also written to the "spots" collection. If the server stops in the middle of
// a round, those records are found again and the stakes are given back (see recoverStaleSpots).
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type SpotDoc struct {
	ID    primitive.ObjectID `bson:"_id,omitempty"`
	GID   string             `bson:"gid"`
	N     int                `bson:"n"`
	Tok   string             `bson:"tok"`
	UID   int64              `bson:"uid"`
	Field string             `bson:"field"`
	Fee   int64              `bson:"fee"`
	At    time.Time          `bson:"at"`
}

func newTok() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ---- background jobs (ledger entries, refunds) so players never wait for them ----

var bgq = make(chan func(), 8192)

func startBG() {
	for i := 0; i < 4; i++ {
		go func() {
			for f := range bgq {
				f()
			}
		}()
	}
}

func bg(f func()) {
	select {
	case bgq <- f:
	default:
		go f() // queue full: do it anyway
	}
}

// pay takes the stake from the player's wallet. It holds no game lock, so one slow database call
// can no longer stop everybody else from picking numbers. Returns the wallet field used, or an error code.
func (g *Game) pay(cl *Client, n int, gid, tok string) (string, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if isBlocked(cl.uid) {
		return "", "blocked"
	}
	if !cl.registered { // checked once per connection, not once per tap
		u, err := loadUser(ctx, cl.uid)
		if err != nil {
			return "", "server"
		}
		if u.Phone == "" && !cfg.Dev {
			return "", "register"
		}
		cl.registered = true
	}
	field, err := spend(ctx, cl.uid, g.fee)
	if err != nil {
		log.Println("claim:", err)
		return "", "server"
	}
	if field == "" {
		return "", "funds"
	}
	d := SpotDoc{GID: gid, N: n, Tok: tok, UID: cl.uid, Field: field, Fee: g.fee, At: time.Now()}
	if _, err := db.Collection("spots").InsertOne(ctx, d); err != nil {
		log.Println("spot record:", err) // the game goes on; only crash recovery is missing for this cartela
	}
	uid, fee := cl.uid, g.fee
	bg(func() { // history + weekly cashback counters
		c2, cancel2 := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel2()
		addTx(c2, uid, "entry", -fee, strconv.Itoa(n))
		cbInc(c2, uid, "wagered", fee)
	})
	return field, ""
}

// refundByTok gives one stake back and removes its record.
func refundByTok(gid string, n int, s Spot, fee int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_ = db.Collection("spots").FindOneAndDelete(ctx, bson.M{"gid": gid, "n": n, "tok": s.Tok}).Err()
	refundSpot(ctx, s, n, fee)
}

// dropSpotDocs removes the records of a finished round (before prizes are paid, so they can never be refunded twice).
func dropSpotDocs(ctx context.Context, gid string) {
	if _, err := db.Collection("spots").DeleteMany(ctx, bson.M{"gid": gid}); err != nil {
		log.Println("drop spots:", err)
	}
}

// startSpotJanitor looks every minute for cartelas of rounds that no longer exist (the server crashed or
// was restarted mid-round) and gives those stakes back. A round lasts a few minutes at most, so records older than
// 6 minutes that do not belong to a running round are leftovers.
func startSpotJanitor() {
	for {
		time.Sleep(time.Minute)
		recoverStaleSpots()
	}
}

func recoverStaleSpots() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	live := map[string]bool{}
	for _, g := range games {
		g.mu.Lock()
		live[g.gid] = true
		g.mu.Unlock()
	}
	cur, err := db.Collection("spots").Find(ctx, bson.M{"at": bson.M{"$lt": time.Now().Add(-6 * time.Minute)}})
	if err != nil {
		log.Println("spot janitor:", err)
		return
	}
	var docs []SpotDoc
	if err := cur.All(ctx, &docs); err != nil {
		return
	}
	for _, d := range docs {
		if live[d.GID] {
			continue // a round that is still waiting for enough players
		}
		if err := db.Collection("spots").FindOneAndDelete(ctx, bson.M{"_id": d.ID}).Err(); err != nil {
			continue // somebody else already took care of it
		}
		refundSpot(ctx, Spot{UID: d.UID, Bonus: d.Field == "bonus"}, d.N, d.Fee)
		go notify(d.UID, "ጨዋታው በመቋረጡ የ"+birr(d.Fee)+" ብር ክፍያዎ ተመልሷል።")
	}
}
