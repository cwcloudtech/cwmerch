package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"cwmerch-api/internal/db"
	"cwmerch-api/internal/models"
	"cwmerch-api/internal/php"
	"cwmerch-api/internal/utils"
)

// UserStore reads and writes WordPress users (`<prefix>users` +
// `<prefix>usermeta`), so an account is the same on WordPress and cwmerch,
// plus cwmerch's own extras in `<prefix>cwmerch_users`.
type UserStore struct {
	db *db.DB
}

func NewUserStore(d *db.DB) *UserStore { return &UserStore{db: d} }

// cwmerchUserData is cwmerch's extras on a WordPress user.
type cwmerchUserData struct {
	Status        string   `json:"status,omitempty"` // "", disabled, ban
	MFAEnabled    bool     `json:"mfaEnabled,omitempty"`
	MFATOTPSecret string   `json:"mfaTotpSecret,omitempty"`
	Picture       string   `json:"picture,omitempty"`
	PictureX      *float64 `json:"pictureX,omitempty"`
	PictureY      *float64 `json:"pictureY,omitempty"`
}

const defaultImagePosition = 50.0

func pos(v *float64) float64 {
	if v == nil {
		return defaultImagePosition
	}
	return *v
}

// WordPress roles cwmerch maps to its own.
const (
	WPRoleAdministrator = "administrator"
	WPRoleShopManager   = "shop_manager"
	WPRoleCustomer      = "customer"
)

func (s *UserStore) capsKey() string  { return s.db.Raw("capabilities") }
func (s *UserStore) levelKey() string { return s.db.Raw("user_level") }

// wpRoles decodes the capabilities meta (a:1:{s:8:"customer";b:1;}).
func wpRoles(serialized string) []string {
	roles := []string{}
	caps := php.UnmarshalArray(serialized)
	for _, k := range caps.Keys {
		if v := caps.Get(k); v == true || php.ToString(v) == "1" {
			roles = append(roles, php.ToString(k))
		}
	}
	return roles
}

func roleFrom(roles []string, status string) models.GlobalRole {
	switch status {
	case string(models.GlobalRoleBan):
		return models.GlobalRoleBan
	case string(models.GlobalRoleDisabled):
		return models.GlobalRoleDisabled
	}
	for _, r := range roles {
		if r == WPRoleAdministrator {
			return models.GlobalRoleSuperuser
		}
	}
	for _, r := range roles {
		if r == WPRoleShopManager {
			return models.GlobalRoleManager
		}
	}
	return models.GlobalRoleConfirmed
}

const userColumns = `u.ID, u.user_login, u.user_pass, u.user_email, u.display_name, u.user_registered`

func (s *UserStore) userFrom() string {
	return ` FROM ` + s.db.T("users") + ` u`
}

