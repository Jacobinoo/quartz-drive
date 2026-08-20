package captcha

import (
	"encoding/json"
	"net/http"
	"net/url"
	"quartz/config"
)

func VerifyCaptchaToken(token, ip string) (bool, []string, error) {
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
