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
	AdminKey, MiniAppURL, TelebirrNumber, Port, TelebirrName, SupportUser string
	Dev                                            bool
}

var (
	cfg  Config
	db   *mongo.Database
	game *Game
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
		TelebirrNumber: env("TELEBIRR_NUMBER", "0975081362"), TelebirrName: env("TELEBIRR_NAME", "Haileyesusi"), SupportUser: os.Getenv("SUPPORT_USER"), Port: env("PORT", "8080"), Dev: os.Getenv("DEV") == "1",
	}
	if cfg.BotToken == "" || cfg.WebhookSecret == "" {
		log.Fatal("BOT_TOKEN and WEBHOOK_SECRET are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongoURI))
	if err != nil {
		log.Fatal(err)
	}
	db = client.Database("bingo")
	ensureIndexes(ctx)

	game = newGame()
	go game.run()
	go setupBotMenu()

	r := gin.Default()
	r.GET("/ws", wsHandler)
	r.POST("/bot/:secret", botWebhook)

	api := r.Group("/api", userAuth())
	api.GET("/me", meHandler)
	api.GET("/txs", txsHandler)
	api.POST("/withdraw", withdrawHandler)

	adm := r.Group("/admin", adminAuth())
	adm.GET("/stats", statsHandler)
	adm.GET("/deposits", listHandler("deposits"))
	adm.GET("/withdrawals", listHandler("withdrawals"))
	adm.POST("/deposits/:id/:action", depositAction)
	adm.POST("/withdrawals/:id/:action", withdrawalAction)

	r.Static("/assets", "../frontend/dist/assets")
	r.NoRoute(func(c *gin.Context) { c.File("../frontend/dist/index.html") })
	log.Fatal(r.Run(":" + cfg.Port))
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