func (s *UserStore) scanUsers(ctx context.Context, rows *sql.Rows) ([]models.User, error) {
	defer rows.Close()
	users := []models.User{}
	for rows.Next() {
		var u models.User
		var id uint64
		var registered sql.NullTime
		if err := rows.Scan(&id, &u.Login, &u.PasswordHash, &u.Email, &u.DisplayName, &registered); err != nil {
			return nil, err
		}
		u.ID = idString(id)
		if registered.Valid {
			u.CreatedAt = registered.Time
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return users, s.enrich(ctx, users)
}

// enrich fills names, roles and cwmerch extras for users.
func (s *UserStore) enrich(ctx context.Context, users []models.User) error {
	if len(users) == 0 {
		return nil
	}
	ids := make([]any, 0, len(users))
	index := map[string]int{}
	for i, u := range users {
		ids = append(ids, userIDArg(u.ID))
		index[u.ID] = i
	}
	metaArgs := append([]any{s.capsKey()}, ids...)
	rows, err := s.db.QueryContext(ctx, `SELECT user_id, meta_key, meta_value FROM `+s.db.T("usermeta")+
		` WHERE meta_key IN ('first_name', 'last_name', ?) AND user_id IN (`+placeholders(len(ids))+`)`, metaArgs...)
	if err != nil {
		return err
	}
	caps := map[string]string{}
	for rows.Next() {
		var uid uint64
		var key string
		var value sql.NullString
		if err := rows.Scan(&uid, &key, &value); err != nil {
			rows.Close()
			return err
		}
		i := index[idString(uid)]
		switch key {
		case "first_name":
			users[i].Name = value.String
		case "last_name":
			users[i].Surname = value.String
		default:
			caps[idString(uid)] = value.String
		}
	}
	rows.Close()
	extras := map[string]cwmerchUserData{}
	updated := map[string]time.Time{}
	rows, err = s.db.QueryContext(ctx, `SELECT user_id, data, updated_at FROM `+s.db.T("cwmerch_users")+
		` WHERE user_id IN (`+placeholders(len(ids))+`)`, ids...)
	if err != nil {
		return err
	}
	for rows.Next() {
		var uid uint64
		var raw string
		var at time.Time
		if err := rows.Scan(&uid, &raw, &at); err != nil {
			rows.Close()
			return err
		}
		var d cwmerchUserData
		_ = json.Unmarshal([]byte(raw), &d)
		extras[idString(uid)] = d
		updated[idString(uid)] = at
	}
	rows.Close()
	for i := range users {
		u := &users[i]
		d := extras[u.ID]
		u.WPRoles = wpRoles(caps[u.ID])
		u.Role = roleFrom(u.WPRoles, d.Status)
		u.MFAEnabled, u.MFATOTPSecret = d.MFAEnabled, d.MFATOTPSecret
		u.Picture, u.PictureX, u.PictureY = d.Picture, pos(d.PictureX), pos(d.PictureY)
		u.UpdatedAt = u.CreatedAt
		if at, ok := updated[u.ID]; ok {
			u.UpdatedAt = at
		}
	}
	return nil
}

func (s *UserStore) findOne(ctx context.Context, where string, args ...any) (models.User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+userColumns+s.userFrom()+` WHERE `+where+` LIMIT 1`, args...)
	if err != nil {
		return models.User{}, err
	}
	users, err := s.scanUsers(ctx, rows)
	if err != nil {
		return models.User{}, err
	}
	if len(users) == 0 {
		return models.User{}, ErrNotFound
	}
	return users[0], nil
}

// Count is the number of WordPress users.
func (s *UserStore) Count(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.db.T("users")).Scan(&n)
	return n, err
}

// CountAdministrators is the number of WordPress administrators.
func (s *UserStore) CountAdministrators(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.db.T("usermeta")+
		` WHERE meta_key = ? AND meta_value LIKE '%"administrator"%'`, s.capsKey()).Scan(&n)
	return n, err
}

func (s *UserStore) FindByEmail(ctx context.Context, email string) (models.User, error) {
	return s.findOne(ctx, "u.user_email = ?", strings.TrimSpace(email))
}

// FindByLogin matches an email or a WordPress username, like wp-login.php.
func (s *UserStore) FindByLogin(ctx context.Context, login string) (models.User, error) {
	login = strings.TrimSpace(login)
	if strings.Contains(login, "@") {
		if u, err := s.FindByEmail(ctx, login); err == nil {
			return u, nil
		}
	}
	return s.findOne(ctx, "u.user_login = ?", login)
}

func (s *UserStore) FindByID(ctx context.Context, id string) (models.User, error) {
	uid := userIDArg(id)
	if uid == 0 {
		return models.User{}, ErrNotFound
	}
	return s.findOne(ctx, "u.ID = ?", uid)
}

var loginSanitizer = regexp.MustCompile(`[^a-z0-9_.\-@]`)

// uniqueLogin derives a free user_login from an email, like WooCommerce's
// wc_create_new_customer_username.
func (s *UserStore) uniqueLogin(ctx context.Context, q db.Querier, email string) (string, error) {
	base := strings.ToLower(strings.SplitN(email, "@", 2)[0])
	base = loginSanitizer.ReplaceAllString(base, "")
	if base == "" {
		base = "customer"
	}
	if len(base) > 50 {
		base = base[:50]
	}
	candidate := base
	for i := 1; i < 1000; i++ {
		var n int
		if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.db.T("users")+` WHERE user_login = ?`, candidate).Scan(&n); err != nil {
			return "", err
		}
		if n == 0 {
			return candidate, nil
		}
		candidate = base + strconv.Itoa(i)
	}
	return base + strconv.FormatInt(time.Now().UnixNano(), 36), nil
}

