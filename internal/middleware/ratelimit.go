package middleware

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-redis/redis_rate/v10"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// extractIP gets the true IP address even behind proxies
func extractIP(r *http.Request) string {
	// ALWAYS trust Cloudflare's IP header first
	if cfIP := r.Header.Get("CF-Connecting-IP"); cfIP != "" {
		return cfIP
	}

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

// RateLimitIP limits based on client IP address
func RateLimitIP(rdb *redis.Client, name string, requests int, per time.Duration, burst int) func(http.HandlerFunc) http.HandlerFunc {
	limiter := redis_rate.NewLimiter(rdb)
	limit := redis_rate.Limit{
		Rate:   requests,
		Burst:  burst,
		Period: per,
	}

	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ip := extractIP(r)
			res, err := limiter.Allow(r.Context(), "rate:ip:"+name+":"+ip, limit)

			if err != nil {
				// Fail open if Redis is temporarily unreachable
				next.ServeHTTP(w, r)
				return
			}

			if res.Allowed == 0 {
				http.Error(w, "429 Too Many Requests", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		}
	}
}

// RateLimitUser limits based on UserID from JWT context
func RateLimitUser(rdb *redis.Client, name string, requests int, per time.Duration, burst int) func(http.HandlerFunc) http.HandlerFunc {
	limiter := redis_rate.NewLimiter(rdb)
	limit := redis_rate.Limit{
		Rate:   requests,
		Burst:  burst,
		Period: per,
	}

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

			res, err := limiter.Allow(r.Context(), "rate:user:"+name+":"+userID.String(), limit)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			if res.Allowed == 0 {
				http.Error(w, "429 Too Many Requests", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		}
	}
}
