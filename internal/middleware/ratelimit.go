package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/time/rate"
)

type client struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}


func cleanupClients(clientsMap *sync.Map) {
	now := time.Now()
	clientsMap.Range(func(key, value interface{}) bool {
		c := value.(*client)
		// If the client hasn't made a request in 3 minutes, remove their bucket
		if now.Sub(c.lastSeen) > 3*time.Minute {
			clientsMap.Delete(key)
		}
		return true
	})
}

// extractIP gets the true IP address even behind proxies
func extractIP(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		ips := strings.Split(xff, ",")
		return strings.TrimSpace(ips[0])
	}
	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		return realIP
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func getLimiter(clientsMap *sync.Map, key string, r rate.Limit, b int) *rate.Limiter {
	if v, ok := clientsMap.Load(key); ok {
		c := v.(*client)
		c.lastSeen = time.Now()
		return c.limiter
	}

	limiter := rate.NewLimiter(r, b)
	clientsMap.Store(key, &client{
		limiter:  limiter,
		lastSeen: time.Now(),
	})
	return limiter
}

// RateLimitIP limits based on client IP address
func RateLimitIP(requests int, per time.Duration, burst int) func(http.HandlerFunc) http.HandlerFunc {
	limit := rate.Limit(float64(requests) / per.Seconds())
	clientsMap := &sync.Map{}

	go func() {
		for {
			time.Sleep(time.Minute)
			cleanupClients(clientsMap)
		}
	}()

	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ip := extractIP(r)
			limiter := getLimiter(clientsMap, ip, limit, burst)
			
			if !limiter.Allow() {
				http.Error(w, "429 Too Many Requests", http.StatusTooManyRequests)
				return
			}
			
			next.ServeHTTP(w, r)
		}
	}
}

// RateLimitUser limits based on UserID from JWT context
func RateLimitUser(requests int, per time.Duration, burst int) func(http.HandlerFunc) http.HandlerFunc {
	limit := rate.Limit(float64(requests) / per.Seconds())
	clientsMap := &sync.Map{}

	go func() {
		for {
			time.Sleep(time.Minute)
			cleanupClients(clientsMap)
		}
	}()

	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// Extract UUID from context
			userIDVal := r.Context().Value(UserIDKey)
			if userIDVal == nil {
				// Fallback if somehow placed before Auth middleware
				http.Error(w, "Missing Context Data", http.StatusInternalServerError)
				return
			}
			
			userID, ok := userIDVal.(uuid.UUID)
			if !ok {
				http.Error(w, "Invalid Context Data", http.StatusInternalServerError)
				return
			}
			
			limiter := getLimiter(clientsMap, userID.String(), limit, burst)
			if !limiter.Allow() {
				http.Error(w, "429 Too Many Requests", http.StatusTooManyRequests)
				return
			}
			
			next.ServeHTTP(w, r)
		}
	}
}
