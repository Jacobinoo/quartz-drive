package dev

import (
	"net/http"
	"quartz/config"
	apperrors "quartz/pkg/app-errors"
	"quartz/pkg/email"
)

// PreviewPasswordReset renders the raw HTML of the email directly to the browser
func PreviewPasswordReset(w http.ResponseWriter, r *http.Request) error {
	data := email.TemplateData{
		Email:       "test-user@quartz.com",
		ActionURL:   config.Cfg.App.FrontendURL + "/reset-password#token=preview-token-12345",
		FrontendURL: config.Cfg.App.FrontendURL,
	}

	html, err := email.RenderEmail(data, email.PasswordResetHTML, email.PasswordResetFooterHTML)
	if err != nil {
		return apperrors.NewInternal(err)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write([]byte(html))
	if err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}

func PreviewSignupVerification(w http.ResponseWriter, r *http.Request) error {
	data := email.TemplateData{
		Email:       "test-user@quartz.com",
		ActionURL:   config.Cfg.App.FrontendURL + "/verify-email#token=preview-token-12345",
		FrontendURL: config.Cfg.App.FrontendURL,
	}

	html, err := email.RenderEmail(data, email.SignupVerificationHTML, email.SignupVerificationFooterHTML)
	if err != nil {
		return apperrors.NewInternal(err)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write([]byte(html))
	if err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}

func PreviewSignupAccountExists(w http.ResponseWriter, r *http.Request) error {
	data := email.TemplateData{
		Email:            "test-user@quartz.com",
		ActionURL:        config.Cfg.App.FrontendURL + "/signin",
		ActionRecoverURL: config.Cfg.App.FrontendURL + "/forgot-password",
		FrontendURL:      config.Cfg.App.FrontendURL,
	}

	html, err := email.RenderEmail(data, email.SignupAccountExistsHTML, email.SignupAccountExistsFooterHTML)
	if err != nil {
		return apperrors.NewInternal(err)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write([]byte(html))
	if err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}
