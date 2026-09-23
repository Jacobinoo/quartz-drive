package middleware

import (
	"net"
	"net/http"
	"strings"
	"time"

	"quartz/pkg/app-errors"
	"quartz/pkg/contextkeys"
	"quartz/pkg/httputils"

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
func RateLimitIP(rdb *redis.Client, name string, requests int, per time.Duration, burst int) func(httputils.APIHandler) httputils.APIHandler {
	limiter := redis_rate.NewLimiter(rdb)
	limit := redis_rate.Limit{
		Rate:   requests,
		Burst:  burst,
		Period: per,
	}

	return func(next httputils.APIHandler) httputils.APIHandler {
		return func(w http.ResponseWriter, r *http.Request) error {
			ip := extractIP(r)
			res, err := limiter.Allow(r.Context(), "rate:ip:"+name+":"+ip, limit)

			if err != nil {
				// Fail open if Redis is temporarily unreachable
				return next(w, r)
			}

			if res.Allowed == 0 {
				return apperrors.NewRateLimited("429 Too Many Requests")
			}

			return next(w, r)
		}
	}
}

// RateLimitUser limits based on UserID from JWT context
func RateLimitUser(rdb *redis.Client, name string, requests int, per time.Duration, burst int) func(httputils.APIHandler) httputils.APIHandler {
	limiter := redis_rate.NewLimiter(rdb)
	limit := redis_rate.Limit{
		Rate:   requests,
		Burst:  burst,
		Period: per,
	}

	return func(next httputils.APIHandler) httputils.APIHandler {
		return func(w http.ResponseWriter, r *http.Request) error {
			// Extract UUID from context
			userIDVal := r.Context().Value(contextkeys.UserIDKey)
			if userIDVal == nil {
				// Fallback if somehow placed before Auth middleware
				return apperrors.NewInternal(nil)
			}

			userID, ok := userIDVal.(uuid.UUID)
			if !ok {
				return apperrors.NewInternal(nil)
			}

			res, err := limiter.Allow(r.Context(), "rate:user:"+name+":"+userID.String(), limit)
			if err != nil {
				return next(w, r)
			}

			if res.Allowed == 0 {
				return apperrors.NewRateLimited("429 Too Many Requests")
			}

			return next(w, r)
		}
	}
}
