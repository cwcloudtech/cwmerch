package woo

import (
	"context"
	"database/sql"
	"strings"

	"cwmerch-api/internal/php"
)

// WordPress options shared with the CWCloud WordPress plugin
// (cwcloud-wordpress-email stores its key in "cwcloud-api-secret"), so the
// key set on one side is the key used on the other.
const (
	OptCWCloudAPIKey      = "cwcloud-api-secret"
	OptCWCloudAPIURL      = "cwcloud-api-url"
	OptCWCloudContactForm = "cwcloud-contact-form-id"
	optContactRecipient   = "cwmerch_contact_recipient"
	optContactSubject     = "cwmerch_contact_subject"
)

// CWCloudDefaults are the environment's values, used when WordPress has none.
type CWCloudDefaults struct {
	APIURL        string
	APIKey        string
	ContactFormID string
}

// SetCWCloudDefaults wires the environment fallbacks.
func (s *Store) SetCWCloudDefaults(d CWCloudDefaults) { s.cwcloud = d }

// CWCloudSettings is the email/contact configuration as the administration reads it.
type CWCloudSettings struct {
	APIURL           string `json:"apiUrl"`
	APIKeySet        bool   `json:"apiKeySet"`
	APIKeySource     string `json:"apiKeySource"` // wordpress | env | ""
	ContactFormID    string `json:"contactFormId"`
	ContactRecipient string `json:"contactRecipient"`
	RecipientSource  string `json:"recipientSource"` // contact-form-7 | option | admin_email
	ContactSubject   string `json:"contactSubject"`
	ContactForm7ID   uint64 `json:"contactForm7Id,omitempty"`
	ContactForm7     string `json:"contactForm7,omitempty"`
	EmailFrom        string `json:"emailFrom"`
	ContactMode      string `json:"contactMode"` // contactreq | email | disabled
}

// CWCloudSettingsInput edits them (a blank apiKey keeps the stored one).
type CWCloudSettingsInput struct {
	APIURL *string `json:"apiUrl"`
	APIKey *string `json:"apiKey"`
	// ClearAPIKey removes the key stored in WordPress (the environment's, if
	// any, applies again).
	ClearAPIKey      bool    `json:"clearApiKey,omitempty"`
	ContactFormID    *string `json:"contactFormId"`
	ContactRecipient *string `json:"contactRecipient"`
	ContactSubject   *string `json:"contactSubject"`
	EmailFrom        *string `json:"emailFrom"`
}

// CWCloudAPI is the URL and key emails are sent with.
func (s *Store) CWCloudAPI(ctx context.Context) (url, key string) {
	url = strings.TrimRight(strings.TrimSpace(s.Option(ctx, OptCWCloudAPIURL)), "/")
	if url == "" {
		url = s.cwcloud.APIURL
	}
	key = strings.TrimSpace(s.Option(ctx, OptCWCloudAPIKey))
	if key == "" {
		key = s.cwcloud.APIKey
	}
	return url, key
}

// ContactFormID is the CWCloud contact form (contactreq) id, if any.
func (s *Store) ContactFormID(ctx context.Context) string {
	if v := strings.TrimSpace(s.Option(ctx, OptCWCloudContactForm)); v != "" {
		return v
	}
	return s.cwcloud.ContactFormID
}

// contactForm7 is the first published Contact Form 7 form and its mail settings.
func (s *Store) contactForm7(ctx context.Context) (uint64, string, *php.Array) {
	var id uint64
	var title string
	err := s.DB.QueryRowContext(ctx, `SELECT ID, post_title FROM `+s.T("posts")+` WHERE post_type = 'wpcf7_contact_form' AND post_status = 'publish'
		ORDER BY ID LIMIT 1`).Scan(&id, &title)
	if err != nil {
		return 0, "", nil
	}
	var raw sql.NullString
	_ = s.DB.QueryRowContext(ctx, `SELECT meta_value FROM `+s.T("postmeta")+` WHERE post_id = ? AND meta_key = '_mail' LIMIT 1`, id).Scan(&raw)
	return id, title, php.UnmarshalArray(raw.String)
}

