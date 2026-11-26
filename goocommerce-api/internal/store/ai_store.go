package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"goocommerce-api/internal/db"
	"goocommerce-api/internal/models"
)

// AIStore holds the assistant's provider catalogue, the users' subscriptions
// (connections) and who they are shared with, and the conversations.
type AIStore struct {
	db *db.DB
}

func NewAIStore(d *db.DB) *AIStore { return &AIStore{db: d} }

type aiProviderData struct {
	Label   string   `json:"label,omitempty"`
	KeyHint string   `json:"keyHint,omitempty"`
	Models  []string `json:"models,omitempty"`
}

type rowScanner interface{ Scan(dest ...any) error }

func (s *AIStore) ListProviders(ctx context.Context) ([]models.AIProvider, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name, format, base_url, default_model, data FROM `+
		s.db.T("goo_ai_providers")+` ORDER BY position, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	providers := []models.AIProvider{}
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		providers = append(providers, p)
	}
	return providers, rows.Err()
}

func (s *AIStore) FindProvider(ctx context.Context, name string) (models.AIProvider, error) {
	return scanProvider(s.db.QueryRowContext(ctx, `SELECT name, format, base_url, default_model, data FROM `+
		s.db.T("goo_ai_providers")+` WHERE name = ?`, name))
}

func scanProvider(row rowScanner) (models.AIProvider, error) {
	var p models.AIProvider
	var raw string
	if err := row.Scan(&p.Name, &p.Format, &p.BaseURL, &p.DefaultModel, &raw); err != nil {
		return models.AIProvider{}, notFound(err)
	}
	var data aiProviderData
	_ = json.Unmarshal([]byte(raw), &data)
	p.Label, p.KeyHint, p.Models = data.Label, data.KeyHint, data.Models
	if p.Label == "" {
		p.Label = p.Name
	}
	if p.Models == nil {
		p.Models = []string{}
	}
	return p, nil
}

type aiConnectionData struct {
	Name      string `json:"name,omitempty"`
	Provider  string `json:"provider"`
	BaseURL   string `json:"baseUrl"`
	Model     string `json:"model,omitempty"`
	MaxTokens int    `json:"maxTokens,omitempty"`
	Token     string `json:"token,omitempty"`
}

// connectionRow is a connection row joined with the caller's share (role) and
// the provider catalogue (format).
type connectionRow struct {
	ID, Raw   string
	OwnerID   uint64
	Created   time.Time
	Updated   time.Time
	ShareRole sql.NullString
	Format    sql.NullString
}

func (r connectionRow) model(userID uint64) models.AIConnection {
	var data aiConnectionData
	_ = json.Unmarshal([]byte(r.Raw), &data)
	c := models.AIConnection{
		ID: r.ID, OwnerID: idString(r.OwnerID), Name: data.Name, Provider: data.Provider,
		BaseURL: data.BaseURL, Model: data.Model, MaxTokens: data.MaxTokens, Token: data.Token,
		HasToken: strings.TrimSpace(data.Token) != "", CreatedAt: r.Created, UpdatedAt: r.Updated,
		Format: r.Format.String,
	}
	if userID != 0 {
		if r.OwnerID == userID {
			c.Role = models.AIRoleOwner
		} else {
			c.Role = r.ShareRole.String
		}
	}
	return c
}