func nicename(login string) string {
	n := strings.ToLower(loginSanitizer.ReplaceAllString(strings.ToLower(login), "-"))
	if len(n) > 50 {
		n = n[:50]
	}
	return n
}

// SetUserMeta upserts one user meta value (WordPress keeps a single row per key).
func (s *UserStore) SetUserMeta(ctx context.Context, q db.Querier, userID uint64, key, value string) error {
	var metaID uint64
	err := q.QueryRowContext(ctx, `SELECT umeta_id FROM `+s.db.T("usermeta")+` WHERE user_id = ? AND meta_key = ? LIMIT 1`,
		userID, key).Scan(&metaID)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = q.ExecContext(ctx, `INSERT INTO `+s.db.T("usermeta")+` (user_id, meta_key, meta_value) VALUES (?, ?, ?)`,
			userID, key, value)
		return err
	}
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `UPDATE `+s.db.T("usermeta")+` SET meta_value = ? WHERE umeta_id = ?`, value, metaID)
	return err
}

// UserMeta is a user's meta values for keys (missing keys absent).
func (s *UserStore) UserMeta(ctx context.Context, userID string, keys ...string) (map[string]string, error) {
	out := map[string]string{}
	if len(keys) == 0 {
		return out, nil
	}
	args := []any{userIDArg(userID)}
	for _, k := range keys {
		args = append(args, k)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT meta_key, meta_value FROM `+s.db.T("usermeta")+
		` WHERE user_id = ? AND meta_key IN (`+placeholders(len(keys))+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var v sql.NullString
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v.String
	}
	return out, rows.Err()
}

// SetUserMetas upserts several meta values.
func (s *UserStore) SetUserMetas(ctx context.Context, userID string, values map[string]string) error {
	uid := userIDArg(userID)
	return s.db.InTx(ctx, func(tx *sql.Tx) error {
		for k, v := range values {
			if err := s.SetUserMeta(ctx, tx, uid, k, v); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *UserStore) setWPRole(ctx context.Context, q db.Querier, uid uint64, wpRole string) error {
	caps := php.NewArray().Set(wpRole, true)
	level := "0"
	switch wpRole {
	case WPRoleAdministrator:
		level = "10"
	case WPRoleShopManager:
		level = "0"
	}
	if err := s.SetUserMeta(ctx, q, uid, s.capsKey(), php.Marshal(caps)); err != nil {
		return err
	}
	return s.SetUserMeta(ctx, q, uid, s.levelKey(), level)
}

// CreateWP inserts a WordPress user with wpRole and the cwmerch status.
func (s *UserStore) CreateWP(ctx context.Context, email, passwordHash, name, surname, wpRole, status string) (models.User, error) {
	if _, err := s.FindByEmail(ctx, email); err == nil {
		return models.User{}, ErrDuplicateEmail
	}
	var id uint64
	err := s.db.InTx(ctx, func(tx *sql.Tx) error {
		login, err := s.uniqueLogin(ctx, tx, email)
		if err != nil {
			return err
		}
		display := strings.TrimSpace(name + " " + surname)
		if display == "" {
			display = login
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO `+s.db.T("users")+
			` (user_login, user_pass, user_nicename, user_email, user_url, user_registered, user_activation_key, user_status, display_name)
			VALUES (?, ?, ?, ?, '', UTC_TIMESTAMP(), '', 0, ?)`, login, passwordHash, nicename(login), email, display)
		if err != nil {
			if db.IsDuplicate(err) {
				return ErrDuplicateEmail
			}
			return err
		}
		last, _ := res.LastInsertId()
		id = uint64(last)
		metas := map[string]string{
			"nickname": login, "first_name": name, "last_name": surname, "description": "",
			"rich_editing": "true", "syntax_highlighting": "true", "comment_shortcuts": "false",
			"admin_color": "fresh", "use_ssl": "0", "show_admin_bar_front": "true", "locale": "",
		}
		if wpRole == WPRoleCustomer {
			metas["billing_email"] = email
			metas["billing_first_name"] = name
			metas["billing_last_name"] = surname
		}
		for k, v := range metas {
			if err := s.SetUserMeta(ctx, tx, id, k, v); err != nil {
				return err
			}
		}
		if err := s.setWPRole(ctx, tx, id, wpRole); err != nil {
			return err
		}
		data, _ := json.Marshal(cwmerchUserData{Status: status})
		_, err = tx.ExecContext(ctx, `INSERT INTO `+s.db.T("cwmerch_users")+` (user_id, data) VALUES (?, ?)`, id, string(data))
		return err
	})
	if err != nil {
		return models.User{}, err
	}
	return s.FindByID(ctx, idString(id))
}

