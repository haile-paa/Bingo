// backend/security.go - rate limits, safe logging and request guards for public use
package main

import (
	"crypto/subtle"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// limiter is a small sliding-window rate limiter (no extra dependencies).
type limiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

var rl = newLimiter()

func newLimiter() *limiter {
	l := &limiter{hits: map[string][]time.Time{}}
	go func() { // forget idle keys so memory stays small
		for {
			time.Sleep(10 * time.Minute)
			cut := time.Now().Add(-2 * time.Hour)
			l.mu.Lock()
			for k, h := range l.hits {
				if len(h) == 0 || h[len(h)-1].Before(cut) {
					delete(l.hits, k)
				}
			}
			l.mu.Unlock()
		}
	}()
	return l
}

// allow reports whether key may do one more thing: at most n times per period.
func (l *limiter) allow(key string, n int, per time.Duration) bool {
	now := time.Now()
	cut := now.Add(-per)
	l.mu.Lock()
	defer l.mu.Unlock()
	h := l.hits[key]
	i := 0
	for i < len(h) && h[i].Before(cut) {
		i++
	}
	h = h[i:]
	if len(h) >= n {
		l.hits[key] = h
		return false
	}
	l.hits[key] = append(h, now)
	return true
}

// accessLog logs method, path, status and time - never the query string (it carries the login token)
// and never the webhook secret in /bot/<secret>.
func accessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/bot/") {
			path = "/bot/***"
		}
		if path == "/health" && c.Writer.Status() == 200 {
			return
		}
		log.Printf("%d %s %s %s", c.Writer.Status(), c.Request.Method, path, time.Since(start).Round(time.Millisecond))
	}
}

func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(c.Request.URL.Path, "/api/") || strings.HasPrefix(c.Request.URL.Path, "/admin/") {
			h.Set("Cache-Control", "no-store")
		}
		c.Next()
	}
}

// bodyLimit stops huge request bodies (the biggest legitimate body is a pasted SMS).
func bodyLimit(n int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, n)
		c.Next()
	}
}

// originOK accepts WebSocket connections only from this site, MINIAPP_URL or FRONTEND_ORIGIN.
// Telegram's mobile apps send no Origin header, which is also fine.
func originOK(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	u, err := url.Parse(o)
	if err != nil {
		return false
	}
	allowed := []string{r.Host}
	for _, v := range []string{cfg.MiniAppURL, os.Getenv("FRONTEND_ORIGIN")} {
		if pu, err := url.Parse(v); err == nil && pu.Host != "" {
			allowed = append(allowed, pu.Host)
		}
	}
	for _, h := range allowed {
		if strings.EqualFold(u.Host, h) {
			return true
		}
	}
	return false
}

func sameSecret(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }

// countUser is how many sockets one user has open in this room.
func (g *Game) countUser(uid int64) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	n := 0
	for cl := range g.clients {
		if cl.uid == uid {
			n++
		}
	}
	return n
}