func (s *AIStore) connections(ctx context.Context, userID uint64, where string, args ...any) ([]connectionRow, error) {
	q := `SELECT c.id, c.owner_id, c.data, c.created_at, c.updated_at, a.role, p.format
		FROM ` + s.db.T("goo_ai_connections") + ` c
		LEFT JOIN ` + s.db.T("goo_ai_connection_shares") + ` a ON a.connection_id = c.id AND a.user_id = ?
		LEFT JOIN ` + s.db.T("goo_ai_providers") + ` p ON p.name = JSON_UNQUOTE(JSON_EXTRACT(c.data, '$.provider'))
		WHERE ` + where + ` ORDER BY (c.owner_id = ?) DESC, c.position, c.created_at`
	all := append([]any{userID}, args...)
	all = append(all, userID)
	rows, err := s.db.QueryContext(ctx, q, all...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []connectionRow{}
	for rows.Next() {
		var r connectionRow
		if err := rows.Scan(&r.ID, &r.OwnerID, &r.Raw, &r.Created, &r.Updated, &r.ShareRole, &r.Format); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *AIStore) FindByID(ctx context.Context, id string) (models.AIConnection, error) {
	rows, err := s.connections(ctx, 0, "c.id = ?", id)
	if err != nil {
		return models.AIConnection{}, err
	}
	if len(rows) == 0 {
		return models.AIConnection{}, ErrNotFound
	}
	return rows[0].model(0), nil
}

// ListFor is every connection userID owns or was given, own first; tokens blanked.
func (s *AIStore) ListFor(ctx context.Context, userID string) ([]models.AIConnection, error) {
	uid := userIDArg(userID)
	rows, err := s.connections(ctx, uid, "(c.owner_id = ? OR a.user_id = ?)", uid, uid)
	if err != nil {
		return nil, err
	}
	out := []models.AIConnection{}
	for _, r := range rows {
		c := r.model(uid)
		c.Token = ""
		out = append(out, c)
	}
	return out, nil
}

func (s *AIStore) RoleFor(ctx context.Context, connectionID, userID string) (string, error) {
	uid := userIDArg(userID)
	rows, err := s.connections(ctx, uid, "c.id = ?", connectionID)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", ErrNotFound
	}
	return rows[0].model(uid).Role, nil
}

// Usable is the connection a question is spent on by default: the caller's
// first own one, else the first shared with them.
func (s *AIStore) Usable(ctx context.Context, userID string) (models.AIConnection, error) {
	uid := userIDArg(userID)
	rows, err := s.connections(ctx, uid, "(c.owner_id = ? OR a.user_id = ?)", uid, uid)
	if err != nil {
		return models.AIConnection{}, err
	}
	if len(rows) == 0 {
		return models.AIConnection{}, ErrNotFound
	}
	return rows[0].model(uid), nil
}

func (s *AIStore) UsableByID(ctx context.Context, userID, connectionID string) (models.AIConnection, error) {
	uid := userIDArg(userID)
	rows, err := s.connections(ctx, uid, "c.id = ? AND (c.owner_id = ? OR a.user_id = ?)", connectionID, uid, uid)
	if err != nil {
		return models.AIConnection{}, err
	}
	if len(rows) == 0 {
		return models.AIConnection{}, ErrNotFound
	}
	return rows[0].model(uid), nil
}

func (s *AIStore) Create(ctx context.Context, ownerID string, c models.AIConnection) (models.AIConnection, error) {
	data, err := json.Marshal(aiConnectionData{Name: c.Name, Provider: c.Provider, BaseURL: c.BaseURL,
		Model: c.Model, MaxTokens: c.MaxTokens, Token: c.Token})
	if err != nil {
		return models.AIConnection{}, err
	}
	uid := userIDArg(ownerID)
	var pos int
	_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(position) + 1, 0) FROM `+s.db.T("goo_ai_connections")+
		` WHERE owner_id = ?`, uid).Scan(&pos)
	id := newID()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO `+s.db.T("goo_ai_connections")+
		` (id, owner_id, data, position) VALUES (?, ?, ?, ?)`, id, uid, string(data), pos); err != nil {
		return models.AIConnection{}, err
	}
	return s.FindByID(ctx, id)
}

