package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"goocommerce-api/internal/db"
	"goocommerce-api/internal/models"
)

// SessionStore keeps the addresses connections came from (one row per
// address and user, user 0 = anonymous, counted once a day) and the banned
// addresses. It backs the IP administration screens and the ban guard.
type SessionStore struct {
	db *db.DB
}

func NewSessionStore(d *db.DB) *SessionStore { return &SessionStore{db: d} }

type sessionData struct {
	Count int               `json:"count"`
	Geo   map[string]string `json:"geo,omitempty"`
}

// RecordVisit counts a visit from ip (by userID, nil = anonymous) at most once
// a UTC day, and reports whether this call counted it.
func (s *SessionStore) RecordVisit(ctx context.Context, userID *string, ip string, at time.Time) (bool, error) {
	uid := uint64(0)
	if userID != nil {
		uid = userIDArg(*userID)
	}
	day := at.UTC().Truncate(24 * time.Hour)
	var id, raw string
	var updated time.Time
	err := s.db.QueryRowContext(ctx, `SELECT id, data, updated_at FROM `+s.db.T("goo_sessions")+
		` WHERE ip = ? AND user_id = ?`, ip, uid).Scan(&id, &raw, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		data, _ := json.Marshal(sessionData{Count: 1})
		_, err = s.db.ExecContext(ctx, `INSERT INTO `+s.db.T("goo_sessions")+
			` (id, user_id, ip, data, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			newID(), uid, ip, string(data), at.UTC(), at.UTC())
		if db.IsDuplicate(err) {
			return false, nil
		}
		return err == nil, err
	}
	if err != nil {
		return false, err
	}
	if !updated.Before(day) {
		return false, nil
	}
	var d sessionData
	_ = json.Unmarshal([]byte(raw), &d)
	d.Count++
	data, _ := json.Marshal(d)
	_, err = s.db.ExecContext(ctx, `UPDATE `+s.db.T("goo_sessions")+` SET data = ?, updated_at = ? WHERE id = ?`,
		string(data), at.UTC(), id)
	return err == nil, err
}

// NeedsGeo reports whether no row for ip carries a geo lookup yet.
func (s *SessionStore) NeedsGeo(ctx context.Context, ip string) (bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT data FROM `+s.db.T("goo_sessions")+` WHERE ip = ?`, ip)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return false, err
		}
		var d sessionData
		if json.Unmarshal([]byte(raw), &d) == nil && len(d.Geo) > 0 {
			return false, nil
		}
	}
	return true, rows.Err()
}

// SetGeo stores the geo lookup on every row of ip.
func (s *SessionStore) SetGeo(ctx context.Context, ip string, info map[string]string) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id, data FROM `+s.db.T("goo_sessions")+` WHERE ip = ?`, ip)
	if err != nil {
		return err
	}
	type row struct{ id, raw string }
	var all []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.raw); err != nil {
			rows.Close()
			return err
		}
		all = append(all, r)
	}
	rows.Close()
	for _, r := range all {
		var d sessionData
		_ = json.Unmarshal([]byte(r.raw), &d)
		d.Geo = info
		data, _ := json.Marshal(d)
		if _, err := s.db.ExecContext(ctx, `UPDATE `+s.db.T("goo_sessions")+` SET data = ? WHERE id = ?`, string(data), r.id); err != nil {
			return err
		}
	}
	return nil
}

func (s *SessionStore) IsIpBanned(ctx context.Context, ip string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.db.T("goo_ip_bans")+` WHERE ip = ?`, ip).Scan(&n)
	return n > 0, err
}

func (s *SessionStore) BanIp(ctx context.Context, ip, reason, bannedBy string) error {
	data, _ := json.Marshal(map[string]string{"reason": reason, "bannedBy": bannedBy})
	_, err := s.db.ExecContext(ctx, `INSERT INTO `+s.db.T("goo_ip_bans")+` (id, ip, data) VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE data = VALUES(data)`, newID(), ip, string(data))
	return err
}

func (s *SessionStore) UnbanIp(ctx context.Context, ip string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM `+s.db.T("goo_ip_bans")+` WHERE ip = ?`, ip)
	return err
}

