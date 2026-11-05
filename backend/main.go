// backend/main.go
package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Config struct {
	MongoURI, BotToken, BotUsername, WebhookSecret string
	AdminKey, MiniAppURL, TelebirrNumber, Port, TelebirrName, SupportUser, VerifyETKey string
	Dev                                            bool
}

var (
	cfg  Config
	db   *mongo.Database
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func hmacSHA256(key, msg []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(msg)
	return m.Sum(nil)
}

// validateInitData verifies the signed data Telegram gives a Mini App.
func validateInitData(raw string) (int64, string, bool) {
	vals, err := url.ParseQuery(raw)
	if err != nil {
		return 0, "", false
	}
	hash := vals.Get("hash")
	vals.Del("hash")
	keys := make([]string, 0, len(vals))
	for k := range vals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+vals.Get(k))
	}
	secret := hmacSHA256([]byte("WebAppData"), []byte(cfg.BotToken))
	sum := hex.EncodeToString(hmacSHA256(secret, []byte(strings.Join(parts, "\n"))))
	if !hmac.Equal([]byte(sum), []byte(hash)) {
		return 0, "", false
	}
	authDate, err := strconv.ParseInt(vals.Get("auth_date"), 10, 64)
	if err != nil || time.Since(time.Unix(authDate, 0)) > 24*time.Hour {
		return 0, "", false
	}
	var u struct {
		ID        int64  `json:"id"`
		FirstName string `json:"first_name"`
	}
	if err := json.Unmarshal([]byte(vals.Get("user")), &u); err != nil || u.ID == 0 {
		return 0, "", false
	}
	return u.ID, u.FirstName, true
}

func authValue(h string) (int64, string, bool) {
	if cfg.Dev && strings.HasPrefix(h, "dev ") {
		id, err := strconv.ParseInt(strings.TrimPrefix(h, "dev "), 10, 64)
		return id, "Player " + strconv.FormatInt(id, 10), err == nil && id > 0
	}
	if !strings.HasPrefix(h, "tma ") {
		return 0, "", false
	}
	return validateInitData(strings.TrimPrefix(h, "tma "))
}

func userAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, name, ok := authValue(c.GetHeader("Authorization"))
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		if !rl.allow("api:"+strconv.FormatInt(id, 10), 60, time.Minute) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "Too many requests. Please wait a moment."})
			return
		}
		if _, err := ensureUser(c.Request.Context(), id, name, 0); err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "server error"})
			return
		}
		c.Set("uid", id)
		c.Next()
	}
}

func adminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		got := []byte(c.GetHeader("X-Admin-Key"))
		if cfg.AdminKey == "" || subtle.ConstantTimeCompare(got, []byte(cfg.AdminKey)) != 1 {
			time.Sleep(400 * time.Millisecond) // slows down key guessing
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}

func main() {
	loadDotEnv(".env")
	cfg = Config{
		MongoURI: env("MONGO_URI", "mongodb://localhost:27017"), BotToken: os.Getenv("BOT_TOKEN"),
		BotUsername: os.Getenv("BOT_USERNAME"), WebhookSecret: os.Getenv("WEBHOOK_SECRET"),
		AdminKey: os.Getenv("ADMIN_KEY"), MiniAppURL: os.Getenv("MINIAPP_URL"),
		TelebirrNumber: env("TELEBIRR_NUMBER", "0975081362"), TelebirrName: env("TELEBIRR_NAME", "Haileyesusi"), SupportUser: os.Getenv("SUPPORT_USER"), VerifyETKey: os.Getenv("VERIFY_ET_API_KEY"), Port: env("PORT", "8080"), Dev: os.Getenv("DEV") == "1",
	}
	if cfg.BotToken == "" || cfg.WebhookSecret == "" {
		log.Fatal("BOT_TOKEN and WEBHOOK_SECRET are required")
	}
	onRender := os.Getenv("RENDER") != ""
	if cfg.Dev && onRender {
		log.Fatal("DEV=1 must never be set on Render: it lets anybody log in as any user")
	}
	if onRender && os.Getenv("MONGO_URI") == "" {
		log.Fatal("MONGO_URI is required in production")
	}
	if cfg.Dev {
		log.Println("WARNING: DEV mode is ON (local testing only)")
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	if len(cfg.AdminKey) < 24 {
		log.Println("WARNING: ADMIN_KEY is missing or shorter than 24 characters - use a long random key (e.g. 40 random characters)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongoURI).SetMaxPoolSize(50).SetServerSelectionTimeout(10*time.Second))
	if err != nil {
		log.Fatal(err)
	}
	db = client.Database("bingo")
	ensureIndexes(ctx)

	startMaintenance()
	startBots()
	startBG()
	startGames()
	go startSpotJanitor()
	go startCashback()
	go setupBotMenu()

	r := gin.New()
	r.SetTrustedProxies(nil)
	r.Use(gin.Recovery(), accessLog(), securityHeaders(), bodyLimit(64<<10), cors) // lets a separately hosted frontend (FRONTEND_ORIGIN) call this API
	r.GET("/health", func(c *gin.Context) { c.String(200, "ok") }) // ping this to keep the server awake
	r.GET("/ws", wsHandler)
	r.GET("/api/rooms", roomsHandler)
	r.GET("/api/status", statusHandler)       // public: is the game open?
	r.GET("/maintenance/image", maintImage) // public: the picture of the maintenance screen
	r.POST("/bot/:secret", botWebhook)

	api := r.Group("/api", maintenanceGate(), userAuth())
	api.GET("/me", meHandler)
	api.GET("/txs", txsHandler)
	api.POST("/withdraw", withdrawHandler)

	adm := r.Group("/admin", adminAuth())
	adm.GET("/stats", statsHandler)
	adm.GET("/deposits", listHandler("deposits"))
	adm.GET("/withdrawals", listHandler("withdrawals"))
	adm.POST("/deposits/:id/:action", depositAction)
	adm.POST("/withdrawals/:id/:action", withdrawalAction)

	registerFinance(r) // owner-only finance dashboard at /finance (needs OWNER_PHONE + OWNER_PASSWORD)

	r.Static("/assets", "../frontend/dist/assets")
	r.NoRoute(func(c *gin.Context) { c.File("../frontend/dist/index.html") })
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: r, ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout: 30 * time.Second, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 16 << 10}
	log.Fatal(srv.ListenAndServe())
}

// loadDotEnv reads KEY=VALUE lines from a .env file. Real environment variables take priority.
func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}
// cors allows the frontend origin set in FRONTEND_ORIGIN (e.g. https://bigbingo.pages.dev).
// When the frontend is served by this same server the variable is not needed.
func cors(c *gin.Context) {
	if o := os.Getenv("FRONTEND_ORIGIN"); o != "" {
		h := c.Writer.Header()
		h.Set("Access-Control-Allow-Origin", strings.TrimRight(o, "/"))
		h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Admin-Key")
		h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		h.Set("Vary", "Origin")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
	}
	c.Next()
}