// Update replaces the settings; a blank Token keeps the stored key.
func (s *AIStore) Update(ctx context.Context, id string, c models.AIConnection) (models.AIConnection, error) {
	current, err := s.FindByID(ctx, id)
	if err != nil {
		return models.AIConnection{}, err
	}
	token := strings.TrimSpace(c.Token)
	if token == "" {
		token = current.Token
	}
	data, err := json.Marshal(aiConnectionData{Name: c.Name, Provider: c.Provider, BaseURL: c.BaseURL,
		Model: c.Model, MaxTokens: c.MaxTokens, Token: token})
	if err != nil {
		return models.AIConnection{}, err
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE `+s.db.T("goo_ai_connections")+` SET data = ? WHERE id = ?`,
		string(data), id); err != nil {
		return models.AIConnection{}, err
	}
	return s.FindByID(ctx, id)
}

func (s *AIStore) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM `+s.db.T("goo_ai_connections")+` WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Reorder puts ids first, in that order, and keeps the rest after them.
func (s *AIStore) Reorder(ctx context.Context, ownerID string, ids []string) error {
	uid := userIDArg(ownerID)
	return s.db.InTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT id FROM `+s.db.T("goo_ai_connections")+
			` WHERE owner_id = ? ORDER BY position, created_at`, uid)
		if err != nil {
			return err
		}
		existing := []string{}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			existing = append(existing, id)
		}
		rows.Close()
		seen := map[string]bool{}
		order := []string{}
		owned := map[string]bool{}
		for _, id := range existing {
			owned[id] = true
		}
		for _, id := range ids {
			if owned[id] && !seen[id] {
				order = append(order, id)
				seen[id] = true
			}
		}
		for _, id := range existing {
			if !seen[id] {
				order = append(order, id)
			}
		}
		for pos, id := range order {
			if _, err := tx.ExecContext(ctx, `UPDATE `+s.db.T("goo_ai_connections")+` SET position = ? WHERE id = ?`, pos, id); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *AIStore) Grant(ctx context.Context, connectionID, userID, role string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO `+s.db.T("goo_ai_connection_shares")+
		` (id, connection_id, user_id, role) VALUES (?, ?, ?, ?) ON DUPLICATE KEY UPDATE role = VALUES(role)`,
		newID(), connectionID, userIDArg(userID), role)
	return err
}

func (s *AIStore) Revoke(ctx context.Context, connectionID, userID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM `+s.db.T("goo_ai_connection_shares")+
		` WHERE connection_id = ? AND user_id = ?`, connectionID, userIDArg(userID))
	return err
}

