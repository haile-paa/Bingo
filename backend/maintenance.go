// backend/maintenance.go - switch the game off from /finance with a message, picture and countdown
//
// While maintenance is ON:
//   - players get a "we'll be back" screen (the web app asks /api/status every few seconds)
//   - the game API and the game connection answer 503, nobody can pick or play
//   - rounds that have not started yet are cancelled and their stakes refunded; rounds already running finish normally
//   - the Telegram bot keeps working (deposits, balance, withdrawals)
package main

import (
	"bytes"
	"context"
	"image"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type maintDoc struct {
	ID        string `bson:"_id"`
	On        bool   `bson:"on"`
	Title     string `bson:"title"`
	Message   string `bson:"message"`
	Until     int64  `bson:"until"` // unix ms, 0 = no countdown
	Image     []byte `bson:"image,omitempty"`
	ImageType string `bson:"imageType,omitempty"`
	V         int64  `bson:"v"` // changes with every save; used to refresh the cached picture
}

var maint atomic.Pointer[maintDoc]

func maintOn() bool {
	m := maint.Load()
	return m != nil && m.On
}

func loadMaint(ctx context.Context) {
	var d maintDoc
	err := db.Collection("settings").FindOne(ctx, bson.M{"_id": "maintenance"}).Decode(&d)
	if err == mongo.ErrNoDocuments {
		maint.Store(&maintDoc{})
		return
	}
	if err != nil {
		log.Println("maintenance load:", err) // keep the last known state
		return
	}
	maint.Store(&d)
}

// startMaintenance loads the setting and re-reads it every 10 seconds (so a second server instance follows too).
func startMaintenance() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	loadMaint(ctx)
	cancel()
	go func() {
		for {
			time.Sleep(10 * time.Second)
			c2, c2cancel := context.WithTimeout(context.Background(), 8*time.Second)
			loadMaint(c2)
			c2cancel()
		}
	}()
}

// maintenanceGate stops the game API while maintenance is on.
func maintenanceGate() gin.HandlerFunc {
	return func(c *gin.Context) {
		if maintOn() {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "maintenance"})
			return
		}
		c.Next()
	}
}

// statusHandler is public: the web app asks it whether the game is open.
func statusHandler(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	m := maint.Load()
	if m == nil || !m.On {
		c.JSON(200, gin.H{"on": false, "now": time.Now().UnixMilli()})
		return
	}
	c.JSON(200, gin.H{"on": true, "title": m.Title, "message": m.Message, "until": m.Until, "image": len(m.Image) > 0, "v": m.V, "now": time.Now().UnixMilli()})
}

func maintImage(c *gin.Context) {
	m := maint.Load()
	if m == nil || !m.On || len(m.Image) == 0 {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "public, max-age=3600")
	c.Data(200, m.ImageType, m.Image)
}

// drainPicks cancels rounds that have not started (everybody is refunded). Running rounds are left alone.
func drainPicks() {
	for _, g := range games {
		g.mu.Lock()
		if g.phase == "pick" {
			gid, fee := g.gid, g.fee
			for n, s := range g.spots {
				if s.Pending {
					continue
				}
				delete(g.spots, n)
				n, s := n, s
				bg(func() { refundByTok(gid, n, s, fee) })
				bg(func() {
					notify(s.UID, "🛠 ጥገና ተጀምሯል። ያልተጀመረው ጨዋታ ክፍያዎ ተመልሷል።")
				})
			}
			g.dirty = nil
			g.pushState()
		}
		g.mu.Unlock()
	}
}

// ---------- owner endpoints (in /finance) ----------

func maintView(m *maintDoc) gin.H {
	if m == nil {
		m = &maintDoc{}
	}
	return gin.H{"on": m.On, "title": m.Title, "message": m.Message, "until": m.Until, "hasImage": len(m.Image) > 0, "v": m.V}
}

// finMaintImage lets the owner see the saved picture even while maintenance is off.
func finMaintImage(c *gin.Context) {
	m := maint.Load()
	if m == nil || len(m.Image) == 0 {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "private, max-age=300")
	c.Data(200, m.ImageType, m.Image)
}

func finMaintGet(c *gin.Context) { c.JSON(200, maintView(maint.Load())) }

func finMaintSet(c *gin.Context) {
	if !rl.allow("maint", 30, time.Hour) {
		c.JSON(429, gin.H{"error": "Too many changes. Try again later."})
		return
	}
	const maxImg = 2 << 20
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxImg+(1<<20))
	if err := c.Request.ParseMultipartForm(maxImg + (1 << 20)); err != nil {
		c.JSON(400, gin.H{"error": "The upload is too large or unreadable (limit 2 MB)."})
		return
	}
	bad := func(msg string) { c.JSON(400, gin.H{"error": msg}) }

	prev := maint.Load()
	if prev == nil {
		prev = &maintDoc{}
	}
	d := maintDoc{ID: "maintenance", On: c.PostForm("on") == "1", Title: strings.TrimSpace(c.PostForm("title")),
		Message: strings.TrimSpace(c.PostForm("message")), Image: prev.Image, ImageType: prev.ImageType, V: time.Now().UnixMilli()}
	if utf8.RuneCountInString(d.Title) > 80 || utf8.RuneCountInString(d.Message) > 600 {
		bad("The title is limited to 80 characters and the message to 600.")
		return
	}
	if d.Title == "" {
		d.Title = "We'll be right back"
	}
	if v, err := strconv.ParseInt(c.PostForm("until"), 10, 64); err == nil && v > 0 {
		if v < time.Now().UnixMilli() || v > time.Now().Add(31*24*time.Hour).UnixMilli() {
			if d.On {
				bad("Pick a return time in the future (within 31 days), or clear it.")
				return
			}
			v = 0
		}
		d.Until = v
	}
	if c.PostForm("removeImage") == "1" {
		d.Image, d.ImageType = nil, ""
	}
	if fh, err := c.FormFile("image"); err == nil {
		if fh.Size > maxImg {
			bad("The picture is larger than 2 MB.")
			return
		}
		f, err := fh.Open()
		if err != nil {
			bad("Could not read the picture.")
			return
		}
		img, _ := io.ReadAll(io.LimitReader(f, maxImg+1))
		f.Close()
		ct := http.DetectContentType(img)
		cfgImg, _, derr := image.DecodeConfig(bytes.NewReader(img))
		if (ct != "image/jpeg" && ct != "image/png") || derr != nil || cfgImg.Width > 6000 || cfgImg.Height > 6000 {
			bad("Use a JPG or PNG picture (up to 6000 px).")
			return
		}
		d.Image, d.ImageType = img, ct
	}
	if _, err := db.Collection("settings").ReplaceOne(c.Request.Context(), bson.M{"_id": "maintenance"}, d, options.Replace().SetUpsert(true)); err != nil {
		c.JSON(500, gin.H{"error": "server error"})
		return
	}
	maint.Store(&d)
	if d.On && !prev.On {
		log.Println("MAINTENANCE turned ON by the owner")
		go drainPicks()
	} else if !d.On && prev.On {
		log.Println("MAINTENANCE turned OFF by the owner")
	}
	c.JSON(200, maintView(&d))
}
