// backend/blocklist.go - the owner can block a player from the whole game (set in /finance -> People).
//
// A blocked player:
//   - cannot open the game (the websocket sends {"type":"blocked"} and closes) or pick cartelas
//   - cannot use the API (balance, history, WITHDRAW) - every /api call answers 403 "blocked"
//   - gets no answers from the Telegram bot except a short "blocked" message
//   - is thrown out of any open game; cartelas held in a round that is still in the pick phase are released and
//     the stake is refunded. A round that is already running is NOT cancelled for the others.
//   - keeps the money in the wallet (nothing is taken away), it is just frozen until the owner unblocks.
//
// The phone number stays registered to the blocked account (it is unique), so the same phone cannot be used
// to open a second account.
package main

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const msgBlocked = "🚫 አካውንትዎ ታግዷል።\nYour account is blocked."

var blockedIDs = struct {
	sync.RWMutex
	m map[int64]bool
}{m: map[int64]bool{}}

// isBlocked is a memory lookup (no database call), so it is cheap enough to run on every request.
func isBlocked(uid int64) bool {
	blockedIDs.RLock()
	defer blockedIDs.RUnlock()
	return blockedIDs.m[uid]
}

func cacheBlocked(uid int64, on bool) {
	blockedIDs.Lock()
	defer blockedIDs.Unlock()
	if on {
		blockedIDs.m[uid] = true
	} else {
		delete(blockedIDs.m, uid)
	}
}

// loadBlocked fills the memory list from the database; runs once at startup.
func loadBlocked(ctx context.Context) {
	cur, err := db.Collection("users").Find(ctx, bson.M{"blocked": true}, options.Find().SetProjection(bson.M{"_id": 1}))
	if err != nil {
		log.Fatal("load blocked players: ", err) // better not to start than to start with everybody unblocked
	}
	var rows []struct {
		ID int64 `bson:"_id"`
	}
	if err := cur.All(ctx, &rows); err != nil {
		log.Fatal("load blocked players: ", err)
	}
	for _, r := range rows {
		cacheBlocked(r.ID, true)
	}
	log.Printf("blocked players loaded: %d", len(rows))
}

// setBlocked blocks or unblocks a player.
func setBlocked(ctx context.Context, uid int64, on bool, reason string) error {
	upd := bson.M{"$unset": bson.M{"blocked": "", "blockedAt": "", "blockReason": ""}}
	if on {
		upd = bson.M{"$set": bson.M{"blocked": true, "blockedAt": time.Now(), "blockReason": reason}}
	}
	res, err := db.Collection("users").UpdateOne(ctx, bson.M{"_id": uid}, upd)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}
	cacheBlocked(uid, on)
	action := "unblock"
	if on {
		action = "block"
	}
	if _, err := db.Collection("adminlog").InsertOne(ctx, bson.M{"at": time.Now(), "action": action, "uid": uid, "reason": reason}); err != nil {
		log.Println("adminlog:", err)
	}
	log.Printf("PLAYER %s uid=%d", strings.ToUpper(action), uid)
	if on {
		kickPlayer(uid)
	}
	peopleMu.Lock() // the People list must show the new state at once
	peopleCache = nil
	peopleMu.Unlock()
	return nil
}

// kickPlayer closes the player's open games and releases the cartelas they hold in rounds that are still picking.
func kickPlayer(uid int64) {
	for _, g := range games {
		var kicked []*Client
		g.mu.Lock()
		for cl := range g.clients {
			if cl.uid == uid {
				kicked = append(kicked, cl)
			}
		}
		if g.phase == "pick" {
			for n, s := range g.spots {
				if s.UID != uid || s.Pending { // a payment still in progress is refunded by toggle() when it finishes
					continue
				}
				delete(g.spots, n)
				g.dirty = append(g.dirty, gin.H{"n": n, "on": false})
				gid, fee, n, s := g.gid, g.fee, n, s
				bg(func() { refundByTok(gid, n, s, fee) })
			}
		}
		g.mu.Unlock()
		for _, cl := range kicked {
			cl.pushJSON(gin.H{"type": "blocked"})
			cl := cl
			go func() {
				time.Sleep(400 * time.Millisecond) // let the "blocked" message reach the phone first
				cl.conn.Close()
			}()
		}
	}
}

// finBlock: POST /finance/api/people/:id/block   body {"blocked": true, "reason": "..."}
func finBlock(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var in struct {
		Blocked bool   `json:"blocked"`
		Reason  string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad request"})
		return
	}
	reason := strings.TrimSpace(in.Reason)
	if utf8.RuneCountInString(reason) > 200 {
		reason = string([]rune(reason)[:200])
	}
	if err := setBlocked(c.Request.Context(), id, in.Blocked, reason); err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{"error": "player not found"})
			return
		}
		log.Println("block:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "blocked": in.Blocked})
}
