package handlers

import (
	"encoding/json"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"strings"

	"cwmerch-api/internal/email"
	"cwmerch-api/internal/models"
	"cwmerch-api/internal/store"
	"cwmerch-api/internal/woo"

	"cwmerch-api/internal/contact"
	"cwmerch-api/internal/utils"
)

// cwcloudContactErrors maps CWCloud's own contact-request i18n_code values
// to this API's error response (status + i18n_code) - see contact.APIError.
// A code CWCloud returns that isn't in this map (including a blank one,
// since i18n_code is optional there) falls back to the generic
// CodeContactFormFailed response.
var cwcloudContactErrors = map[string]struct {
	status int
	code   string
	msg    string
}{
	"cf_rate_limiting":  {http.StatusTooManyRequests, CodeContactRateLimited, "You're sending too many messages, please try again later"},
	"message_too_short": {http.StatusBadRequest, CodeContactMessageTooShort, "Your message is too short"},
	"gibberish":         {http.StatusBadRequest, CodeContactGibberish, "Your message looks like spam, please rewrite it"},
}

type ContactHandler struct {
	contact *contact.Client
	woo     *woo.Store
	mailer  *email.Sender
	audit   auditor
}

// WithEmailFallback lets the form email the WordPress recipient (Contact
// Form 7's, cwmerch's option or the admin email) when no CWCloud contact form
// id is configured.
func (h *ContactHandler) WithEmailFallback(w *woo.Store, mailer *email.Sender, audit *store.AuditStore) *ContactHandler {
	h.woo, h.mailer, h.audit = w, mailer, newAuditor(audit)
	return h
}

func NewContactHandler(contact *contact.Client) *ContactHandler {
	return &ContactHandler{contact: contact}
}

// contactPayload is the JSON body accepted by POST /v1/contact - name and
// firstname are optional (ai-instruct-54), everything else is required. The
// caller's IP is never part of this payload (ai-instruct-56): it's resolved
// from the request's own X-Real-IP/X-Forwarded-By headers, already set by
// the reverse proxy (see contact.ResolveClientIP), and forwarded to CWCloud
// as the X-Client-IP header.
type contactPayload struct {
	Email     string `json:"email"`
	Subject   string `json:"subject"`
	Message   string `json:"message"`
	Name      string `json:"name"`
	Firstname string `json:"firstname"`
}

// Create submits the contact form to CWCloud's contact-request API. Answers
// 405 (matching ai-instruct-54's spec exactly) when CWCLOUD_CONTACT_FORM_ID
// isn't configured, rather than the 500 an unconfigured downstream
// dependency would normally get - this is a deploy-time configuration gap,
// not a request-time server error.
func (h *ContactHandler) Create(w http.ResponseWriter, r *http.Request) {
	viaEmail := !h.contact.Configured()
	if viaEmail && (h.woo == nil || h.woo.CWCloudSettings(r.Context()).ContactMode != "email") {
		writeError(w, http.StatusMethodNotAllowed, "The contact form is not configured", CodeContactFormNotConfigured)
		return
	}

	var p contactPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil ||
		utils.IsBlank(p.Email) || utils.IsBlank(p.Subject) || utils.IsBlank(p.Message) {
		writeError(w, http.StatusBadRequest, "Please add email, subject and message fields", CodeAllFieldsRequired)
		return
	}
	if !utils.IsValidEmail(p.Email) {
		writeError(w, http.StatusBadRequest, "Please add a valid email", CodeInvalidEmail)
		return
	}

	if viaEmail {
		recipient, subjectPrefix, _ := h.woo.ContactRecipient(r.Context())
		subject := strings.TrimSpace(strings.TrimSpace(subjectPrefix) + " " + p.Subject)
		name := strings.TrimSpace(p.Firstname + " " + p.Name)
		body := template.HTML("<p><strong>" + template.HTMLEscapeString(orDash(name)) + "</strong> &lt;" + template.HTMLEscapeString(p.Email) +
			"&gt;</p><p>" + strings.ReplaceAll(template.HTMLEscapeString(p.Message), "\n", "<br>") + "</p>")
		html, err := h.mailer.RenderLayout(subject, body)
		if err != nil || !h.mailer.Send(r.Context(), "", recipient, "", p.Email, subject, html, nil) {
			writeError(w, http.StatusBadGateway, "Failed to send your message, please try again later", CodeContactFormFailed)
			return
		}
		h.audit.recordAs(r.Context(), "", models.AuditContactSent, models.AuditSubjectSetting, "contact", map[string]any{"email": p.Email, "via": "email"})
		writeJSON(w, http.StatusOK, map[string]string{"message": "Your message has been sent."})
		return
	}
	err := h.contact.Send(r.Context(), contact.Submission{
		Email: p.Email, Subject: p.Subject, Message: p.Message,
		Name: p.Name, Firstname: p.Firstname,
		ClientIP: contact.ResolveClientIP(r),
	})
	if err != nil {
		var apiErr *contact.APIError
		if errors.As(err, &apiErr) {
			if mapped, ok := cwcloudContactErrors[apiErr.Code]; ok {
				writeError(w, mapped.status, mapped.msg, mapped.code)
				return
			}
		}
		slog.Error("failed to submit contact form", "error", err)
		writeError(w, http.StatusBadGateway, "Failed to send your message, please try again later", CodeContactFormFailed)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Your message has been sent."})
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
