// backend/bots.go - clearly labeled house bots that help fill a room
//
// How it works (all controlled from /finance -> Bots):
//   - per stake (5 and 10 birr) the owner turns bots ON/OFF and sets how many cartelas the bots take
//   - at the start of each round, bots take random free numbers during the first 25 seconds of the 30 s pick time
//   - real players can pick any free number during the whole 30 s; bots never block real players, they only hold numbers
//   - EVERY bot is shown to players with a BOT badge (picker + winner screen). Nothing is hidden.
//   - bots are free for the house: no stake is taken and no wallet exists for them
//   - the draw is the same fair random draw for everybody. "Bots can win" is OFF by default: with it OFF the bots only
//     hold numbers; with it ON a bot can win like any cartela, and a bot's share is NOT paid out (it stays with the house).
//   - the prize pool is calculated from ALL held cartelas (bots + real players), live as cartelas are taken. A bot cartela adds to the prize but costs no player anything: if a real player wins, the house pays the bot part.
package main

import (
	"context"
	"log"
	"net/http"
	"sort"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const botFillWindow = 25 * time.Second // bots finish taking their numbers inside this window

type botRoom struct {
	On     bool `bson:"on" json:"on"`
	Count  int  `bson:"count" json:"count"`   // how many cartelas the bots hold
	CanWin bool `bson:"canWin" json:"canWin"` // default false
}

type botsDoc struct {
	ID    string             `bson:"_id"`
	Rooms map[string]botRoom `bson:"rooms"`
}

var botCfg atomic.Pointer[botsDoc]

func botSettings(stake int) botRoom {
	d := botCfg.Load()
	if d == nil {
		return botRoom{}
	}
	return d.Rooms[strconv.Itoa(stake)]
}

func loadBots(ctx context.Context) {
	var d botsDoc
	err := db.Collection("settings").FindOne(ctx, bson.M{"_id": "bots"}).Decode(&d)
	if err == mongo.ErrNoDocuments {
		botCfg.Store(&botsDoc{ID: "bots", Rooms: map[string]botRoom{}})
		return
	}
	if err != nil {
		log.Println("bots load:", err)
		return
	}
	if d.Rooms == nil {
		d.Rooms = map[string]botRoom{}
	}
	botCfg.Store(&d)
}

func startBots() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	loadBots(ctx)
	cancel()
	go func() {
		for {
			time.Sleep(10 * time.Second)
			c2, c2cancel := context.WithTimeout(context.Background(), 8*time.Second)
			loadBots(c2)
			c2cancel()
		}
	}()
}

// ---- names shown with the BOT badge ----

var botFirst = []string{"Abebe", "Kebede", "Tigist", "Almaz", "Dawit", "Selam", "Mulugeta", "Hana", "Yonas", "Meron",
	"Bereket", "Tsion", "Henok", "Rahel", "Samuel", "Eden", "Biniam", "Marta", "Fikru", "Lemlem",
	"Getachew", "Birtukan", "Tesfaye", "Senait", "Mekonnen", "Aster", "Habtamu", "Genet", "Solomon", "Betty"}
var botLast = []string{"Tadesse", "Bekele", "Alemu", "Haile", "Girma", "Assefa", "Worku", "Mengistu", "Desta", "Gebre",
	"Negash", "Lemma", "Teshome", "Abera", "Kassa", "Demissie", "Wolde", "Tilahun", "Mamo", "Shiferaw"}

// bots have negative ids: -1, -2, ... (real Telegram ids are always positive)
func botName(uid int64) string {
	i := int(-uid) - 1
	if i < 0 {
		i = 0
	}
	return botFirst[i%len(botFirst)] + " " + botLast[(i*7+3)%len(botLast)]
}

// ---- helpers (callers hold g.mu) ----

func (g *Game) botList() []int {
	out := []int{}
	for n, s := range g.spots {
		if s.Bot {
			out = append(out, n)
		}
	}
	sort.Ints(out)
	return out
}

