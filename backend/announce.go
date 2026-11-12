// backend/announce.go - send announcements (poster + description + button) to the bot's players from /finance
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	_ "image/jpeg" // lets image.DecodeConfig read JPEG and PNG headers
	_ "image/png"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Announcement struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Text       string             `bson:"text" json:"text"`
	HasImage   bool               `bson:"hasImage" json:"hasImage"`
	Button     string             `bson:"button" json:"button"` // none | play | link
	ButtonText string             `bson:"buttonText" json:"buttonText"`
	ButtonURL  string             `bson:"buttonUrl" json:"buttonUrl"`
	Audience   string             `bson:"audience" json:"audience"`
	Total      int                `bson:"total" json:"total"`
	Sent       int                `bson:"sent" json:"sent"`
	Failed     int                `bson:"failed" json:"failed"`
	Blocked    int                `bson:"blocked" json:"blocked"` // people who stopped the bot
	Status     string             `bson:"status" json:"status"`   // sending | done | cancelled | failed | interrupted
	CreatedAt  time.Time          `bson:"createdAt" json:"createdAt"`
	FinishedAt *time.Time         `bson:"finishedAt,omitempty" json:"finishedAt,omitempty"`
}

var (
	annMu      sync.Mutex
	annRunning bool
	annID      primitive.ObjectID
	annCancel  context.CancelFunc
	annClient  = &http.Client{Timeout: 60 * time.Second}
)

const (
	annMaxImage   = 5 << 20
	annImageCap   = 1000                  // caption limit with a photo (Telegram allows 1024)
	annTextCap    = 4000                  // plain message limit (Telegram allows 4096)
	annSendPeriod = 55 * time.Millisecond // about 18 messages a second, under Telegram's 30/s limit
)

type tgReply struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
	ErrorCode   int    `json:"error_code"`
	Parameters  struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
	Result struct {
		Photo []struct {
			FileID string `json:"file_id"`
		} `json:"photo"`
	} `json:"result"`
}

