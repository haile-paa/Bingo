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
	UID   int64
	Bonus bool // paid from the play-only bonus wallet
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
	uid  int64
	name string
	conn *websocket.Conn
	send chan []byte
}

type Game struct {
	mu       sync.Mutex
	phase    string // pick | run | result
	round    int64
	gid      string // short public id of the current game
	deadline time.Time
	spots    map[int]Spot
	called   []int
	marked   [76]bool
	result   *Result
	clients  map[*Client]struct{}
}

func newGame() *Game {
	return &Game{phase: "pick", gid: newGameID(), deadline: time.Now().Add(pickTime), spots: map[int]Spot{},
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

// prizePool: 5000 birr when all 600 cartelas are sold, proportional below that (30 => 250 birr).
func prizePool(n int) int64 { return int64(n) * prizeFull / totalSpots }

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
		if s.UID == uid {
			mine = append(mine, n)
			cards[strconv.Itoa(n)] = cartelas[n]
		}
	}
	sort.Ints(mine)
	return mine, cards
}

func (g *Game) stateFor(uid int64) gin.H {
	taken := make([]int, 0, len(g.spots))
	for n := range g.spots {
		taken = append(taken, n)
	}
	mine, cards := g.mineOf(uid)
	return gin.H{"type": "state", "phase": g.phase, "round": g.round, "gid": g.gid, "deadline": g.deadline.UnixMilli(),
		"taken": taken, "mine": mine, "cards": cards, "prize": prizePool(len(g.spots)), "fee": entryFee,
		"total": totalSpots, "called": g.called, "result": g.result,
		"cfg": gin.H{"pick": int(pickTime / time.Second), "call": int(callEvery / time.Second),
			"max": maxPerUser, "min": minSpots, "full": prizeFull}}
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
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.phase != "pick" {
		cl.fail("closed")
		return
	}
	if n < 1 || n > totalSpots {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s, held := g.spots[n]
	if held && s.UID != cl.uid {
		cl.fail("taken")
		return
	}
	if held { // tap again to release and get the money back
		delete(g.spots, n)
		refundSpot(ctx, s, n)
		g.announce(cl, n, false)
		return
	}
	if code := g.claim(ctx, cl, n); code != "" {
		cl.fail(code)
		return
	}
	g.announce(cl, n, true)
}

func (g *Game) claim(ctx context.Context, cl *Client, n int) string {
	u, err := loadUser(ctx, cl.uid)
	if err != nil {
		return "server"
	}
	if u.Phone == "" && !cfg.Dev {
		return "register"
	}
	owned := 0
	for _, s := range g.spots {
		if s.UID == cl.uid {
			owned++
		}
	}
	if owned >= maxPerUser {
		return "limit"
	}
	field, err := spend(ctx, cl.uid, entryFee)
	if err != nil {
		log.Println("claim:", err)
		return "server"
	}
	if field == "" {
		return "funds"
	}
	addTx(ctx, cl.uid, "entry", -entryFee, strconv.Itoa(n))
	g.spots[n] = Spot{UID: cl.uid, Bonus: field == "bonus"}
	return ""
}

func (g *Game) announce(cl *Client, n int, on bool) {
	mine, cards := g.mineOf(cl.uid)
	cl.pushJSON(gin.H{"type": "mine", "mine": mine, "cards": cards})
	g.pushAll(gin.H{"type": "spot", "n": n, "on": on, "prize": prizePool(len(g.spots))})
}

// --- round loop ---

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
	g.spots, g.called = map[int]Spot{}, []int{}
	g.marked = [76]bool{}
	g.pushState()
	g.mu.Unlock()
	for {
		time.Sleep(time.Second)
		g.mu.Lock()
		due := time.Now().After(g.deadline)
		ready := due && len(g.spots) >= minSpots
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
	prize := prizePool(len(g.spots))
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
	round := g.round
	g.mu.Unlock()

	res := &Result{Winners: []WinnerOut{}}
	if len(wins) == 0 { // nobody got bingo: give every stake back
		for n, s := range spots {
			refundSpot(ctx, s, n)
		}
	} else {
		sort.Ints(wins)
		share := prize / int64(len(wins))
		res.Prize = prize
		paid := map[int64]int64{}
		for _, n := range wins {
			uid := spots[n].UID
			u, err := loadUser(ctx, uid)
			if err != nil {
				log.Println("winner lookup:", uid, err)
			}
			res.Winners = append(res.Winners, WinnerOut{Cartela: n, UID: uid, Name: u.Name,
				Phone: maskPhone(u.Phone), Share: share, Card: cartelas[n]})
			paid[uid] += share
		}
		for uid, amt := range paid {
			if err := payWinner(ctx, uid, amt, round); err != nil {
				log.Println("PAYOUT FAILED", uid, amt, err)
				continue
			}
			go notify(uid, "🏆 አሸንፈዋል! "+birr(amt)+" ብር ወደ ዋሌትዎ ተጨምሯል።")
		}
	}
	g.mu.Lock()
	g.phase, g.result, g.deadline = "result", res, time.Now().Add(resultPause)
	g.pushState()
	g.mu.Unlock()
}

// --- websocket ---

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

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
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	cl := &Client{uid: uid, name: name, conn: conn, send: make(chan []byte, 32)}
	game.add(cl)
	go cl.writeLoop()
	cl.readLoop()
}

func (cl *Client) readLoop() {
	defer func() {
		game.remove(cl)
		cl.conn.Close()
	}()
	cl.conn.SetReadLimit(512)
	cl.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
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
			game.toggle(cl, m.N)
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