func (s *SessionStore) bannedSet(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT ip, data FROM `+s.db.T("goo_ip_bans"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var ip, raw string
		if err := rows.Scan(&ip, &raw); err != nil {
			return nil, err
		}
		var d map[string]string
		_ = json.Unmarshal([]byte(raw), &d)
		out[ip] = d["reason"]
	}
	return out, rows.Err()
}

// ListByUser is every address one member connected from.
func (s *SessionStore) ListByUser(ctx context.Context, userID string) ([]models.ConnectionIP, error) {
	banned, err := s.bannedSet(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT ip, data, created_at, updated_at FROM `+s.db.T("goo_sessions")+
		` WHERE user_id = ? ORDER BY updated_at DESC`, userIDArg(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ips := []models.ConnectionIP{}
	for rows.Next() {
		var ip models.ConnectionIP
		var raw string
		if err := rows.Scan(&ip.IP, &raw, &ip.FirstSeen, &ip.LastSeen); err != nil {
			return nil, err
		}
		var d sessionData
		_ = json.Unmarshal([]byte(raw), &d)
		ip.Count = d.Count
		if len(d.Geo) > 0 {
			ip.Geo = d.Geo
		}
		_, ip.Banned = banned[ip.IP]
		ips = append(ips, ip)
	}
	return ips, rows.Err()
}

// ListIps is the administration roster (every address seen or banned), most
// recently seen first, filtered by term on the address or a member's email/name.
func (s *SessionStore) ListIps(ctx context.Context, term string, offset, limit int) ([]models.IpSummary, error) {
	banned, err := s.bannedSet(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT s.ip, s.user_id, s.data, s.created_at, s.updated_at,
		COALESCE(u.user_email, ''), COALESCE(u.display_name, ''), COALESCE(u.user_login, '')
		FROM `+s.db.T("goo_sessions")+` s LEFT JOIN `+s.db.T("users")+` u ON u.ID = s.user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byIP := map[string]*models.IpSummary{}
	order := []string{}
	get := func(ip string) *models.IpSummary {
		if sm, ok := byIP[ip]; ok {
			return sm
		}
		sm := &models.IpSummary{IP: ip, Members: []models.IpMember{}}
		byIP[ip] = sm
		order = append(order, ip)
		return sm
	}
	matches := map[string]bool{}
	term = strings.ToLower(strings.TrimSpace(term))
	for rows.Next() {
		var ip, raw, email, display, login string
		var uid uint64
		var created, updated time.Time
		if err := rows.Scan(&ip, &uid, &raw, &created, &updated, &email, &display, &login); err != nil {
			return nil, err
		}
		sm := get(ip)
		var d sessionData
		_ = json.Unmarshal([]byte(raw), &d)
		sm.Count += d.Count
		if sm.FirstSeen == nil || created.Before(*sm.FirstSeen) {
			c := created
			sm.FirstSeen = &c
		}
		if sm.LastSeen == nil || updated.After(*sm.LastSeen) {
			u := updated
			sm.LastSeen = &u
		}
		if len(d.Geo) > 0 && sm.Geo == nil {
			sm.Geo = d.Geo
		}
		if uid != 0 {
			sm.Members = append(sm.Members, models.IpMember{ID: idString(uid), Email: email, Name: display, Handle: login})
		}
		if term == "" || strings.Contains(strings.ToLower(ip), term) || strings.Contains(strings.ToLower(email), term) ||
			strings.Contains(strings.ToLower(display), term) || strings.Contains(strings.ToLower(login), term) {
			matches[ip] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for ip, reason := range banned {
		sm := get(ip)
		sm.Banned, sm.Reason = true, reason
		if term == "" || strings.Contains(strings.ToLower(ip), term) {
			matches[ip] = true
		}
	}
	out := []models.IpSummary{}
	for _, ip := range order {
		if matches[ip] {
			out = append(out, *byIP[ip])
		}
	}
	sortIPSummaries(out)
	if offset >= len(out) {
		return []models.IpSummary{}, nil
	}
	end := offset + limit
	if limit <= 0 || end > len(out) {
		end = len(out)
	}
	return out[offset:end], nil
}

func sortIPSummaries(out []models.IpSummary) {
	less := func(a, b models.IpSummary) bool {
		switch {
		case a.LastSeen == nil && b.LastSeen == nil:
			return a.IP < b.IP
		case a.LastSeen == nil:
			return false
		case b.LastSeen == nil:
			return true
		}
		return a.LastSeen.After(*b.LastSeen)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && less(out[j], out[j-1]); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
}

func (s *SessionStore) Stats(ctx context.Context) (models.IpStats, error) {
	var out models.IpStats
	err := s.db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM (SELECT ip FROM `+s.db.T("goo_sessions")+` UNION SELECT ip FROM `+s.db.T("goo_ip_bans")+`) t),
		(SELECT COUNT(*) FROM `+s.db.T("goo_ip_bans")+`)`).Scan(&out.Total, &out.Banned)
	return out, err
}
