package httputils

import "net/http"

func GetRequestIp(r *http.Request) string {
	ip := r.Header.Get("CF-Connecting-IP")
	if ip == "" {
		ip = r.Header.Get("X-Forwarded-For")
	}
	if ip == "" {
		ip = r.Header.Get("X-Real-IP")
	}
	return ip
}