// Create registers a customer. When the database has no administrator yet
// (a fresh cwmerch-only install), the first account becomes one.
func (s *UserStore) Create(ctx context.Context, email, passwordHash, name, surname string) (models.User, error) {
	return s.CreateWithMode(ctx, email, passwordHash, name, surname, models.ActivationModeAuto)
}

// CreateWithMode is Create with the activation mode deciding whether a new
// customer starts disabled (admin/email) or active (auto).
func (s *UserStore) CreateWithMode(ctx context.Context, email, passwordHash, name, surname, mode string) (models.User, error) {
	admins, err := s.CountAdministrators(ctx)
	if err != nil {
		return models.User{}, err
	}
	if admins == 0 {
		return s.CreateWP(ctx, email, passwordHash, name, surname, WPRoleAdministrator, "")
	}
	status := utils.If(mode == models.ActivationModeAuto, "", string(models.GlobalRoleDisabled))
	return s.CreateWP(ctx, email, passwordHash, name, surname, WPRoleCustomer, status)
}

// FindOrCreateOIDC signs in by a verified email: an existing account is used
// as is, otherwise a customer is created with no usable password (a random
// one, so WordPress refuses any password sign-in until it is reset).
func (s *UserStore) FindOrCreateOIDC(ctx context.Context, email, name, surname, activationMode string) (models.User, error) {
	user, err := s.FindByEmail(ctx, email)
	if err == nil {
		return user, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return models.User{}, err
	}
	random, _ := generateToken()
	mode := activationMode
	if mode == models.ActivationModeEmail {
		mode = models.ActivationModeAuto // the identity provider verified the address
	}
	return s.CreateWithMode(ctx, email, "$P$"+random[:31], name, surname, mode)
}

// CreateDisabled pre-creates an account (e.g. sharing an AI subscription
// with an address that has none yet).
func (s *UserStore) CreateDisabled(ctx context.Context, email, name, surname string) (models.User, error) {
	random, _ := generateToken()
	return s.CreateWP(ctx, email, "$P$"+random[:31], name, surname, WPRoleCustomer, string(models.GlobalRoleDisabled))
}

func (s *UserStore) patchExtras(ctx context.Context, id string, fn func(d *cwmerchUserData)) (models.User, error) {
	uid := userIDArg(id)
	if uid == 0 {
		return models.User{}, ErrNotFound
	}
	err := s.db.InTx(ctx, func(tx *sql.Tx) error {
		var raw string
		err := tx.QueryRowContext(ctx, `SELECT data FROM `+s.db.T("cwmerch_users")+` WHERE user_id = ? FOR UPDATE`, uid).Scan(&raw)
		exists := err == nil
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var d cwmerchUserData
		_ = json.Unmarshal([]byte(raw), &d)
		fn(&d)
		data, _ := json.Marshal(d)
		if exists {
			_, err = tx.ExecContext(ctx, `UPDATE `+s.db.T("cwmerch_users")+` SET data = ? WHERE user_id = ?`, string(data), uid)
		} else {
			_, err = tx.ExecContext(ctx, `INSERT INTO `+s.db.T("cwmerch_users")+` (user_id, data) VALUES (?, ?)`, uid, string(data))
		}
		return err
	})
	if err != nil {
		return models.User{}, err
	}
	return s.FindByID(ctx, id)
}

