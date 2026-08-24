package captcha

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"quartz/config"
	"quartz/pkg/httputils"
	"strings"
)

func VerifyCaptchaTokenInRequest(r *http.Request) (bool, []string, error) {
	ip := httputils.GetRequestIp(r)

	header := r.Header.Get("X-Verify-Token")
	token := strings.TrimSpace(header)
	if token == "" {
		return false, []string{"no captcha token in request headers"}, fmt.Errorf("no captcha token in request headers")
	}

	form := url.Values{
		"secret":   {config.Cfg.Security.CaptchaSecret},
		"response": {token},
		"remoteip": {ip},
		"sitekey":  {config.Cfg.Security.CaptchaSiteKey},
	}
	resp, err := http.PostForm(
		"https://api.hcaptcha.com/siteverify",
		form,
	)
	if err != nil {
		return false, nil, err
	}
	defer resp.Body.Close()

	var out struct {
		Success    bool     `json:"success"`
		ErrorCodes []string `json:"error-codes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return false, nil, err
	}
	if out.Success {
		return true, []string{}, nil
	}
	return false, out.ErrorCodes, nil
}

func VerifyTurnstileTokenInRequest(r *http.Request) (bool, []string, error) {
	ip := httputils.GetRequestIp(r)

	header := r.Header.Get("X-Verify-Token")
	token := strings.TrimSpace(header)
	if token == "" {
		return false, []string{"no turnstile token in request headers"}, fmt.Errorf("no turnstile token in request headers")
	}

	form := url.Values{
		"secret":   {config.Cfg.Security.TurnstileSecret},
		"response": {token},
		"remoteip": {ip},
	}
	resp, err := http.PostForm(
		"https://challenges.cloudflare.com/turnstile/v0/siteverify",
		form,
	)
	if err != nil {
		return false, nil, err
	}
	defer resp.Body.Close()

	var out struct {
		Success    bool     `json:"success"`
		ErrorCodes []string `json:"error-codes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return false, nil, err
	}
	if out.Success {
		return true, []string{}, nil
	}
	return false, out.ErrorCodes, nil
}
