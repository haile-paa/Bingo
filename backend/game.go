// backend/game.go
package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"log"
	"math/big"
	mrand "math/rand"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	pickTime    = 30 * time.Second // time to choose cartelas
	callEvery   = 3 * time.Second  // one number every 3 s, until somebody gets bingo (max 75 numbers)
	resultPause = 8 * time.Second  // how long the winner screen stays before the next round
)

// cartelas[n] is the fixed card of cartela number n: the same card every round.
var cartelas [totalSpots + 1][5][5]int

func init() {
	r := mrand.New(mrand.NewSource(20261030))
	for n := 1; n <= totalSpots; n++ {
		for c := 0; c < 5; c++ {
			pool := r.Perm(15)
			for row := 0; row < 5; row++ {
				cartelas[n][row][c] = c*15 + pool[row] + 1
			}
		}
		cartelas[n][2][2] = 0
	}
}

type Spot struct {
	UID     int64
	Bonus   bool   // paid from the play-only bonus wallet
	Pending bool   // reserved, payment still in progress (not part of the round yet)
	Tok     string // unique id of this purchase
}

type WinnerOut struct {
	Cartela int       `json:"cartela"`
	UID     int64     `json:"uid"`
	Name    string    `json:"name"`
	Phone   string    `json:"phone"`
	Share   int64     `json:"share"`
	Card    [5][5]int `json:"card"`
}

type Result struct {
	Winners []WinnerOut `json:"winners"` // empty = no winner
	Prize   int64       `json:"prize"`
}

type Client struct {
	registered bool // phone number already checked on this connection
	game *Game
	uid  int64
	name string
	conn *websocket.Conn
	send chan []byte
}

type Game struct {
	mu       sync.Mutex
	fee      int64  // stake of this room in santim: 500 = 5 birr, 1000 = 10 birr
	phase    string // pick | run | result
	round    int64
	gid      string // short public id of the current game
	dirty    []gin.H // spot changes waiting to be sent to everybody (sent 4 times a second in one message)
	deadline time.Time
	spots    map[int]Spot
	called   []int
	marked   [76]bool
	result   *Result
	clients  map[*Client]struct{}
}

// games holds one room per stake (in birr). Each room runs its own rounds.
var games = map[int]*Game{}

var stakes = []int{5, 10}

func startGames() {
	for _, st := range stakes {
		g := newGame(int64(st) * 100)
		games[st] = g
		go g.run()
		go g.flushLoop()
	}
}

func onlineTotal() int {
	n := 0
	for _, g := range games {
		n += g.online()
	}
	return n
}

// roomsHandler is public: the first page shows the live numbers of every room.
func roomsHandler(c *gin.Context) {
	out := []gin.H{}
	for _, st := range stakes {
		g := games[st]
		g.mu.Lock()
		out = append(out, gin.H{"stake": st, "fee": g.fee, "phase": g.phase, "round": g.round,
			"players": len(g.spots), "prize": g.prizePool(len(g.spots)), "total": totalSpots})
		g.mu.Unlock()
	}
	c.JSON(200, out)
}

func newGame(fee int64) *Game {
	return &Game{fee: fee, phase: "pick", gid: newGameID(), deadline: time.Now().Add(pickTime), spots: map[int]Spot{},
		called: []int{}, clients: map[*Client]struct{}{}}
}

func randInt(n int) int {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		log.Fatal("crypto/rand failed: ", err)
	}
	return int(v.Int64())
}

func newGameID() string {
	const abc = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = abc[randInt(len(abc))]
	}
	return string(b)
}

func shuffle(a []int) {
	for i := len(a) - 1; i > 0; i-- {
		j := randInt(i + 1)
		a[i], a[j] = a[j], a[i]
	}
}

func hasBingo(card [5][5]int, m *[76]bool) bool {
	ok := func(r, c int) bool { return card[r][c] == 0 || m[card[r][c]] }
	for i := 0; i < 5; i++ {
		row, col := true, true
		for j := 0; j < 5; j++ {
			row = row && ok(i, j)
			col = col && ok(j, i)
		}
		if row || col {
			return true
		}
	}
	d1, d2 := true, true
	for i := 0; i < 5; i++ {
		d1 = d1 && ok(i, i)
		d2 = d2 && ok(i, 4-i)
	}
	return d1 || d2
}