func tgRaw(method string, body io.Reader, contentType string) (tgReply, error) {
	var r tgReply
	resp, err := annClient.Post("https://api.telegram.org/bot"+cfg.BotToken+"/"+method, contentType, body)
	if err != nil {
		return r, errors.New(strings.ReplaceAll(err.Error(), cfg.BotToken, "<token>"))
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = json.Unmarshal(raw, &r)
	return r, nil
}

func annMarkup(a Announcement) any {
	switch a.Button {
	case "play":
		return gin.H{"inline_keyboard": [][]gin.H{{{"text": a.ButtonText, "web_app": gin.H{"url": cfg.MiniAppURL}}}}}
	case "link":
		return gin.H{"inline_keyboard": [][]gin.H{{{"text": a.ButtonText, "url": a.ButtonURL}}}}
	}
	return nil
}

// sendAnn sends one announcement to one person. The first photo is uploaded; Telegram gives back a
// file_id that every later message reuses, so the poster is uploaded only once.
func sendAnn(a Announcement, uid int64, fileID string, img []byte, fname string) (tgReply, error) {
	markup := annMarkup(a)
	if !a.HasImage {
		p := gin.H{"chat_id": uid, "text": a.Text, "disable_web_page_preview": true}
		if markup != nil {
			p["reply_markup"] = markup
		}
		b, _ := json.Marshal(p)
		return tgRaw("sendMessage", bytes.NewReader(b), "application/json")
	}
	if fileID != "" {
		p := gin.H{"chat_id": uid, "photo": fileID, "caption": a.Text}
		if markup != nil {
			p["reply_markup"] = markup
		}
		b, _ := json.Marshal(p)
		return tgRaw("sendPhoto", bytes.NewReader(b), "application/json")
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("chat_id", strconv.FormatInt(uid, 10))
	if a.Text != "" {
		_ = mw.WriteField("caption", a.Text)
	}
	if markup != nil {
		mb, _ := json.Marshal(markup)
		_ = mw.WriteField("reply_markup", string(mb))
	}
	fw, err := mw.CreateFormFile("photo", fname)
	if err != nil {
		return tgReply{}, err
	}
	_, _ = fw.Write(img)
	_ = mw.Close()
	return tgRaw("sendPhoto", &buf, mw.FormDataContentType())
}

// ---------- audiences ----------

func ownerID(ctx context.Context) int64 {
	var u User
	if err := db.Collection("users").FindOne(ctx, bson.M{"phone": finPhone}).Decode(&u); err != nil {
		return 0
	}
	return u.ID
}

func idSet(vals []any) map[int64]bool {
	m := make(map[int64]bool, len(vals))
	for _, v := range vals {
		m[num(v)] = true
	}
	return m
}

// annRecipients: people who finished registration in the bot (only they can be messaged).
func annRecipients(ctx context.Context, kind string) ([]int64, error) {
	if kind == "test" {
		if id := ownerID(ctx); id != 0 {
			return []int64{id}, nil
		}
		return nil, errors.New("Your owner phone number is not registered in the bot. Open the bot, press Register with that number, then try again.")
	}
	cur, err := db.Collection("users").Find(ctx, bson.M{"phone": bson.M{"$exists": true, "$ne": ""}, "blocked": bson.M{"$ne": true}},
		options.Find().SetProjection(bson.M{"_id": 1}))
	if err != nil {
		return nil, err
	}
	var us []User
	if err := cur.All(ctx, &us); err != nil {
		return nil, err
	}
	var keep map[int64]bool
	invert := false
	switch kind {
	case "all":
	case "depositors":
		vals, err := db.Collection("deposits").Distinct(ctx, "uid", bson.M{"status": "approved"})
		if err != nil {
			return nil, err
		}
		keep = idSet(vals)
	case "inactive":
		vals, err := db.Collection("txs").Distinct(ctx, "uid", bson.M{"at": bson.M{"$gte": time.Now().Add(-7 * 24 * time.Hour)}})
		if err != nil {
			return nil, err
		}
		keep, invert = idSet(vals), true
	default:
		return nil, errors.New("Unknown audience.")
	}
	out := make([]int64, 0, len(us))
	for _, u := range us {
		if keep == nil || keep[u.ID] != invert {
			out = append(out, u.ID)
		}
	}
	return out, nil
}

// ---------- handlers ----------

func annInfo(c *gin.Context) {
	annMu.Lock()
	running := annRunning
	annMu.Unlock()
	c.JSON(200, gin.H{"bot": cfg.BotUsername, "support": cfg.SupportUser, "miniApp": cfg.MiniAppURL,
		"ownerLinked": ownerID(c.Request.Context()) != 0, "sending": running})
}

func annAudience(c *gin.Context) {
	ids, err := annRecipients(c.Request.Context(), c.Query("kind"))
	if err != nil {
		c.JSON(200, gin.H{"count": 0, "note": err.Error()})
		return
	}
	c.JSON(200, gin.H{"count": len(ids)})
}

func annList(c *gin.Context) {
	ctx := c.Request.Context()
	out := []Announcement{}
	cur, err := db.Collection("announcements").Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(30))
	if err != nil || cur.All(ctx, &out) != nil {
		c.JSON(500, gin.H{"error": "server error"})
		return
	}
	c.JSON(200, gin.H{"rows": out})
}

func annSend(c *gin.Context) {
	if !rl.allow("annsend", 8, time.Hour) {
		c.JSON(429, gin.H{"error": "Too many announcements in the last hour."})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, annMaxImage+(1<<20))
	if err := c.Request.ParseMultipartForm(annMaxImage + (1 << 20)); err != nil {
		c.JSON(400, gin.H{"error": "The upload is too large or unreadable (limit 5 MB)."})
		return
	}
	bad := func(msg string) { c.JSON(400, gin.H{"error": msg}) }

	text := strings.TrimSpace(c.PostForm("text"))
	a := Announcement{Text: text, Button: c.DefaultPostForm("button", "none"), ButtonText: strings.TrimSpace(c.PostForm("buttonText")),
		ButtonURL: strings.TrimSpace(c.PostForm("buttonUrl")), Audience: c.PostForm("audience"), Status: "sending", CreatedAt: time.Now()}

	var img []byte
	fname := "poster.jpg"
	if fh, err := c.FormFile("image"); err == nil {
		if fh.Size > annMaxImage {
			bad("The picture is larger than 5 MB.")
			return
		}
		f, err := fh.Open()
		if err != nil {
			bad("Could not read the picture.")
			return
		}
		img, _ = io.ReadAll(io.LimitReader(f, annMaxImage+1))
		f.Close()
		ct := http.DetectContentType(img)
		cfgImg, _, derr := image.DecodeConfig(bytes.NewReader(img))
		if (ct != "image/jpeg" && ct != "image/png") || derr != nil {
			bad("Use a JPG or PNG picture.")
			return
		}
		if cfgImg.Width+cfgImg.Height > 10000 || cfgImg.Width > 20*cfgImg.Height || cfgImg.Height > 20*cfgImg.Width {
			bad("The picture size is not allowed by Telegram (too big or too narrow).")
			return
		}
		if ct == "image/png" {
			fname = "poster.png"
		}
		a.HasImage = true
	}
	limit := annTextCap
	if a.HasImage {
		limit = annImageCap
	}
	if n := utf8.RuneCountInString(text); n > limit || (n == 0 && !a.HasImage) {
		bad("Write a description (up to " + strconv.Itoa(limit) + " characters), or add a picture.")
		return
	}
	switch a.Button {
	case "none":
	case "play":
		if cfg.MiniAppURL == "" {
			bad("MINIAPP_URL is not set on the server.")
			return
		}
	case "link":
		if u, err := url.Parse(a.ButtonURL); err != nil || u.Scheme != "https" || u.Host == "" || len(a.ButtonURL) > 500 {
			bad("The button link must start with https://")
			return
		}
	default:
		bad("Unknown button type.")
		return
	}
	if a.Button != "none" && (a.ButtonText == "" || utf8.RuneCountInString(a.ButtonText) > 40) {
		bad("Write the button text (up to 40 characters).")
		return
	}

	ctx := c.Request.Context()
	ids, err := annRecipients(ctx, a.Audience)
	if err != nil {
		bad(err.Error())
		return
	}
	if len(ids) == 0 {
		bad("Nobody is in this audience yet.")
		return
	}
	annMu.Lock()
	if annRunning {
		annMu.Unlock()
		c.JSON(409, gin.H{"error": "Another announcement is still being sent. Wait until it finishes."})
		return
	}
	annRunning = true
	annMu.Unlock()

	a.Total = len(ids)
	res, err := db.Collection("announcements").InsertOne(ctx, a)
	if err != nil {
		annMu.Lock()
		annRunning = false
		annMu.Unlock()
		c.JSON(500, gin.H{"error": "server error"})
		return
	}
	a.ID, _ = res.InsertedID.(primitive.ObjectID)
	go runAnnouncement(a, ids, img, fname)
	c.JSON(200, gin.H{"ok": true, "id": a.ID, "total": a.Total})
}

func annCancelH(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"error": "bad id"})
		return
	}
	annMu.Lock()
	defer annMu.Unlock()
	if annRunning && annID == id && annCancel != nil {
		annCancel()
	}
	c.JSON(200, gin.H{"ok": true})
}

