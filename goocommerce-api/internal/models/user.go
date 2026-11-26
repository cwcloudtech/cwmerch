package models

import "time"

// GlobalRole is the account-wide role. It is derived from the WordPress role
// (the `<prefix>capabilities` user meta) unless goocommerce has disabled or
// banned the account, which it records in its own goo_users row so WordPress
// itself is left untouched.
type GlobalRole string

const (
	// GlobalRoleSuperuser is a WordPress administrator.
	GlobalRoleSuperuser GlobalRole = "superuser"
	// GlobalRoleManager is a WooCommerce shop_manager: the whole admin area
	// except user administration.
	GlobalRoleManager GlobalRole = "manager"
	// GlobalRoleConfirmed is a customer (or any other WordPress role).
	GlobalRoleConfirmed GlobalRole = "customer"
	GlobalRoleDisabled  GlobalRole = "disabled"
	GlobalRoleBan       GlobalRole = "ban"
)

// IsValidGlobalRole reports a role an administrator may set.
func IsValidGlobalRole(r GlobalRole) bool {
	switch r {
	case GlobalRoleSuperuser, GlobalRoleManager, GlobalRoleConfirmed, GlobalRoleDisabled, GlobalRoleBan:
		return true
	}
	return false
}

// CanManageShop reports whether role reaches the admin area.
func CanManageShop(r GlobalRole) bool { return r == GlobalRoleSuperuser || r == GlobalRoleManager }

// User is a WordPress user plus goocommerce's extras.
type User struct {
	ID           string     `json:"id"`
	Email        string     `json:"email"`
	Login        string     `json:"username"`
	DisplayName  string     `json:"displayName"`
	Name         string     `json:"name"`
	Surname      string     `json:"surname"`
	Role         GlobalRole `json:"role"`
	WPRoles      []string   `json:"wpRoles"`
	PasswordHash string     `json:"-"`
	Picture      string     `json:"picture,omitempty"`
	PictureX     float64    `json:"pictureX"`
	PictureY     float64    `json:"pictureY"`
	MFAEnabled   bool       `json:"-"`
	// MFATOTPSecret is written when enrolment starts; MFAEnabled only once a
	// code from it was confirmed.
	MFATOTPSecret string    `json:"-"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type UserResponse struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	Username    string     `json:"username"`
	DisplayName string     `json:"displayName"`
	Name        string     `json:"name"`
	Surname     string     `json:"surname"`
	Role        GlobalRole `json:"role"`
	Token       string     `json:"token"`
	Picture     string     `json:"picture,omitempty"`
	PictureX    float64    `json:"pictureX"`
	PictureY    float64    `json:"pictureY"`
	MFAEnabled  bool       `json:"mfaEnabled"`
	I18nCode    string     `json:"i18nCode,omitempty"`
}

// MFAChallengeResponse replaces UserResponse when the password was right but
// the account has a second factor: only a short-lived challenge token is issued.
type MFAChallengeResponse struct {
	MFARequired    bool   `json:"mfaRequired"`
	ChallengeToken string `json:"challengeToken"`
	HasTOTP        bool   `json:"hasTotp"`
	HasWebAuthn    bool   `json:"hasWebAuthn"`
}

type UserMeResponse struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	Username    string     `json:"username"`
	DisplayName string     `json:"displayName"`
	Name        string     `json:"name"`
	Surname     string     `json:"surname"`
	Role        GlobalRole `json:"role"`
	Picture     string     `json:"picture,omitempty"`
	PictureX    float64    `json:"pictureX"`
	PictureY    float64    `json:"pictureY"`
	MFAEnabled  bool       `json:"mfaEnabled"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	I18nCode    string     `json:"i18nCode,omitempty"`
}

// MeResponse renders u for the account endpoints.
func MeResponse(u User, activationMode string) UserMeResponse {
	return UserMeResponse{
		ID: u.ID, Email: u.Email, Username: u.Login, DisplayName: u.DisplayName,
		Name: u.Name, Surname: u.Surname, Role: u.Role, Picture: u.Picture,
		PictureX: u.PictureX, PictureY: u.PictureY, MFAEnabled: u.MFAEnabled,
		CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt,
		I18nCode: I18nCodeForRole(u.Role, activationMode),
	}
}

// LoginResponse renders u with its session token.
func LoginResponse(u User, token, activationMode string) UserResponse {
	return UserResponse{
		ID: u.ID, Email: u.Email, Username: u.Login, DisplayName: u.DisplayName,
		Name: u.Name, Surname: u.Surname, Role: u.Role, Token: token, Picture: u.Picture,
		PictureX: u.PictureX, PictureY: u.PictureY, MFAEnabled: u.MFAEnabled,
		I18nCode: I18nCodeForRole(u.Role, activationMode),
	}
}
