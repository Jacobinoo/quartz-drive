package email

import (
	"context"
	"fmt"
	"quartz/config"
	"quartz/pkg/contextkeys"

	"github.com/resend/resend-go/v2"
)

// SendPasswordReset prepares the HTML/Text templates and sends the recovery email
func SendPasswordReset(ctx context.Context, emailClient *resend.Client, toEmail, magicLink string) error {
	data := TemplateData{
		Email:       toEmail,
		ActionURL:   magicLink,
		FrontendURL: config.Cfg.App.FrontendURL,
	}
	addEmailSupportID(ctx, &data)

	htmlContent, err := RenderEmail(data, PasswordResetHTML, PasswordResetFooterHTML)
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

	if config.Cfg.App.Env == "development" {
		toEmail = config.Cfg.Email.LocalDeliveryAddress
	}

	from := fmt.Sprintf("%s <%s>", config.Cfg.Email.UpdatesFromSenderName, config.Cfg.Email.UpdatesVerifiedDomain)

	params := &resend.SendEmailRequest{
		From:    from,
		To:      []string{toEmail},
		Subject: "[Quartz Drive] Account recovery request",
		Html:    htmlContent,
		Text:    textContent,
	}

	opt := &resend.SendEmailOptions{}
	if emailID, ok := ctx.Value(contextkeys.EmailIDKey).(string); ok && emailID != "" {
		opt.IdempotencyKey = emailID
	}

	_, err = emailClient.Emails.SendWithOptions(ctx, params, opt)
	if err != nil {
		return fmt.Errorf("resend api failed to send email: %w", err)
	}

	return nil
}

func addEmailSupportID(ctx context.Context, templateData *TemplateData) {
	if emailID, ok := ctx.Value(contextkeys.EmailIDKey).(string); ok {
		templateData.EmailID = emailID
	}
}

// SendSignupVerification prepares the HTML/Text templates and sends the signup verification email
func SendSignupVerification(ctx context.Context, emailClient *resend.Client, toEmail, magicLink string) error {
	data := TemplateData{
		Email:       toEmail,
		ActionURL:   magicLink,
		FrontendURL: config.Cfg.App.FrontendURL,
	}
	addEmailSupportID(ctx, &data)

	htmlContent, err := RenderEmail(data, SignupVerificationHTML, SignupVerificationFooterHTML)
	if err != nil {
		return fmt.Errorf("failed to render html email template: %w", err)
	}

	textContent := fmt.Sprintf(`Quartz Drive - Verify your email address

Your Quartz Drive account (%s) is almost ready to use. To continue to Quartz Drive, please verify your email address.

Copy and paste this link into your browser:
%s

For your security, this link will expire in 15 minutes, and the Quartz Drive account will be deleted.

---
This email was sent automatically because you or someone created a Quartz Drive account with this email address.
If you didn't request this action, you don't have to do anything, just ignore it.

If you need help regarding this email, contact support and provide this identifier: %s

Quartz Drive
Terms of Service: %s/terms
Privacy Policy: %s/privacy`, data.Email, data.ActionURL, data.EmailID, data.FrontendURL, data.FrontendURL)

	if config.Cfg.App.Env == "development" {
		toEmail = config.Cfg.Email.LocalDeliveryAddress
	}

	from := fmt.Sprintf("%s <%s>", config.Cfg.Email.UpdatesFromSenderName, config.Cfg.Email.UpdatesVerifiedDomain)

	params := &resend.SendEmailRequest{
		From:    from,
		To:      []string{toEmail},
		Subject: "[Quartz Drive] Verify your email",
		Html:    htmlContent,
		Text:    textContent,
	}
	opt := &resend.SendEmailOptions{}
	if emailID, ok := ctx.Value(contextkeys.EmailIDKey).(string); ok && emailID != "" {
		opt.IdempotencyKey = emailID
	}

	_, err = emailClient.Emails.SendWithOptions(ctx, params, opt)
	if err != nil {
		return fmt.Errorf("resend api failed to send email: %w", err)
	}

	return nil
}

// SendSignupAccountExist prepares the HTML/Text templates and sends the signup account exists email
func SendSignupAccountExist(ctx context.Context, emailClient *resend.Client, toEmail string) error {
	data := TemplateData{
		Email:            toEmail,
		ActionURL:        fmt.Sprintf("%s/%s", config.Cfg.App.FrontendURL, "signin"),
		ActionRecoverURL: fmt.Sprintf("%s/%s", config.Cfg.App.FrontendURL, "forgot-password"),
		FrontendURL:      config.Cfg.App.FrontendURL,
	}
	addEmailSupportID(ctx, &data)

	htmlContent, err := RenderEmail(data, SignupAccountExistsHTML, SignupAccountExistsFooterHTML)
	if err != nil {
		return fmt.Errorf("failed to render html email template: %w", err)
	}

	textContent := fmt.Sprintf(`Quartz Drive - Account already exists

Someone recently tried to create a new Quartz account using your email address (%s).
However, this email is already registered with us.

If this was you: You don't need to do anything, you can just log in to your account here: %s

Forgot your password?: If you were trying to sign up because you forgot your password, you can use your Recovery Phrase to recover your account here: %s

If this wasn't you: You can safely ignore this email. Your account remains completely secure, and the person who attempted to sign up cannot access your data.

---
This email was sent automatically because someone attempted to create a new Quartz Drive account with this email address, which is already registered.
If you didn't request this action, you don't have to do anything, just ignore this email.

If you need help regarding this email, contact support and provide this identifier: %s

Quartz Drive
Terms of Service: %s/terms
Privacy Policy: %s/privacy`, data.Email, data.ActionURL, data.ActionRecoverURL, data.EmailID, data.FrontendURL, data.FrontendURL)

	if config.Cfg.App.Env == "development" {
		toEmail = config.Cfg.Email.LocalDeliveryAddress
	}

	from := fmt.Sprintf("%s <%s>", config.Cfg.Email.UpdatesFromSenderName, config.Cfg.Email.UpdatesVerifiedDomain)

	params := &resend.SendEmailRequest{
		From:    from,
		To:      []string{toEmail},
		Subject: "[Quartz Drive] Account already exists",
		Html:    htmlContent,
		Text:    textContent,
	}
	opt := &resend.SendEmailOptions{}
	if emailID, ok := ctx.Value(contextkeys.EmailIDKey).(string); ok && emailID != "" {
		opt.IdempotencyKey = emailID
	}

	_, err = emailClient.Emails.SendWithOptions(ctx, params, opt)
	if err != nil {
		return fmt.Errorf("resend api failed to send email: %w", err)
	}

	return nil
}