// prizePool: players get payoutPercent (80%) of all stakes paid in; the company keeps the rest (20%).
func (g *Game) prizePool(n int) int64 { return int64(n) * g.fee * payoutPercent / 100 }

// --- messaging (callers hold g.mu) ---

func (cl *Client) push(b []byte) {
	select {
	case cl.send <- b:
	default: // slow client: drop; the next "call" message resyncs it
	}
}

func (cl *Client) pushJSON(v any) {
	if b, err := json.Marshal(v); err == nil {
		cl.push(b)
	}
}

func (cl *Client) fail(code string) { cl.pushJSON(gin.H{"type": "error", "code": code}) }

func (g *Game) pushAll(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	for cl := range g.clients {
		cl.push(b)
	}
}

func (g *Game) mineOf(uid int64) ([]int, gin.H) {
	mine, cards := []int{}, gin.H{}
	for n, s := range g.spots {
		if s.UID == uid && !s.Pending {
			mine = append(mine, n)
			cards[strconv.Itoa(n)] = cartelas[n]
		}
	}
	sort.Ints(mine)
	return mine, cards
}

// nSpots counts the cartelas that are really in the round (not the ones still being paid for).
func (g *Game) nSpots() int {
	n := 0
	for _, s := range g.spots {
		if !s.Pending {
			n++
		}
	}
	return n
}

func (g *Game) takenList() []int {
	taken := make([]int, 0, len(g.spots))
	for n, s := range g.spots {
		if !s.Pending {
			taken = append(taken, n)
		}
	}
	return taken
}

func (g *Game) stateFor(uid int64) gin.H {
	taken := g.takenList()
	mine, cards := g.mineOf(uid)
	return gin.H{"type": "state", "phase": g.phase, "round": g.round, "gid": g.gid, "deadline": g.deadline.UnixMilli(),
		"taken": taken, "mine": mine, "cards": cards, "prize": g.prizePool(g.nSpots()), "fee": g.fee,
		"total": totalSpots, "called": g.called, "result": g.result,
		"cfg": gin.H{"pick": int(pickTime / time.Second), "call": int(callEvery / time.Second),
			"max": maxPerUser, "min": minSpots, "pct": payoutPercent}}
}

func (g *Game) pushState() {
	for cl := range g.clients {
		cl.pushJSON(g.stateFor(cl.uid))
	}
}

func (g *Game) add(cl *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.clients[cl] = struct{}{}
	cl.pushJSON(g.stateFor(cl.uid))
}

func (g *Game) remove(cl *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.clients[cl]; ok {
		delete(g.clients, cl)
		close(cl.send)
	}
}

func (g *Game) online() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.clients)
}

// --- picking ---

func (g *Game) toggle(cl *Client, n int) {
	if n < 1 || n > totalSpots {
		return
	}
	if maintOn() {
		cl.fail("maintenance")
		return
	}
	g.mu.Lock()
	if g.phase != "pick" {
		g.mu.Unlock()
		cl.fail("closed")
		return
	}
	s, held := g.spots[n]
	if held && s.UID != cl.uid {
		g.mu.Unlock()
		cl.fail("taken")
		return
	}
	if held && s.Pending { // this tap is still being processed
		g.mu.Unlock()
		return
	}
	gid, fee := g.gid, g.fee
	if held { // tap again to release: the number is free at once, the money comes back in the background
		delete(g.spots, n)
		g.announce(cl, n, false)
		g.mu.Unlock()
		bg(func() { refundByTok(gid, n, s, fee) })
		return
	}
	owned := 0
	for _, o := range g.spots {
		if o.UID == cl.uid {
			owned++
		}
	}
	if owned >= maxPerUser {
		g.mu.Unlock()
		cl.fail("limit")
		return
	}
	tok := newTok()
	g.spots[n] = Spot{UID: cl.uid, Pending: true, Tok: tok} // reserve the number, then pay WITHOUT holding the lock
	g.mu.Unlock()

	field, code := g.pay(cl, n, gid, tok)

	g.mu.Lock()
	defer g.mu.Unlock()
	cur, ok := g.spots[n]
	mine := ok && cur.Pending && cur.Tok == tok
	if code != "" {
		if mine {
			delete(g.spots, n)
		}
		cl.fail(code)
		return
	}
	if !mine || g.phase != "pick" || g.gid != gid { // the round moved on while we were paying: give the money back
		if mine {
			delete(g.spots, n)
		}
		paid := Spot{UID: cl.uid, Bonus: field == "bonus", Tok: tok}
		bg(func() { refundByTok(gid, n, paid, fee) })
		cl.fail("closed")
		return
	}
	g.spots[n] = Spot{UID: cl.uid, Bonus: field == "bonus", Tok: tok}
	g.announce(cl, n, true)
}

