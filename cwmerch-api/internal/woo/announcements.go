package woo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// ShopAnnouncement placements.
const (
	PlacementHome = "home" // the home page only
	PlacementAll  = "all"  // every storefront page
)

// ShopAnnouncement is a banner at the top of the storefront, managed in the back
// office (cwmerch_announcements). It shows while Enabled and not expired.
type ShopAnnouncement struct {
	ID uint64 `json:"id"`
	// Title is the back office's label (not shown on the storefront).
	Title string `json:"title"`
	// Content is the HTML written with the WYSIWYG editor.
	Content   string `json:"content"`
	Enabled   bool   `json:"enabled"`
	Placement string `json:"placement"`
	// Background: "" = from the theme (translucent primary colour), else #rrggbb.
	Background  string `json:"background"`
	Dismissible bool   `json:"dismissible"`
	Position    int    `json:"position"`
	// ExpiresAt (UTC): the announcement disappears afterwards; nil = no end.
	ExpiresAt *time.Time `json:"expiresAt"`
	// Expired is computed (ExpiresAt is past).
	Expired   bool      `json:"expired"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// AnnouncementInput is what the back office sends. expiresAt is RFC 3339, or
// "" / null for no end.
type AnnouncementInput struct {
	Title       string  `json:"title"`
	Content     string  `json:"content"`
	Enabled     *bool   `json:"enabled"`
	Placement   string  `json:"placement"`
	Background  string  `json:"background"`
	Dismissible *bool   `json:"dismissible"`
	Position    int     `json:"position"`
	ExpiresAt   *string `json:"expiresAt"`
}

type announcementData struct {
	Title       string `json:"title"`
	Content     string `json:"content"`
	Placement   string `json:"placement"`
	Background  string `json:"background"`
	Dismissible bool   `json:"dismissible"`
}

func parseExpiry(v *string) (*time.Time, error) {
	if v == nil || strings.TrimSpace(*v) == "" {
		return nil, nil
	}
	s := strings.TrimSpace(*v)
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			t = t.UTC().Truncate(time.Second)
			return &t, nil
		}
	}
	return nil, refuse(http.StatusBadRequest, "errors.invalidDate", "expiresAt must be an RFC 3339 date")
}

func (in AnnouncementInput) validate() (announcementData, *time.Time, error) {
	d := announcementData{Title: strings.TrimSpace(in.Title), Content: strings.TrimSpace(in.Content),
		Placement: in.Placement, Background: strings.TrimSpace(in.Background), Dismissible: in.Dismissible == nil || *in.Dismissible}
	if isBlankHTML(d.Content) {
		return d, nil, refuse(http.StatusBadRequest, "errors.announcementEmpty", "The announcement has no content")
	}
	if d.Title == "" {
		d.Title = truncateRunes(stripTags(d.Content), 80)
	}
	if d.Placement != PlacementAll {
		d.Placement = PlacementHome
	}
	if d.Background != "" {
		if d.Background = normColor(d.Background); d.Background == "" {
			return d, nil, refuse(http.StatusBadRequest, "errors.invalidColor", "background must be a CSS colour (#rrggbb)")
		}
	}
	exp, err := parseExpiry(in.ExpiresAt)
	return d, exp, err
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

func stripTags(html string) string {
	s := tagRe.ReplaceAllString(html, " ")
	s = strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&#39;", "'", "&quot;", `"`).Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

func isBlankHTML(html string) bool {
	return stripTags(html) == "" && !strings.Contains(strings.ToLower(html), "<img")
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

const announcementCols = `id, enabled, position, expires_at, data, created_at, updated_at`

func (s *Store) scanAnnouncements(ctx context.Context, where string, args ...any) ([]ShopAnnouncement, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+announcementCols+` FROM `+s.T("cwmerch_announcements")+` WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	now := time.Now().UTC()
	out := []ShopAnnouncement{}
	for rows.Next() {
		var a ShopAnnouncement
		var enabled int
		var exp sql.NullTime
		var raw []byte
		if err := rows.Scan(&a.ID, &enabled, &a.Position, &exp, &raw, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		var d announcementData
		_ = json.Unmarshal(raw, &d)
		a.Title, a.Content, a.Placement, a.Background, a.Dismissible = d.Title, d.Content, d.Placement, d.Background, d.Dismissible
		if a.Placement == "" {
			a.Placement = PlacementHome
		}
		a.Enabled = enabled != 0
		if exp.Valid {
			t := exp.Time.UTC()
			a.ExpiresAt = &t
			a.Expired = !t.After(now)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Announcements lists them for the back office (search in the title/content;
// status "active", "expired", "disabled" or "" for all).
func (s *Store) Announcements(ctx context.Context, search, status string, page, perPage int) (Paged[ShopAnnouncement], error) {
	page, perPage, offset := Page(page, perPage)
	where, args := "1 = 1", []any{}
	now := time.Now().UTC()
	switch status {
	case "active":
		where += " AND enabled = 1 AND (expires_at IS NULL OR expires_at > ?)"
		args = append(args, now)
	case "expired":
		where += " AND expires_at IS NOT NULL AND expires_at <= ?"
		args = append(args, now)
	case "disabled":
		where += " AND enabled = 0"
	}
	for _, word := range strings.Fields(search) {
		where += " AND LOWER(CAST(data AS CHAR)) LIKE ?"
		args = append(args, "%"+likeEscape(strings.ToLower(word))+"%")
	}
	var total int64
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.T("cwmerch_announcements")+` WHERE `+where, args...).Scan(&total); err != nil {
		return Paged[ShopAnnouncement]{}, err
	}
	items, err := s.scanAnnouncements(ctx, where+` ORDER BY position ASC, id DESC LIMIT ? OFFSET ?`, append(args, perPage, offset)...)
	if err != nil {
		return Paged[ShopAnnouncement]{}, err
	}
	return NewPaged(items, total, page, perPage), nil
}

// ActiveAnnouncements is what the storefront shows: enabled, not expired.
func (s *Store) ActiveAnnouncements(ctx context.Context) ([]ShopAnnouncement, error) {
	return s.scanAnnouncements(ctx, `enabled = 1 AND (expires_at IS NULL OR expires_at > ?) ORDER BY position ASC, id DESC LIMIT 20`, time.Now().UTC())
}

func (s *Store) Announcement(ctx context.Context, id uint64) (ShopAnnouncement, error) {
	as, err := s.scanAnnouncements(ctx, `id = ?`, id)
	if err != nil {
		return ShopAnnouncement{}, err
	}
	if len(as) == 0 {
		return ShopAnnouncement{}, ErrNotFound
	}
	return as[0], nil
}

// SaveAnnouncement creates (id 0) or updates an announcement.
func (s *Store) SaveAnnouncement(ctx context.Context, id uint64, in AnnouncementInput, by uint64) (ShopAnnouncement, error) {
	d, exp, err := in.validate()
	if err != nil {
		return ShopAnnouncement{}, err
	}
	raw, _ := json.Marshal(d)
	enabled := 1
	if in.Enabled != nil && !*in.Enabled {
		enabled = 0
	}
	var expArg any
	if exp != nil {
		expArg = *exp
	}
	if id == 0 {
		res, err := s.DB.ExecContext(ctx, `INSERT INTO `+s.T("cwmerch_announcements")+
			` (enabled, position, expires_at, data, created_by, updated_by) VALUES (?, ?, ?, ?, ?, ?)`,
			enabled, in.Position, expArg, string(raw), nullIfZero(by), nullIfZero(by))
		if err != nil {
			return ShopAnnouncement{}, err
		}
		n, _ := res.LastInsertId()
		return s.Announcement(ctx, uint64(n))
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE `+s.T("cwmerch_announcements")+
		` SET enabled = ?, position = ?, expires_at = ?, data = ?, updated_by = ? WHERE id = ?`,
		enabled, in.Position, expArg, string(raw), nullIfZero(by), id)
	if err != nil {
		return ShopAnnouncement{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, err := s.Announcement(ctx, id); err != nil {
			return ShopAnnouncement{}, err
		}
	}
	return s.Announcement(ctx, id)
}

func (s *Store) DeleteAnnouncement(ctx context.Context, id uint64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM `+s.T("cwmerch_announcements")+` WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// IsMissingTable tells whether err is a missing table (migration not applied yet).
func IsMissingTable(err error) bool {
	if err == nil {
		return false
	}
	var we *Error
	if errors.As(err, &we) {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "doesn't exist") || strings.Contains(msg, "does not exist") || strings.Contains(msg, "1146")
}