// ContactRecipient is who receives the contact messages: Contact Form 7's
// recipient when that plugin is used, else cwmerch's option, else WordPress'
// admin email.
func (s *Store) ContactRecipient(ctx context.Context) (recipient, subject, source string) {
	subject = s.Option(ctx, optContactSubject)
	if id, _, mail := s.contactForm7(ctx); id != 0 {
		if r := strings.TrimSpace(mail.String("recipient")); r != "" && !strings.Contains(r, "[") {
			sub := mail.String("subject")
			if strings.Contains(sub, "[") {
				sub = ""
			}
			return r, orDefault(sub, subject), "contact-form-7"
		}
	}
	if r := strings.TrimSpace(s.Option(ctx, optContactRecipient)); r != "" {
		return r, subject, "option"
	}
	return s.Option(ctx, "admin_email"), subject, "admin_email"
}

// CWCloudSettings reads the configuration.
func (s *Store) CWCloudSettings(ctx context.Context) CWCloudSettings {
	url, key := s.CWCloudAPI(ctx)
	out := CWCloudSettings{APIURL: url, APIKeySet: key != "", ContactFormID: s.ContactFormID(ctx),
		EmailFrom: s.Option(ctx, "woocommerce_email_from_address")}
	switch {
	case strings.TrimSpace(s.Option(ctx, OptCWCloudAPIKey)) != "":
		out.APIKeySource = "wordpress"
	case s.cwcloud.APIKey != "":
		out.APIKeySource = "env"
	}
	out.ContactRecipient, out.ContactSubject, out.RecipientSource = s.ContactRecipient(ctx)
	if id, title, _ := s.contactForm7(ctx); id != 0 {
		out.ContactForm7ID, out.ContactForm7 = id, title
	}
	switch {
	case out.ContactFormID != "" && url != "":
		out.ContactMode = "contactreq"
	case key != "" && url != "" && out.ContactRecipient != "":
		out.ContactMode = "email"
	default:
		out.ContactMode = "disabled"
	}
	return out
}

// SaveCWCloudSettings writes the WordPress options (and Contact Form 7's mail
// settings when that is where the recipient lives).
func (s *Store) SaveCWCloudSettings(ctx context.Context, in CWCloudSettingsInput) (CWCloudSettings, error) {
	if in.APIURL != nil {
		v := strings.TrimRight(strings.TrimSpace(*in.APIURL), "/")
		if v != "" && !strings.HasPrefix(v, "https://") && !strings.HasPrefix(v, "http://") {
			return CWCloudSettings{}, refuse(400, "errors.invalidUrl", "The CWCloud API URL must start with https://")
		}
		if err := s.SetOption(ctx, OptCWCloudAPIURL, v); err != nil {
			return CWCloudSettings{}, err
		}
	}
	if in.ClearAPIKey {
		if err := s.DeleteOption(ctx, OptCWCloudAPIKey); err != nil {
			return CWCloudSettings{}, err
		}
	} else if in.APIKey != nil && strings.TrimSpace(*in.APIKey) != "" {
		if err := s.SetOption(ctx, OptCWCloudAPIKey, strings.TrimSpace(*in.APIKey)); err != nil {
			return CWCloudSettings{}, err
		}
	}
	if in.ContactFormID != nil {
		if err := s.SetOption(ctx, OptCWCloudContactForm, strings.TrimSpace(*in.ContactFormID)); err != nil {
			return CWCloudSettings{}, err
		}
	}
	if in.EmailFrom != nil {
		if err := s.SetOption(ctx, "woocommerce_email_from_address", strings.TrimSpace(*in.EmailFrom)); err != nil {
			return CWCloudSettings{}, err
		}
	}
	if in.ContactRecipient != nil || in.ContactSubject != nil {
		id, _, mail := s.contactForm7(ctx)
		if id != 0 {
			if in.ContactRecipient != nil && strings.TrimSpace(*in.ContactRecipient) != "" {
				mail.Set("recipient", strings.TrimSpace(*in.ContactRecipient))
			}
			if in.ContactSubject != nil && strings.TrimSpace(*in.ContactSubject) != "" {
				mail.Set("subject", strings.TrimSpace(*in.ContactSubject))
			}
			if err := s.setPostMeta(ctx, s.DB, id, "_mail", php.Marshal(mail)); err != nil {
				return CWCloudSettings{}, err
			}
		} else {
			if in.ContactRecipient != nil {
				if err := s.SetOption(ctx, optContactRecipient, strings.TrimSpace(*in.ContactRecipient)); err != nil {
					return CWCloudSettings{}, err
				}
			}
			if in.ContactSubject != nil {
				if err := s.SetOption(ctx, optContactSubject, strings.TrimSpace(*in.ContactSubject)); err != nil {
					return CWCloudSettings{}, err
				}
			}
		}
	}
	s.ForgetOptions()
	return s.CWCloudSettings(ctx), nil
}