// announce tells the player about their own cartelas at once, and queues the change for everybody else.
func (g *Game) announce(cl *Client, n int, on bool) {
	mine, cards := g.mineOf(cl.uid)
	cl.pushJSON(gin.H{"type": "mine", "mine": mine, "cards": cards})
	g.dirty = append(g.dirty, gin.H{"n": n, "on": on})
}

// flushLoop sends the queued spot changes 4 times a second as ONE message, and a full list every 5 seconds
// so a phone that missed a message catches up.
func (g *Game) flushLoop() {
	tick := 0
	for {
		time.Sleep(250 * time.Millisecond)
		tick++
		g.mu.Lock()
		if len(g.dirty) > 0 {
			g.pushAll(gin.H{"type": "spots", "list": g.dirty, "prize": g.prizePool(g.nSpots())})
			g.dirty = nil
		}
		if tick%20 == 0 && g.phase == "pick" {
			g.pushAll(gin.H{"type": "sync", "taken": g.takenList(), "prize": g.prizePool(g.nSpots())})
		}
		g.mu.Unlock()
	}
}

func (g *Game) run() {
	for {
		g.pick()
		g.play()
		time.Sleep(resultPause)
	}
}

func (g *Game) pick() {
	g.mu.Lock()
	g.round++
	g.gid = newGameID()
	g.phase, g.deadline, g.result = "pick", time.Now().Add(pickTime), nil
	g.spots, g.called, g.dirty = map[int]Spot{}, []int{}, nil
	g.marked = [76]bool{}
	g.pushState()
	g.mu.Unlock()
	for {
		time.Sleep(time.Second)
		g.mu.Lock()
		due := time.Now().After(g.deadline)
		ready := due && g.nSpots() >= minSpots
		if due && !ready { // not enough cartelas yet: keep the picks and wait another round of time
			g.deadline = time.Now().Add(pickTime)
			g.pushState()
		}
		g.mu.Unlock()
		if ready {
			return
		}
	}
}

func (g *Game) winningSpots() []int {
	var out []int
	for n := range g.spots {
		if hasBingo(cartelas[n], &g.marked) {
			out = append(out, n)
		}
	}
	return out
}

// play calls one number every callEvery and stops the moment somebody has bingo.
// There is no time limit: after all 75 numbers every card is complete, so a winner is guaranteed.
func (g *Game) play() {
	g.mu.Lock()
	g.phase, g.deadline = "run", time.Now()
	for n, s := range g.spots { // a payment still in progress misses this round and is refunded when it finishes
		if s.Pending {
			delete(g.spots, n)
		}
	}
	prize := g.prizePool(len(g.spots))
	g.pushState()
	g.mu.Unlock()

	order := make([]int, 75)
	for i := range order {
		order[i] = i + 1
	}
	shuffle(order)
	for _, n := range order {
		time.Sleep(callEvery)
		g.mu.Lock()
		g.called = append(g.called, n)
		g.marked[n] = true
		wins := g.winningSpots()
		g.pushAll(gin.H{"type": "call", "n": n, "called": g.called})
		g.mu.Unlock()
		if len(wins) > 0 { // winner found: no more numbers are called
			g.finish(wins, prize)
			return
		}
	}
	g.mu.Lock()
	wins := g.winningSpots()
	g.mu.Unlock()
	g.finish(wins, prize)
}