// List pages through users, filtered by a term and a cwmerch role. Every word
// of the term must appear in the email, login, display name, nicename or
// first/last name (or be the user's ID).
func (s *UserStore) List(ctx context.Context, term, role string, limit, offset int) ([]models.User, int64, error) {
	where := "1=1"
	args := []any{}
	for _, word := range strings.Fields(term) {
		like := "%" + likeEscape(word) + "%"
		id, _ := strconv.ParseUint(word, 10, 64)
		where += " AND (u.ID = ? OR u.user_email LIKE ? OR u.user_login LIKE ? OR u.display_name LIKE ? OR u.user_nicename LIKE ? OR EXISTS (SELECT 1 FROM " +
			s.db.T("usermeta") + " nm WHERE nm.user_id = u.ID AND nm.meta_key IN ('first_name', 'last_name', 'nickname') AND nm.meta_value LIKE ?))"
		args = append(args, id, like, like, like, like, like)
	}
	switch models.GlobalRole(role) {
	case models.GlobalRoleSuperuser:
		where += " AND EXISTS (SELECT 1 FROM " + s.db.T("usermeta") + " m WHERE m.user_id = u.ID AND m.meta_key = ? AND m.meta_value LIKE '%\"administrator\"%')"
		args = append(args, s.capsKey())
	case models.GlobalRoleManager:
		where += " AND EXISTS (SELECT 1 FROM " + s.db.T("usermeta") + " m WHERE m.user_id = u.ID AND m.meta_key = ? AND m.meta_value LIKE '%\"shop_manager\"%')"
		args = append(args, s.capsKey())
	case models.GlobalRoleDisabled, models.GlobalRoleBan:
		where += " AND EXISTS (SELECT 1 FROM " + s.db.T("cwmerch_users") + " g WHERE g.user_id = u.ID AND g.data LIKE ?)"
		args = append(args, "%\"status\":\""+role+"\"%")
	}
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*)`+s.userFrom()+` WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+userColumns+s.userFrom()+` WHERE `+where+
		` ORDER BY u.user_registered DESC, u.ID DESC LIMIT ? OFFSET ?`, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	users, err := s.scanUsers(ctx, rows)
	return users, total, err
}

func (s *UserStore) UpdatePicture(ctx context.Context, id, picture string, x, y float64) (models.User, error) {
	return s.patchExtras(ctx, id, func(d *cwmerchUserData) { d.Picture, d.PictureX, d.PictureY = picture, &x, &y })
}

// UpdateProfile sets first/last name (and display name when given) and, when
// passwordHash is set, the WordPress password.
func (s *UserStore) UpdateProfile(ctx context.Context, id, name, surname string, passwordHash *string) (models.User, error) {
	return s.UpdateAccount(ctx, id, name, surname, "", passwordHash)
}

