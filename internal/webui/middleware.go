package webui

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

// CSRF token — generated once at startup, checked on mutating requests
var (
	csrfToken     string
	csrfTokenOnce sync.Once
)

func getCSRFToken() string {
	csrfTokenOnce.Do(func() {
		b := make([]byte, 16)
		rand.Read(b)
		csrfToken = hex.EncodeToString(b)
	})
	return csrfToken
}

func csrfMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set token cookie on every response
		http.SetCookie(w, &http.Cookie{
			Name:     "veil_csrf",
			Value:    getCSRFToken(),
			Path:     "/",
			HttpOnly: false, // JS needs to read it for htmx
			SameSite: http.SameSiteStrictMode,
		})

		// Check token on mutating requests
		if r.Method == "POST" || r.Method == "PUT" || r.Method == "DELETE" {
			// Accept from header (htmx sends this) or form value
			token := r.Header.Get("X-CSRF-Token")
			if token == "" {
				token = r.FormValue("csrf_token")
			}
			if token == "" {
				// Also check the cookie itself for same-origin verification
				cookie, err := r.Cookie("veil_csrf")
				if err != nil || cookie.Value != getCSRFToken() {
					http.Error(w, "CSRF token missing", http.StatusForbidden)
					return
				}
			} else if token != getCSRFToken() {
				http.Error(w, "CSRF token invalid", http.StatusForbidden)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// Simple rate limiter — per-endpoint, token bucket
type rateLimiter struct {
	mu       sync.Mutex
	tokens   float64
	max      float64
	rate     float64 // tokens per second
	lastTime time.Time
}

func newRateLimiter(maxBurst int, perSecond float64) *rateLimiter {
	return &rateLimiter{
		tokens:   float64(maxBurst),
		max:      float64(maxBurst),
		rate:     perSecond,
		lastTime: time.Now(),
	}
}

func (rl *rateLimiter) allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastTime).Seconds()
	rl.lastTime = now
	rl.tokens += elapsed * rl.rate
	if rl.tokens > rl.max {
		rl.tokens = rl.max
	}

	if rl.tokens < 1 {
		return false
	}
	rl.tokens--
	return true
}

var apiLimiter = newRateLimiter(30, 5) // 30 burst, 5/sec

func rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !apiLimiter.allow() {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