func (g *Game) finish(wins []int, prize int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	g.mu.Lock()
	spots := make(map[int]Spot, len(g.spots))
	for n, s := range g.spots {
		spots[n] = s
	}
	round, gid := g.round, g.gid
	g.mu.Unlock()
	dropSpotDocs(ctx, gid) // before any money moves, so a crash can never refund stakes of a round that paid out

	res := &Result{Winners: []WinnerOut{}}
	if len(wins) == 0 { // nobody got bingo: give every stake back
		for n, s := range spots {
			refundSpot(ctx, s, n, g.fee)
		}
	} else {
		sort.Ints(wins)
		share := prize / int64(len(wins))
		res.Prize = prize
		paid := map[int64]int64{}
		won := map[int64]int64{} // stake money of the winning cartelas, removed from the weekly cashback
		for _, n := range wins {
			uid := spots[n].UID
			u, err := loadUser(ctx, uid)
			if err != nil {
				log.Println("winner lookup:", uid, err)
			}
			res.Winners = append(res.Winners, WinnerOut{Cartela: n, UID: uid, Name: u.Name,
				Phone: maskPhone(u.Phone), Share: share, Card: cartelas[n]})
			paid[uid] += share
			won[uid] += g.fee
		}
		for uid, amt := range paid {
			if err := payWinner(ctx, uid, amt, round); err != nil {
				log.Println("PAYOUT FAILED", uid, amt, err)
				continue
			}
			cbInc(ctx, uid, "wonStake", won[uid])
			go notify(uid, "🏆 አሸንፈዋል! "+birr(amt)+" ብር ወደ ዋሌትዎ ተጨምሯል።")
		}
	}
	g.mu.Lock()
	g.phase, g.result, g.deadline = "result", res, time.Now().Add(resultPause)
	g.pushState()
	g.mu.Unlock()
}

// --- websocket ---

var upgrader = websocket.Upgrader{CheckOrigin: originOK, ReadBufferSize: 1024, WriteBufferSize: 4096}

func wsHandler(c *gin.Context) {
	uid, name, ok := authValue(c.Query("a"))
	if !ok {
		c.Status(http.StatusUnauthorized)
		return
	}
	if _, err := ensureUser(c.Request.Context(), uid, name, 0); err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	if maintOn() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "maintenance"})
		return
	}
	stake, _ := strconv.Atoi(c.DefaultQuery("s", "10"))
	room := games[stake]
	if room == nil {
		c.Status(http.StatusBadRequest)
		return
	}
	if room.countUser(uid) >= 3 || onlineTotal() > 20000 { // too many open sockets for one user / for the server
		c.Status(http.StatusTooManyRequests)
		return
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	cl := &Client{game: room, uid: uid, name: name, conn: conn, send: make(chan []byte, 64)}
	room.add(cl)
	go cl.writeLoop()
	cl.readLoop()
}

func (cl *Client) readLoop() {
	defer func() {
		cl.game.remove(cl)
		cl.conn.Close()
	}()
	cl.conn.SetReadLimit(512)
	cl.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	drops := 0
	cl.conn.SetPongHandler(func(string) error {
		return cl.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	})
	for {
		var m struct {
			Type string `json:"type"`
			N    int    `json:"n"`
		}
		if err := cl.conn.ReadJSON(&m); err != nil {
			return
		}
		if m.Type == "toggle" {
			// at most 12 taps per 3 seconds; keep spamming and the connection is closed
			if !rl.allow("ws:"+strconv.FormatInt(cl.uid, 10), 12, 3*time.Second) {
				if drops++; drops > 30 {
					return
				}
				continue
			}
			cl.game.toggle(cl, m.N)
		}
	}
}

func (cl *Client) writeLoop() {
	t := time.NewTicker(25 * time.Second)
	defer t.Stop()
	defer cl.conn.Close()
	for {
		select {
		case b, ok := <-cl.send:
			if !ok {
				return
			}
			cl.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if cl.conn.WriteMessage(websocket.TextMessage, b) != nil {
				return
			}
		case <-t.C:
			if cl.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)) != nil {
				return
			}
		}
	}
}