func (s *UserStore) UpdateAccount(ctx context.Context, id, name, surname, displayName string, passwordHash *string) (models.User, error) {
	uid := userIDArg(id)
	err := s.db.InTx(ctx, func(tx *sql.Tx) error {
		if err := s.SetUserMeta(ctx, tx, uid, "first_name", name); err != nil {
			return err
		}
		if err := s.SetUserMeta(ctx, tx, uid, "last_name", surname); err != nil {
			return err
		}
		if strings.TrimSpace(displayName) != "" {
			if _, err := tx.ExecContext(ctx, `UPDATE `+s.db.T("users")+` SET display_name = ? WHERE ID = ?`, displayName, uid); err != nil {
				return err
			}
		}
		if passwordHash != nil {
			if _, err := tx.ExecContext(ctx, `UPDATE `+s.db.T("users")+` SET user_pass = ?, user_activation_key = '' WHERE ID = ?`, *passwordHash, uid); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return models.User{}, err
	}
	return s.touch(ctx, id)
}

// UpdateEmail changes the account email (unique).
func (s *UserStore) UpdateEmail(ctx context.Context, id, email string) error {
	email = strings.TrimSpace(email)
	if other, err := s.FindByEmail(ctx, email); err == nil && other.ID != id {
		return ErrDuplicateEmail
	}
	_, err := s.db.ExecContext(ctx, `UPDATE `+s.db.T("users")+` SET user_email = ? WHERE ID = ?`, email, userIDArg(id))
	return err
}

// SetPasswordHash replaces the stored hash (used to upgrade a legacy hash on sign-in).
func (s *UserStore) SetPasswordHash(ctx context.Context, id, hash string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE `+s.db.T("users")+` SET user_pass = ? WHERE ID = ?`, hash, userIDArg(id))
	return err
}

func (s *UserStore) touch(ctx context.Context, id string) (models.User, error) {
	return s.patchExtras(ctx, id, func(*cwmerchUserData) {})
}

// Confirm lifts a pending (disabled) state.
func (s *UserStore) Confirm(ctx context.Context, id string) (models.User, error) {
	return s.patchExtras(ctx, id, func(d *cwmerchUserData) {
		if d.Status == string(models.GlobalRoleDisabled) {
			d.Status = ""
		}
	})
}

func (s *UserStore) SetPendingTOTPSecret(ctx context.Context, id, secret string) (models.User, error) {
	return s.patchExtras(ctx, id, func(d *cwmerchUserData) { d.MFATOTPSecret = secret })
}

func (s *UserStore) ConfirmTOTP(ctx context.Context, id string) (models.User, error) {
	return s.SetMFAEnabled(ctx, id, true)
}

func (s *UserStore) DisableTOTP(ctx context.Context, id string, keepEnabled bool) (models.User, error) {
	return s.patchExtras(ctx, id, func(d *cwmerchUserData) { d.MFATOTPSecret, d.MFAEnabled = "", keepEnabled })
}

func (s *UserStore) SetMFAEnabled(ctx context.Context, id string, enabled bool) (models.User, error) {
	return s.patchExtras(ctx, id, func(d *cwmerchUserData) { d.MFAEnabled = enabled })
}

// AdminUserFields is what an administrator may change on an account.
type AdminUserFields struct {
	Name     *string
	Surname  *string
	Email    *string
	Role     *models.GlobalRole
	Password *string
}

// AdminUpdate applies f. superuser/manager/customer change the WordPress
// role; disabled/ban only cwmerch's status (WordPress keeps the role).
func (s *UserStore) AdminUpdate(ctx context.Context, id string, f AdminUserFields) (models.User, error) {
	user, err := s.FindByID(ctx, id)
	if err != nil {
		return models.User{}, err
	}
	uid := userIDArg(id)
	if f.Email != nil && *f.Email != user.Email {
		if err := s.UpdateEmail(ctx, id, *f.Email); err != nil {
			return models.User{}, err
		}
	}
	name, surname := user.Name, user.Surname
	if f.Name != nil {
		name = *f.Name
	}
	if f.Surname != nil {
		surname = *f.Surname
	}
	if _, err := s.UpdateProfile(ctx, id, name, surname, f.Password); err != nil {
		return models.User{}, err
	}
	if f.Role != nil {
		role := *f.Role
		status := ""
		switch role {
		case models.GlobalRoleSuperuser:
			err = s.setWPRole(ctx, s.db, uid, WPRoleAdministrator)
		case models.GlobalRoleManager:
			err = s.setWPRole(ctx, s.db, uid, WPRoleShopManager)
		case models.GlobalRoleConfirmed:
			if user.Role == models.GlobalRoleSuperuser || user.Role == models.GlobalRoleManager {
				err = s.setWPRole(ctx, s.db, uid, WPRoleCustomer)
			}
		case models.GlobalRoleDisabled, models.GlobalRoleBan:
			status = string(role)
		}
		if err != nil {
			return models.User{}, err
		}
		if _, err := s.patchExtras(ctx, id, func(d *cwmerchUserData) { d.Status = status }); err != nil {
			return models.User{}, err
		}
	}
	return s.FindByID(ctx, id)
}

// Delete removes the WordPress user, its meta and cwmerch's rows. Orders
// keep their customer id (as WordPress does with "attribute content").
func (s *UserStore) Delete(ctx context.Context, id string) error {
	uid := userIDArg(id)
	return s.db.InTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM `+s.db.T("users")+` WHERE ID = ?`, uid)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		for _, t := range []string{"usermeta", "cwmerch_users", "cwmerch_api_keys", "cwmerch_webauthn_credentials"} {
			if _, err := tx.ExecContext(ctx, `DELETE FROM `+s.db.T(t)+` WHERE user_id = ?`, uid); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM `+s.db.T("cwmerch_ai_connections")+` WHERE owner_id = ?`, uid)
		return err
	})
}

// SearchByEmail is the share picker's autocomplete.
func (s *UserStore) SearchByEmail(ctx context.Context, query string, limit int) ([]models.User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+userColumns+s.userFrom()+
		` WHERE u.user_email LIKE ? ORDER BY u.user_email LIMIT ?`, "%"+strings.TrimSpace(query)+"%", limit)
	if err != nil {
		return nil, err
	}
	return s.scanUsers(ctx, rows)
}

// likeEscape escapes LIKE wildcards so "%" or "_" in a search are literal
// (backslash is the default LIKE escape on MySQL/MariaDB and PostgreSQL).
func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