// runAnnouncement sends to everybody one by one and keeps the progress in the database.
func runAnnouncement(a Announcement, ids []int64, img []byte, fname string) {
	ctx, cancel := context.WithCancel(context.Background())
	annMu.Lock()
	annID, annCancel = a.ID, cancel
	annMu.Unlock()
	defer func() {
		cancel()
		annMu.Lock()
		annRunning, annCancel = false, nil
		annMu.Unlock()
	}()

	sent, failed, blocked, uploadFails := 0, 0, 0, 0
	status, fileID := "done", ""
	save := func(final bool) {
		set := bson.M{"sent": sent, "failed": failed, "blocked": blocked}
		if final {
			now := time.Now()
			set["status"], set["finishedAt"] = status, now
		}
		c2, c2cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer c2cancel()
		if _, err := db.Collection("announcements").UpdateOne(c2, bson.M{"_id": a.ID}, bson.M{"$set": set}); err != nil {
			log.Println("announcement progress:", err)
		}
	}
	for i, uid := range ids {
		if ctx.Err() != nil {
			status = "cancelled"
			break
		}
		var rep tgReply
		var err error
		for try := 0; try < 3; try++ {
			rep, err = sendAnn(a, uid, fileID, img, fname)
			if err == nil && !rep.OK && rep.ErrorCode == 429 { // Telegram asks us to slow down
				time.Sleep(time.Duration(rep.Parameters.RetryAfter+1) * time.Second)
				continue
			}
			break
		}
		switch {
		case err == nil && rep.OK:
			sent++
			if a.HasImage && fileID == "" && len(rep.Result.Photo) > 0 {
				fileID = rep.Result.Photo[len(rep.Result.Photo)-1].FileID
			}
		case err == nil && rep.ErrorCode == 403: // the person blocked the bot
			blocked++
		default:
			failed++
			log.Println("announcement:", rep.ErrorCode, rep.Description, err)
			if a.HasImage && fileID == "" && rep.ErrorCode == 400 {
				if uploadFails++; uploadFails >= 3 { // Telegram refuses the picture itself
					status = "failed"
					save(true)
					return
				}
			}
		}
		if (i+1)%25 == 0 {
			save(false)
		}
		time.Sleep(annSendPeriod)
	}
	save(true)
}

// annRecover marks announcements that were being sent when the server stopped.
func annRecover() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	now := time.Now()
	if _, err := db.Collection("announcements").UpdateMany(ctx, bson.M{"status": "sending"},
		bson.M{"$set": bson.M{"status": "interrupted", "finishedAt": now}}); err != nil {
		log.Println("announcement recover:", err)
	}
}
