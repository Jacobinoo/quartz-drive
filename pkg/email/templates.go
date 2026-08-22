package email

import (
	"bytes"
	"fmt"
	"html/template"
)

// TemplateData holds the dynamic variables we inject into our emails
type TemplateData struct {
	Email       string
	ActionURL   string
	FrontendURL string
}

// The Base Layout (Contains Header, Footer, and the {{template "content"}} injection point + additional footer text {{template "footerAdditionalText"}} injection point)
const baseLayoutHTML = `
<!DOCTYPE html>
<html>
<head>
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <meta http-equiv="Content-Type" content="text/html; charset=UTF-8">
</head>
<body style="background-color: #f3f4f6; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased; margin: 0; padding: 0;">
    <table role="presentation" border="0" cellpadding="0" cellspacing="0" style="width: 100%; background-color: #f3f4f6;">
        <tr>
            <td style="padding: 30px 0 20px 0; text-align: center;">
                <h1 style="color: #111827; margin: 0; font-size: 24px; font-weight: 700; letter-spacing: -0.5px;">Quartz Drive</h1>
            </td>
        </tr>
        <tr>
            <td align="center">
                <table role="presentation" border="0" cellpadding="0" cellspacing="0" style="width: 100%; max-width: 600px; background: #ffffff; border-radius: 8px; overflow: hidden; margin-bottom: 20px; box-shadow: 0 1px 3px rgba(0,0,0,0.1);">
                    <tr>
                        <td style="padding: 40px;">
                            {{template "content" .}}
                        </td>
                    </tr>
                </table>
            </td>
        </tr>
        <tr>
            <td align="center" style="padding: 0 20px 40px 20px;">
                <p style="color: #6b7280; font-size: 12px; margin: 0 0 10px 0; text-align: center;">
					{{template "footerAdditionalText" .}}
                </p>
                <p style="color: #6b7280; font-size: 12px; margin: 0; text-align: center;">
                    &copy; 2026 Quartz Drive. 
                    <a href="{{.FrontendURL}}/terms" style="color: #6b7280; text-decoration: underline;">Terms of Service</a> &bull; 
                    <a href="{{.FrontendURL}}/privacy" style="color: #6b7280; text-decoration: underline;">Privacy Policy</a>
                </p>
            </td>
        </tr>
    </table>
</body>
</html>
`

const PasswordResetHTML = `
{{define "content"}}
<h2 style="color: #111827; margin: 0 0 20px 0; font-size: 20px; font-weight: 600;">Reset your password</h2>
<p style="color: #374151; margin: 0 0 24px 0; font-size: 16px; line-height: 24px;">
    We received a request to reset the password for your Quartz account (<strong>{{.Email}}</strong>). Click the button below to securely set up a new password.
</p>

<!-- Fallback button wrapper required for Outlook -->
<table role="presentation" border="0" cellpadding="0" cellspacing="0" style="margin-bottom: 32px;">
    <tr>
        <td align="center" bgcolor="#111827" style="border-radius: 6px;">
            <a href="{{.ActionURL}}" style="display: inline-block; padding: 14px 28px; font-size: 16px; font-weight: 600; color: #ffffff; text-decoration: none; border-radius: 6px; background-color: #111827; border: 1px solid #111827;">Reset Password</a>
        </td>
    </tr>
</table>

<p style="color: #6b7280; margin: 0 0 8px 0; font-size: 13px;">Or copy and paste this link into your browser:</p>
<p style="margin: 0; font-size: 13px; word-break: break-all;">
    <a href="{{.ActionURL}}" style="color: #2563eb; text-decoration: underline;">{{.ActionURL}}</a>
</p>
<p style="color: #6b7280; margin: 24px 0 0 0; font-size: 13px;">For your security, this link will expire in 15 minutes.</p>
{{end}}
`

const PasswordResetFooterHTML = `
{{define "footerAdditionalText"}}
This email was sent automatically. If you didn't request this action, please ignore it and your account will remain secure.
{{end}}`

const SignupVerificationFooterHTML = `
{{define "footerAdditionalText"}}
This email was sent automatically because you created a Quartz Drive account.<br>If you didn't request this action, please ignore it, and the account assigned to your email will be deleted.
{{end}}
`

const SignupVerificationHTML = `
{{define "content"}}
<h2 style="color: #111827; margin: 0 0 20px 0; font-size: 20px; font-weight: 600;">Verify your email address</h2>
<p style="color: #374151; margin: 0 0 24px 0; font-size: 16px; line-height: 24px;">
    Your Quartz Drive account (<strong>{{.Email}}</strong>) is almost ready to use.<br>To continue to Quartz Drive, please verify your email address.
</p>

<!-- Fallback button wrapper required for Outlook -->
<table role="presentation" border="0" cellpadding="0" cellspacing="0" style="margin-bottom: 32px;">
    <tr>
        <td align="center" bgcolor="#111827" style="border-radius: 6px;">
            <a href="{{.ActionURL}}" style="display: inline-block; padding: 14px 28px; font-size: 16px; font-weight: 600; color: #ffffff; text-decoration: none; border-radius: 6px; background-color: #111827; border: 1px solid #111827;">Verify your email address</a>
        </td>
    </tr>
</table>

<p style="color: #6b7280; margin: 0 0 8px 0; font-size: 13px;">Or copy and paste this link into your browser:</p>
<p style="margin: 0; font-size: 13px; word-break: break-all;">
    <a href="{{.ActionURL}}" style="color: #2563eb; text-decoration: underline;">{{.ActionURL}}</a>
</p>
<p style="color: #6b7280; margin: 24px 0 0 0; font-size: 13px;">For your security, this link will expire in 15 minutes, and the Quartz Drive account will be deleted.</p>
{{end}}
`

// RenderEmail parses the layout and injects the content
func RenderEmail(data TemplateData, html string, footerTextHtml string) (string, error) {
	// Parse the base layout, then parse the child content into it
	t, err := template.New("layout").Parse(baseLayoutHTML)
	if err != nil {
		return "", fmt.Errorf("failed to parse layout: %w", err)
	}

	t, err = t.Parse(html)
	if err != nil {
		return "", fmt.Errorf("failed to parse content: %w", err)
	}

	t, err = t.Parse(footerTextHtml)
	if err != nil {
		return "", fmt.Errorf("failed to parse footer text: %w", err)
	}

	// Execute the template, passing our dynamic variables
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}
