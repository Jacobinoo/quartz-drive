package dev

import (
	"net/http"
	"quartz/config"
	apperrors "quartz/pkg/app-errors"
	"quartz/pkg/email"
)

// PreviewPasswordReset renders the raw HTML of the email directly to the browser
func PreviewPasswordReset(w http.ResponseWriter, r *http.Request) error {
	// Dummy data for the visual preview
	data := email.TemplateData{
		Email:       "test-user@quartz.com",
		ActionURL:   config.Cfg.App.FrontendURL + "/reset-password?token=preview-token-12345",
		FrontendURL: config.Cfg.App.FrontendURL,
	}

	html, err := email.RenderPasswordResetEmail(data)
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