func (s *AIStore) ListGrants(ctx context.Context, connectionID string) ([]models.AIConnectionACL, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT a.user_id, u.user_email,
		COALESCE((SELECT meta_value FROM `+s.db.T("usermeta")+` m WHERE m.user_id = u.ID AND m.meta_key = 'first_name' LIMIT 1), ''),
		COALESCE((SELECT meta_value FROM `+s.db.T("usermeta")+` m WHERE m.user_id = u.ID AND m.meta_key = 'last_name' LIMIT 1), ''),
		a.role, a.created_at
		FROM `+s.db.T("goo_ai_connection_shares")+` a JOIN `+s.db.T("users")+` u ON u.ID = a.user_id
		WHERE a.connection_id = ? ORDER BY a.created_at`, connectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	grants := []models.AIConnectionACL{}
	for rows.Next() {
		var g models.AIConnectionACL
		var uid uint64
		if err := rows.Scan(&uid, &g.Email, &g.Name, &g.Surname, &g.Role, &g.CreatedAt); err != nil {
			return nil, err
		}
		g.UserID = idString(uid)
		grants = append(grants, g)
	}
	return grants, rows.Err()
}

type aiConversationData struct {
	Title string `json:"title,omitempty"`
}

type aiPromptData struct {
	Provider   string         `json:"provider,omitempty"`
	Model      string         `json:"model,omitempty"`
	Connection string         `json:"connection,omitempty"`
	Usage      map[string]any `json:"usage,omitempty"`
}

func (s *AIStore) ListConversations(ctx context.Context, userID string) ([]models.AIConversation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT c.id, c.data, c.created_at, c.updated_at,
		(SELECT COUNT(*) FROM `+s.db.T("goo_ai_prompts")+` p WHERE p.conversation_id = c.id)
		FROM `+s.db.T("goo_ai_conversations")+` c WHERE c.user_id = ? ORDER BY c.updated_at DESC`, userIDArg(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.AIConversation{}
	for rows.Next() {
		var c models.AIConversation
		var raw string
		if err := rows.Scan(&c.ID, &raw, &c.CreatedAt, &c.UpdatedAt, &c.Count); err != nil {
			return nil, err
		}
		var data aiConversationData
		_ = json.Unmarshal([]byte(raw), &data)
		c.Title = data.Title
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *AIStore) FindConversation(ctx context.Context, id, userID string) (models.AIConversation, error) {
	var c models.AIConversation
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT id, data, created_at, updated_at FROM `+s.db.T("goo_ai_conversations")+
		` WHERE id = ? AND user_id = ?`, id, userIDArg(userID)).Scan(&c.ID, &raw, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return models.AIConversation{}, notFound(err)
	}
	var data aiConversationData
	_ = json.Unmarshal([]byte(raw), &data)
	c.Title = data.Title
	rows, err := s.db.QueryContext(ctx, `SELECT id, question, answer, data, created_at FROM `+s.db.T("goo_ai_prompts")+
		` WHERE conversation_id = ? ORDER BY created_at`, c.ID)
	if err != nil {
		return models.AIConversation{}, err
	}
	defer rows.Close()
	c.Exchanges = []models.AIPrompt{}
	for rows.Next() {
		var e models.AIPrompt
		var meta string
		if err := rows.Scan(&e.ID, &e.Question, &e.Answer, &meta, &e.CreatedAt); err != nil {
			return models.AIConversation{}, err
		}
		var m aiPromptData
		_ = json.Unmarshal([]byte(meta), &m)
		e.Provider, e.Model, e.Connection, e.Usage = m.Provider, m.Model, m.Connection, m.Usage
		c.Exchanges = append(c.Exchanges, e)
	}
	c.Count = len(c.Exchanges)
	return c, rows.Err()
}

func (s *AIStore) CreateConversation(ctx context.Context, userID, title string) (string, error) {
	data, _ := json.Marshal(aiConversationData{Title: title})
	id := newID()
	_, err := s.db.ExecContext(ctx, `INSERT INTO `+s.db.T("goo_ai_conversations")+` (id, user_id, data) VALUES (?, ?, ?)`,
		id, userIDArg(userID), string(data))
	return id, err
}

func (s *AIStore) AppendExchange(ctx context.Context, conversationID string, e models.AIPrompt) error {
	data, _ := json.Marshal(aiPromptData{Provider: e.Provider, Model: e.Model, Connection: e.Connection, Usage: e.Usage})
	return s.db.InTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO `+s.db.T("goo_ai_prompts")+
			` (id, conversation_id, question, answer, data) VALUES (?, ?, ?, ?, ?)`,
			newID(), conversationID, e.Question, e.Answer, string(data)); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE `+s.db.T("goo_ai_conversations")+` SET updated_at = CURRENT_TIMESTAMP(6) WHERE id = ?`, conversationID)
		return err
	})
}

func (s *AIStore) DeleteConversations(ctx context.Context, userID string, ids []string, all bool) (int64, error) {
	uid := userIDArg(userID)
	if all {
		res, err := s.db.ExecContext(ctx, `DELETE FROM `+s.db.T("goo_ai_conversations")+` WHERE user_id = ?`, uid)
		if err != nil {
			return 0, err
		}
		return res.RowsAffected()
	}
	if len(ids) == 0 {
		return 0, nil
	}
	args := []any{uid}
	for _, id := range ids {
		args = append(args, id)
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM `+s.db.T("goo_ai_conversations")+` WHERE user_id = ? AND id IN (`+
		placeholders(len(ids))+`)`, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func placeholders(n int) string {
	if n <= 0 {
		return "NULL"
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}
