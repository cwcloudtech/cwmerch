// Package email sends transactional emails through CWCloud's email API
// (POST {CWCLOUD_API_URL}/v1/email, X-Auth-Token).
package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"cwmerch-api/internal/templates"
	"cwmerch-api/internal/utils"
)

// Attachment is a base64 file attached to an email.
type Attachment struct {
	MimeType string `json:"mime_type"`
	FileName string `json:"file_name"`
	B64      string `json:"b64"`
}

type request struct {
	From        string       `json:"from"`
	To          string       `json:"to"`
	Cc          string       `json:"cc,omitempty"`
	Bcc         string       `json:"bcc,omitempty"`
	ReplyTo     string       `json:"reply_to,omitempty"`
	Subject     string       `json:"subject"`
	Content     string       `json:"content"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

// Sender posts emails to CWCloud's email API.
type Sender struct {
	apiURL      string
	apiKey      string
	from        string
	selfBaseURL string
	client      *http.Client
	// OnFailure is told about every email that could not be sent (Discord alert).
	OnFailure func(ctx context.Context, to, subject, detail string)
	// Resolve reads the CWCloud URL and key at send time (the WordPress
	// option shared with the CWCloud plugin, set from the administration).
	Resolve func(ctx context.Context) (apiURL, apiKey string)
}

func (s *Sender) creds(ctx context.Context) (string, string) {
	if s.Resolve != nil {
		if url, key := s.Resolve(ctx); url != "" && key != "" {
			return url, key
		}
	}
	return s.apiURL, s.apiKey
}

// NewSender builds a Sender; a blank apiURL/apiKey disables sending (logged).
func NewSender(apiURL, apiKey, from, selfBaseURL string) *Sender {
	return &Sender{apiURL: apiURL, apiKey: apiKey, from: from, selfBaseURL: selfBaseURL, client: &http.Client{Timeout: 15 * time.Second}}
}

// Configured reports whether emails can actually be sent.
func (s *Sender) Configured() bool {
	url, key := s.creds(context.Background())
	return utils.IsNotBlank(url) && utils.IsNotBlank(key)
}

// From is the default sender address.
func (s *Sender) From() string { return s.from }

var bodyTemplate = template.Must(template.New("email").Parse(templates.EmailHTML))

const buttonStyle = "display:inline-block;margin-top:8px;padding:10px 20px;background-color:#3b8fe0;color:#ffffff;" +
	"font-weight:600;border-radius:6px;text-decoration:none;"
const mutedStyle = "color:#64748b;"
const centerStyle = "text-align:center;"

// RenderLayout wraps body in cwmerch's account-email layout.
func (s *Sender) RenderLayout(title string, body template.HTML) (string, error) {
	var buf bytes.Buffer
	err := bodyTemplate.Execute(&buf, struct {
		Title string
		Logo  template.URL
		Body  template.HTML
	}{Title: title, Logo: template.URL(s.LogoURL()), Body: body})
	return buf.String(), err
}

// LogoURL is the hosted logo emails reference (mail clients strip data: URIs).
func (s *Sender) LogoURL() string { return s.selfBaseURL + "/v1/assets/logo.png" }

// Send posts one email best-effort (from "" = the default address).
func (s *Sender) Send(ctx context.Context, from, to, cc, replyTo, subject, htmlContent string, attachments []Attachment) bool {
	if utils.IsBlank(from) {
		from = s.from
	}
	apiURL, apiKey := s.creds(ctx)
	if utils.IsBlank(apiURL) || utils.IsBlank(apiKey) {
		slog.WarnContext(ctx, "cwcloud email api is not configured (shop settings or CWCLOUD_API_KEY), skipping email", "to", to, "subject", subject)
		return false
	}
	payload := request{From: from, To: to, Cc: cc, ReplyTo: replyTo, Subject: subject, Content: htmlContent, Attachments: attachments}
	body, err := json.Marshal(payload)
	if err != nil {
		slog.ErrorContext(ctx, "failed to marshal email payload", "error", err)
		return false
	}
	req, err := http.NewRequestWithContext(context.WithoutCancel(ctx), http.MethodPost, apiURL+"/v1/email", bytes.NewReader(body))
	if err != nil {
		slog.ErrorContext(ctx, "failed to build cwcloud email request", "error", err)
		return false
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Auth-Token", apiKey)
	resp, err := s.client.Do(req)
	if err != nil {
		slog.ErrorContext(ctx, "cwcloud email api is not available", "error", err, "to", to, "subject", subject)
		s.fail(ctx, to, subject, err.Error())
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		slog.ErrorContext(ctx, "cwcloud email api returned an error", "status", resp.StatusCode, "to", to, "subject", subject)
		s.fail(ctx, to, subject, fmt.Sprintf("HTTP %d", resp.StatusCode))
		return false
	}
	slog.InfoContext(ctx, "email sent", "to", to, "subject", subject)
	return true
}

func (s *Sender) fail(ctx context.Context, to, subject, detail string) {
	if s.OnFailure != nil {
		s.OnFailure(ctx, to, subject, detail)
	}
}

func (s *Sender) cta(ctx context.Context, to, title, intro, button, link string) {
	body := template.HTML(fmt.Sprintf(`<p>%s</p><p style="%s"><a href="%s" style="%s">%s</a></p><p style="%s">%s</p>`,
		template.HTMLEscapeString(intro), centerStyle, template.HTMLEscapeString(link), buttonStyle, template.HTMLEscapeString(button),
		mutedStyle, "If you did not ask for this, you can safely ignore this email."))
	html, err := s.RenderLayout(title, body)
	if err != nil {
		slog.ErrorContext(ctx, "failed to render email", "error", err)
		return
	}
	s.Send(ctx, "", to, "", "", title, html, nil)
}

// SendConfirmation emails the account-confirmation link (activation mode "email").
func (s *Sender) SendConfirmation(ctx context.Context, to, confirmURL string) {
	s.cta(ctx, to, "Confirm your account", "Welcome! Please confirm your account by clicking the button below.", "Confirm my account", confirmURL)
}

// SendPasswordReset emails the password-renewal link.
func (s *Sender) SendPasswordReset(ctx context.Context, to, resetURL string) {
	s.cta(ctx, to, "Reset your password", "We received a request to reset your password.", "Choose a new password", resetURL)
}
