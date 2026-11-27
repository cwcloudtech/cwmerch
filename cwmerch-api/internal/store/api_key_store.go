package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"cwmerch-api/internal/db"
	"cwmerch-api/internal/models"
	"cwmerch-api/internal/utils"
)

// ApiKeyStore holds the long-term API keys (X-Api-Key). Only the SHA-256 of a
// token is kept; the plaintext is returned once, at creation.
type ApiKeyStore struct {
	db *db.DB
}

func NewApiKeyStore(d *db.DB) *ApiKeyStore { return &ApiKeyStore{db: d} }

func generateToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return utils.EMPTY, err
	}
	return "cwmerch_" + hex.EncodeToString(buf), nil
}

func (s *ApiKeyStore) get(ctx context.Context, id string) (models.ApiKey, error) {
	var k models.ApiKey
	var uid uint64
	err := s.db.QueryRowContext(ctx, `SELECT id, user_id, description, expires_at, created_at, updated_at FROM `+
		s.db.T("cwmerch_api_keys")+` WHERE id = ?`, id).Scan(&k.ID, &uid, &k.Description, &k.ExpiresAt, &k.CreatedAt, &k.UpdatedAt)
	k.UserID = idString(uid)
	return k, notFound(err)
}

// Create mints a key for userID; the plaintext token is only returned here.
func (s *ApiKeyStore) Create(ctx context.Context, userID, description string, expiresAt *time.Time) (models.ApiKey, string, error) {
	token, err := generateToken()
	if err != nil {
		return models.ApiKey{}, utils.EMPTY, err
	}
	id := newID()
	var exp any
	if expiresAt != nil {
		exp = expiresAt.UTC()
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO `+s.db.T("cwmerch_api_keys")+
		` (id, user_id, key_hash, description, expires_at) VALUES (?, ?, ?, ?, ?)`,
		id, userIDArg(userID), utils.HashToken(token), description, exp); err != nil {
		return models.ApiKey{}, utils.EMPTY, err
	}
	k, err := s.get(ctx, id)
	return k, token, err
}

func (s *ApiKeyStore) ListByUser(ctx context.Context, userID string) ([]models.ApiKey, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_id, description, expires_at, created_at, updated_at FROM `+
		s.db.T("cwmerch_api_keys")+` WHERE user_id = ? ORDER BY created_at DESC`, userIDArg(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := []models.ApiKey{}
	for rows.Next() {
		var k models.ApiKey
		var uid uint64
		if err := rows.Scan(&k.ID, &uid, &k.Description, &k.ExpiresAt, &k.CreatedAt, &k.UpdatedAt); err != nil {
			return nil, err
		}
		k.UserID = idString(uid)
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (s *ApiKeyStore) Delete(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM `+s.db.T("cwmerch_api_keys")+` WHERE id = ? AND user_id = ?`, id, userIDArg(userID))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// VerifyHash answers the user a still-valid key belongs to.
func (s *ApiKeyStore) VerifyHash(ctx context.Context, hash string) (string, error) {
	var uid uint64
	err := s.db.QueryRowContext(ctx, `SELECT user_id FROM `+s.db.T("cwmerch_api_keys")+
		` WHERE key_hash = ? AND (expires_at IS NULL OR expires_at > UTC_TIMESTAMP())`, hash).Scan(&uid)
	if err != nil {
		return utils.EMPTY, notFound(err)
	}
	return idString(uid), nil
}