// nReal counts the real players' cartelas in the round (no bots, no payments still in progress).
func (g *Game) nReal() int {
	n := 0
	for _, s := range g.spots {
		if !s.Pending && !s.Bot {
			n++
		}
	}
	return n
}

// fillBots adds `need` bots at random moments inside the pick time (never later than 5 s before the round starts).
// It is started by pick() whenever bots are ON and fewer bots than the setting are holding numbers, so it also works
// when the owner switches bots on in the middle of a round or while the room waits for more players.
func (g *Game) fillBots(gid string, stake, need int) {
	defer func() {
		g.mu.Lock()
		if g.gid == gid {
			g.botFilling = false
		}
		g.mu.Unlock()
	}()
	g.mu.Lock()
	win := time.Until(g.deadline) - 5*time.Second
	g.mu.Unlock()
	if win > botFillWindow {
		win = botFillWindow
	}
	if win < 2*time.Second {
		win = 2 * time.Second
	}
	delays := make([]time.Duration, need)
	for i := range delays {
		delays[i] = time.Duration(randInt(int(win/time.Millisecond))) * time.Millisecond
	}
	sort.Slice(delays, func(a, b int) bool { return delays[a] < delays[b] })
	start := time.Now()
	for _, d := range delays {
		if w := d - time.Since(start); w > 0 {
			time.Sleep(w)
		}
		if !botSettings(stake).On { // owner switched the bots off: stop adding
			return
		}
		g.mu.Lock()
		if g.gid != gid || g.phase != "pick" {
			g.mu.Unlock()
			return
		}
		free := make([]int, 0, totalSpots)
		for n := 1; n <= totalSpots; n++ {
			if _, held := g.spots[n]; !held {
				free = append(free, n)
			}
		}
		if len(free) <= 1 { // always keep room for real players
			g.mu.Unlock()
			return
		}
		n := free[randInt(len(free))]
		g.botSeq++
		g.spots[n] = Spot{UID: -int64(g.botSeq), Bot: true}
		g.dirty = append(g.dirty, gin.H{"n": n, "on": true, "bot": true})
		g.mu.Unlock()
	}
}

// ---- /finance API ----

func finBotsGet(c *gin.Context) {
	rooms := gin.H{}
	live := gin.H{}
	for _, st := range stakes {
		rooms[strconv.Itoa(st)] = botSettings(st)
		g := games[st]
		g.mu.Lock()
		live[strconv.Itoa(st)] = gin.H{"phase": g.phase, "bots": len(g.botList()), "real": g.nReal(), "total": totalSpots}
		g.mu.Unlock()
	}
	c.JSON(200, gin.H{"rooms": rooms, "live": live, "max": totalSpots - 1})
}

func finBotsSet(c *gin.Context) {
	var in struct {
		Rooms map[string]botRoom `json:"rooms"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Bad request"})
		return
	}
	out := map[string]botRoom{}
	for _, st := range stakes {
		k := strconv.Itoa(st)
		r, ok := in.Rooms[k]
		if !ok {
			r = botSettings(st)
		}
		if r.Count < 0 || r.Count > totalSpots-1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Bot count for " + k + " birr must be between 0 and " + strconv.Itoa(totalSpots-1)})
			return
		}
		out[k] = r
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	_, err := db.Collection("settings").UpdateOne(ctx, bson.M{"_id": "bots"},
		bson.M{"$set": bson.M{"rooms": out}}, options.Update().SetUpsert(true))
	if err != nil {
		log.Println("bots save:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not save"})
		return
	}
	botCfg.Store(&botsDoc{ID: "bots", Rooms: out})
	finBotsGet(c) // applies from the NEXT round
}

// botPhone is a random masked number in the same look as real winners (09*****1404) with the robot icon at the end.
// It is not a real person's number: only the last 4 digits are random.
func botPhone() string {
	return "09*****" + strconv.Itoa(1000+randInt(9000))
}