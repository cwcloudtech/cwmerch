package store

import (
	"context"
	"encoding/json"

	"goocommerce-api/internal/db"
	"goocommerce-api/internal/models"
)

// WebAuthnCredentialStore keeps the security keys (YubiKey...) enrolled as a
// second factor.
type WebAuthnCredentialStore struct {
	db *db.DB
}

func NewWebAuthnCredentialStore(d *db.DB) *WebAuthnCredentialStore {
	return &WebAuthnCredentialStore{db: d}
}

func (s *WebAuthnCredentialStore) Create(ctx context.Context, userID string, credentialID, publicKey []byte, transports []string, name string) (models.WebAuthnCredential, error) {
	if transports == nil {
		transports = []string{}
	}
	raw, _ := json.Marshal(transports)
	id := newID()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO `+s.db.T("goo_webauthn_credentials")+
		` (id, user_id, credential_id, public_key, transports, name) VALUES (?, ?, ?, ?, ?, ?)`,
		id, userIDArg(userID), credentialID, publicKey, string(raw), name); err != nil {
		return models.WebAuthnCredential{}, err
	}
	creds, err := s.ListByUser(ctx, userID)
	if err != nil {
		return models.WebAuthnCredential{}, err
	}
	for _, c := range creds {
		if c.ID == id {
			return c, nil
		}
	}
	return models.WebAuthnCredential{}, ErrNotFound
}

func (s *WebAuthnCredentialStore) ListByUser(ctx context.Context, userID string) ([]models.WebAuthnCredential, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_id, credential_id, public_key, sign_count, transports, name, created_at, updated_at FROM `+
		s.db.T("goo_webauthn_credentials")+` WHERE user_id = ? ORDER BY created_at`, userIDArg(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.WebAuthnCredential{}
	for rows.Next() {
		var c models.WebAuthnCredential
		var uid uint64
		var count uint64
		var transports string
		if err := rows.Scan(&c.ID, &uid, &c.CredentialID, &c.PublicKey, &count, &transports, &c.Name, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.UserID = idString(uid)
		c.SignCount = uint32(count)
		_ = json.Unmarshal([]byte(transports), &c.Transports)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *WebAuthnCredentialStore) CountByUser(ctx context.Context, userID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.db.T("goo_webauthn_credentials")+` WHERE user_id = ?`, userIDArg(userID)).Scan(&n)
	return n, err
}

func (s *WebAuthnCredentialStore) UpdateSignCount(ctx context.Context, credentialID []byte, signCount uint32) error {
	_, err := s.db.ExecContext(ctx, `UPDATE `+s.db.T("goo_webauthn_credentials")+` SET sign_count = ? WHERE credential_id = ?`, signCount, credentialID)
	return err
}

func (s *WebAuthnCredentialStore) Delete(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM `+s.db.T("goo_webauthn_credentials")+` WHERE id = ? AND user_id = ?`, id, userIDArg(userID))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *WebAuthnCredentialStore) DeleteAllForUser(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM `+s.db.T("goo_webauthn_credentials")+` WHERE user_id = ?`, userIDArg(userID))
	return err
}
