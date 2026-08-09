package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRateLimitIP_Strict(t *testing.T) {
	// Strict limit: 5 requests per minute, burst of 2.
	// This means we can only do 2 requests immediately. The 3rd should fail unless time passes.
	middleware := RateLimitIP(5, time.Minute, 2)
	handler := middleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	var successCount int32
	var failCount int32

	var wg sync.WaitGroup
	// Fire 100 requests concurrently from the same mocked IP
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("X-Forwarded-For", "192.168.1.100")
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code == http.StatusOK {
				atomic.AddInt32(&successCount, 1)
			} else if rr.Code == http.StatusTooManyRequests {
				atomic.AddInt32(&failCount, 1)
			}
		}()
	}
	wg.Wait()

	if successCount != 2 {
		t.Errorf("Expected exactly 2 successful requests (burst 2), got %d", successCount)
	}
	if failCount != 98 {
		t.Errorf("Expected exactly 98 failed requests, got %d", failCount)
	}
}

func TestRateLimitUser_Standard(t *testing.T) {
	// Standard limit: 1 request per sec, burst 50.
	// We use 1 req/sec to prevent the bucket from refilling while the test executes.
	middleware := RateLimitUser(1, time.Second, 50)
	handler := middleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	userID := uuid.New()
	var successCount int32
	var failCount int32

	var wg sync.WaitGroup
	// Fire 500 requests concurrently
	for i := 0; i < 500; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/", nil)
			ctx := context.WithValue(req.Context(), UserIDKey, userID)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code == http.StatusOK {
				atomic.AddInt32(&successCount, 1)
			} else if rr.Code == http.StatusTooManyRequests {
				atomic.AddInt32(&failCount, 1)
			}
		}()
	}
	wg.Wait()

	if successCount != 50 {
		t.Errorf("Expected exactly 50 successful requests (burst 50), got %d", successCount)
	}
	if failCount != 450 {
		t.Errorf("Expected exactly 450 failed requests, got %d", failCount)
	}
}

func TestRateLimitStacked(t *testing.T) {
	// Stacked middleware: Loose IP limit, Strict User limit.
	// Loose IP: burst 100
	// Strict User: burst 50

	looseIP := RateLimitIP(300, time.Second, 100)
	strictUser := RateLimitUser(1, time.Second, 50)

	handler := looseIP(strictUser(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	userID := uuid.New()
	var successCount int32
	var failCount int32

	var wg sync.WaitGroup
	// Fire 200 requests concurrently from the same user & IP
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("X-Forwarded-For", "10.0.0.5")
			ctx := context.WithValue(req.Context(), UserIDKey, userID)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code == http.StatusOK {
				atomic.AddInt32(&successCount, 1)
			} else if rr.Code == http.StatusTooManyRequests {
				atomic.AddInt32(&failCount, 1)
			}
		}()
	}
	wg.Wait()

	// Since User limit is stricter (burst 50), it should cap the successes at 50, even though IP allows 100.
	if successCount != 50 {
		t.Errorf("Expected exactly 50 successful requests due to strict User limit, got %d", successCount)
	}
	if failCount != 150 {
		t.Errorf("Expected exactly 150 failed requests, got %d", failCount)
	}
}
