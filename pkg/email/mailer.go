package email

import (
	"context"
	"fmt"
	"quartz/config"

	"github.com/resend/resend-go/v2"
)

// SendPasswordReset prepares the HTML/Text templates and sends the recovery email via Resend
func SendPasswordReset(ctx context.Context, toEmail, magicLink string) error {
	data := TemplateData{
		Email:       toEmail,
		ActionURL:   magicLink,
		FrontendURL: config.Cfg.App.FrontendURL,
	}

	htmlContent, err := RenderPasswordResetEmail(data)
	if err != nil {
		return fmt.Errorf("failed to render html email template: %w", err)
	}

	textContent := fmt.Sprintf(`Quartz Drive - Reset your password

We received a request to reset the password for your Quartz account (%s).

Copy and paste this link into your browser to reset your password:
%s

This link will expire in 15 minutes.

---
This email was sent automatically. If you didn't request this action, please ignore it and your account will remain secure.

Terms of Service: %s/terms
Privacy Policy: %s/privacy`, data.Email, data.ActionURL, data.FrontendURL, data.FrontendURL)

	client := resend.NewClient(config.Cfg.Email.Key)

	from := fmt.Sprintf("%s <%s>", config.Cfg.Email.UpdatesFromSenderName, config.Cfg.Email.UpdatesVerifiedDomain)

	params := &resend.SendEmailRequest{
		From:    from,
		To:      []string{toEmail},
		Subject: "[Quartz Drive] Account recovery request",
		Html:    htmlContent,
		Text:    textContent,
	}

	// should this be sent asynchronously? are we blocking the request?
	// TODO: support idempotency keys to avoid duplicates
	opt := &resend.SendEmailOptions{
		//IdempotencyKey: "",
	}
	_, err = client.Emails.SendWithOptions(ctx, params, opt)
	if err != nil {
		return fmt.Errorf("resend api failed to send email: %w", err)
	}

	return nil
}
