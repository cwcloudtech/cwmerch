package handlers

import (
	"strings"

	"context"
	"cwmerch-api/internal/wppass"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"cwmerch-api/internal/authtoken"
	"cwmerch-api/internal/email"
	"cwmerch-api/internal/middleware"
	"cwmerch-api/internal/models"
	"cwmerch-api/internal/notify"
	"cwmerch-api/internal/store"
	"cwmerch-api/internal/utils"
)

// mfaLoginTokenTTL is how long the challenge token returned by Login (when
// MFA is enabled) stays valid for finishing login via one of the
// /v1/users/login/mfa/* endpoints.
const mfaLoginTokenTTL = 5 * time.Minute

type UserHandler struct {
	auditor
	connTracker
	users                *store.UserStore
	webauthnCreds        *store.WebAuthnCredentialStore
	jwtSecret            string
	maxImageSize         int64
	activationMode       string
	mailer               *email.Sender
	apiBaseURL           string
	uiBaseURL            string
	confirmationTokenTTL time.Duration
	// RegistrationAllowed closes sign-up (shop used as an internal tool).
	RegistrationAllowed func(r *http.Request) bool
	// OnRegistered runs after a successful registration (welcome email).
	OnRegistered func(ctx context.Context, user models.User)
}

// WithConn wires the IP↔user association + Discord sign-in/out notices (optional).
func (h *UserHandler) WithConn(visits *store.SessionStore, enrich middleware.Enricher, disc *notify.Discord) *UserHandler {
	h.setConn(visits, enrich, disc)
	return h
}

func NewUserHandler(users *store.UserStore, webauthnCreds *store.WebAuthnCredentialStore, audit *store.AuditStore, jwtSecret string, maxImageSize int64, activationMode string, mailer *email.Sender, apiBaseURL, uiBaseURL string, confirmationTokenTTL time.Duration) *UserHandler {
	return &UserHandler{
		auditor: newAuditor(audit),
		users:   users, webauthnCreds: webauthnCreds, jwtSecret: jwtSecret, maxImageSize: maxImageSize,
		activationMode: activationMode, mailer: mailer, apiBaseURL: apiBaseURL, uiBaseURL: uiBaseURL,
		confirmationTokenTTL: confirmationTokenTTL,
	}
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type registerPayload struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
	Surname  string `json:"surname"`
}

func (h *UserHandler) Register(w http.ResponseWriter, r *http.Request) {
	if h.RegistrationAllowed != nil && !h.RegistrationAllowed(r) {
		writeError(w, http.StatusForbidden, "Registration is closed", "errors.registrationDisabled")
		return
	}
	var p registerPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "Please add all fields", CodeInvalidRequestBody)
		return
	}
	if utils.IsBlank(p.Email) || utils.IsBlank(p.Password) || utils.IsBlank(p.Name) || utils.IsBlank(p.Surname) {
		writeError(w, http.StatusBadRequest, "Please add all fields", CodeAllFieldsRequired)
		return
	}
	if ok, code := utils.IsPasswordValid(p.Password); !ok {
		writeInvalidPassword(w, code)
		return
	}

	if _, err := h.users.FindByEmail(r.Context(), p.Email); err == nil {
		writeError(w, http.StatusBadRequest, "A user with this email already exists", CodeDuplicateEmail)
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, err.Error(), CodeInternal)
		return
	}

	hash, err := hashPassword(p.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), CodeInternal)
		return
	}

	user, err := h.users.CreateWithMode(r.Context(), strings.TrimSpace(p.Email), hash, p.Name, p.Surname, h.activationMode)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid user data", CodeInvalidUserData)
		return
	}

	if user.Role == models.GlobalRoleDisabled && h.activationMode == models.ActivationModeEmail {
		h.sendConfirmationEmail(r.Context(), user)
	}
	if h.OnRegistered != nil {
		h.OnRegistered(r.Context(), user)
	}

	token, err := authtoken.Generate(h.jwtSecret, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), CodeInternal)
		return
	}

	// Attributed to the new account itself: there is no other actor, and the
	// entry's point is that this account came into existence.
	h.recordAs(r.Context(), user.ID, models.AuditAccountRegistered, models.AuditSubjectUser, user.ID,
		map[string]any{"email": user.Email, "role": string(user.Role)})
	h.associate(r, user.ID)
	h.notifyRegister(r, user)

	writeJSON(w, http.StatusCreated, models.LoginResponse(user, token, h.activationMode))
}

// sendConfirmationEmail mints a purpose-scoped confirmation token for user
// and emails the confirmation link (best-effort - see email.Sender).
func (h *UserHandler) sendConfirmationEmail(ctx context.Context, user models.User) {
	token, err := authtoken.GeneratePurpose(h.jwtSecret, user.ID, authtoken.PurposeConfirmAccount, h.confirmationTokenTTL)
	if err != nil {
		slog.Error("failed to generate confirmation token", "error", err)
		return
	}
	confirmURL := h.apiBaseURL + "/v1/user/confirmation?token=" + url.QueryEscape(token)
	h.mailer.SendConfirmation(ctx, user.Email, confirmURL)
}

func (h *UserHandler) Login(w http.ResponseWriter, r *http.Request) {
	var creds credentials
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid credentials", CodeInvalidCredentials)
		return
	}

	// Like wp-login.php: the email or the WordPress username.
	user, err := h.users.FindByLogin(r.Context(), creds.Email)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Invalid credentials", CodeInvalidCredentials)
		return
	}

	if !checkPassword(creds.Password, user.PasswordHash) {
		writeError(w, http.StatusUnauthorized, "Invalid credentials", CodeInvalidCredentials)
		return
	}
	// Upgrade a legacy (MD5/phpass) hash to the configured format, as
	// WordPress does on sign-in.
	if wppass.NeedsRehash(user.PasswordHash, passwordAlgo) && passwordAlgo != wppass.AlgoPHPass {
		if hashed, err := hashPassword(creds.Password); err == nil {
			_ = h.users.SetPasswordHash(r.Context(), user.ID, hashed)
		}
	}

	if user.MFAEnabled {
		h.respondMFAChallenge(w, r, user)
		return
	}

	token, err := authtoken.Generate(h.jwtSecret, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), CodeInternal)
		return
	}

	h.recordAs(r.Context(), user.ID, models.AuditSignedIn, models.AuditSubjectUser, user.ID,
		map[string]any{"method": "password"})
	h.associate(r, user.ID)
	h.notifyLogin(r, user)

	writeJSON(w, http.StatusOK, models.LoginResponse(user, token, h.activationMode))
}

// Logout records the other half of a session. cwmerch's sessions are
// stateless JWTs, so there is nothing server-side to invalidate and the
// client simply drops its token - this endpoint exists so the trail shows
// when a session ended and not only when it began. It therefore does the
// absolute minimum: no state is touched, and a client that never calls it
// (or whose token has already expired) is unaffected.
func (h *UserHandler) Logout(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserIDFromContext(r.Context())
	h.record(r, models.AuditSignedOut, models.AuditSubjectUser, userID, nil)
	if u, err := h.users.FindByID(r.Context(), userID); err == nil {
		h.notifyLogout(r, u)
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Signed out."})
}

// respondMFAChallenge replaces the normal login response when the account
// has MFA enabled: instead of a usable session token, it mints a short-lived
// PurposeMFALogin token the client exchanges for one via
// MFAHandler.LoginTOTP/LoginWebAuthnFinish once the second factor is
// verified (see ai-instruct-68).
func (h *UserHandler) respondMFAChallenge(w http.ResponseWriter, r *http.Request, user models.User) {
	challengeToken, err := authtoken.GeneratePurpose(h.jwtSecret, user.ID, authtoken.PurposeMFALogin, mfaLoginTokenTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), CodeInternal)
		return
	}

	webauthnCount, err := h.webauthnCreds.CountByUser(r.Context(), user.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, models.MFAChallengeResponse{
		MFARequired:    true,
		ChallengeToken: challengeToken,
		HasTOTP:        utils.IsNotBlank(user.MFATOTPSecret),
		HasWebAuthn:    webauthnCount > 0,
	})
}

func (h *UserHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserIDFromContext(r.Context())

	user, err := h.users.FindByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "User not found", CodeUserNotFound)
		return
	}

	writeJSON(w, http.StatusOK, models.MeResponse(user, h.activationMode))
}

type updatePicturePayload struct {
	Picture string  `json:"picture"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
}

// UpdatePicture lets the connected user set their own avatar picture
// (base64, stored uncropped) along with the x/y position used to display it,
// shown in the profile dropdown.
func (h *UserHandler) UpdatePicture(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserIDFromContext(r.Context())

	var p updatePicturePayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", CodeInvalidRequestBody)
		return
	}
	if utils.ImageSizeExceeds(p.Picture, h.maxImageSize) {
		writeError(w, http.StatusBadRequest, "Image is too large", CodeImageTooLarge)
		return
	}

	user, err := h.users.UpdatePicture(r.Context(), userID, p.Picture, p.X, p.Y)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, models.MeResponse(user, h.activationMode))
}

type updateProfilePayload struct {
	Name            string `json:"name"`
	Surname         string `json:"surname"`
	DisplayName     string `json:"displayName"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirmPassword"`
}

// UpdateProfile lets the connected user set their own name, surname and,
// optionally, a new password (left untouched when the field is empty).
func (h *UserHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserIDFromContext(r.Context())

	var p updateProfilePayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", CodeInvalidRequestBody)
		return
	}
	if utils.IsBlank(p.Name) || utils.IsBlank(p.Surname) {
		writeError(w, http.StatusBadRequest, "Please add name and surname fields", CodeNameAndSurnameRequired)
		return
	}

	var passwordHash *string
	if utils.IsNotBlank(p.Password) {
		if p.Password != p.ConfirmPassword {
			writeError(w, http.StatusBadRequest, "Passwords do not match", CodePasswordsMismatch)
			return
		}
		if ok, code := utils.IsPasswordValid(p.Password); !ok {
			writeInvalidPassword(w, code)
			return
		}
		hashed, err := hashPassword(p.Password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error(), CodeInternal)
			return
		}
		passwordHash = &hashed
	}

	user, err := h.users.UpdateAccount(r.Context(), userID, p.Name, p.Surname, p.DisplayName, passwordHash)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, models.MeResponse(user, h.activationMode))
}

// Search powers email autocomplete when inviting members: it returns users
// whose email contains the "q" query param.
func (h *UserHandler) Search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if len(query) < 2 {
		writeJSON(w, http.StatusOK, []models.UserMeResponse{})
		return
	}

	users, err := h.users.SearchByEmail(r.Context(), query, 10)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	results := make([]models.UserMeResponse, len(users))
	for i, u := range users {
		results[i] = models.UserMeResponse{
			ID: u.ID, Email: u.Email, Name: u.Name, Surname: u.Surname,
			Role: u.Role, Username: u.Login, DisplayName: u.DisplayName, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt,
		}
	}
	writeJSON(w, http.StatusOK, results)
}

// Confirm is the endpoint a user's emailed confirmation link points to
// (activation mode "email"): GET /v1/user/confirmation?token=... - clicked
// directly from the email, so on success/failure it redirects to the
// frontend rather than returning JSON, mirroring the OIDC callback flow.
func (h *UserHandler) Confirm(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	userID, err := authtoken.ParsePurpose(h.jwtSecret, token, authtoken.PurposeConfirmAccount)
	if err != nil {
		h.redirectToLogin(w, r, "invalid")
		return
	}

	user, err := h.users.FindByID(r.Context(), userID)
	if err != nil {
		h.redirectToLogin(w, r, "invalid")
		return
	}

	// A banned account can never be confirmed this way, even with a valid,
	// unexpired token - being banned overrides a pending confirmation.
	if user.Role == models.GlobalRoleBan {
		h.redirectToLogin(w, r, "banned")
		return
	}

	if user.Role == models.GlobalRoleDisabled {
		if _, err := h.users.Confirm(r.Context(), userID); err != nil {
			h.redirectToLogin(w, r, "invalid")
			return
		}
		// Only recorded when it changed something: the link can be clicked
		// again on an already-confirmed account, and that is not an event.
		h.recordAs(r.Context(), userID, models.AuditAccountConfirmed, models.AuditSubjectUser, userID,
			map[string]any{"email": user.Email})
	}

	h.redirectToLogin(w, r, utils.EMPTY)
}

func (h *UserHandler) redirectToLogin(w http.ResponseWriter, r *http.Request, confirmError string) {
	target := h.uiBaseURL + "/login?confirmed=1"
	if utils.IsNotBlank(confirmError) {
		target = h.uiBaseURL + "/login?confirmed=0&reason=" + url.QueryEscape(confirmError)
	}
	http.Redirect(w, r, target, http.StatusFound)
}

type forgotPasswordPayload struct {
	Email string `json:"email"`
}

// ForgotPassword emails a password-renewal link when the address matches an
// account. It always responds 200 with the same generic message regardless
// of whether the account exists (or is banned, which silently skips
// sending) so the endpoint can't be used to test which emails are
// registered.
func (h *UserHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var p forgotPasswordPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil || utils.IsBlank(p.Email) {
		writeError(w, http.StatusBadRequest, "Please add an email", CodeInvalidEmail)
		return
	}

	user, err := h.users.FindByEmail(r.Context(), p.Email)
	// A banned user can never request a password renewal.
	if err == nil && user.Role != models.GlobalRoleBan {
		token, err := authtoken.GeneratePurpose(h.jwtSecret, user.ID, authtoken.PurposeResetPassword, h.confirmationTokenTTL)
		if err != nil {
			slog.Error("failed to generate password reset token", "error", err)
		} else {
			resetURL := h.uiBaseURL + "/reset-password?token=" + url.QueryEscape(token)
			h.mailer.SendPasswordReset(r.Context(), user.Email, resetURL)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "If this email is registered, a reset link has been sent."})
}

type resetPasswordPayload struct {
	Token           string `json:"token"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirmPassword"`
}

// ResetPassword sets a new password from a token minted by ForgotPassword.
func (h *UserHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var p resetPasswordPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil || utils.IsBlank(p.Token) || utils.IsBlank(p.Password) {
		writeError(w, http.StatusBadRequest, "Please add a token and a password", CodeInvalidRequestBody)
		return
	}
	if p.Password != p.ConfirmPassword {
		writeError(w, http.StatusBadRequest, "Passwords do not match", CodePasswordsMismatch)
		return
	}
	if ok, code := utils.IsPasswordValid(p.Password); !ok {
		writeInvalidPassword(w, code)
		return
	}

	userID, err := authtoken.ParsePurpose(h.jwtSecret, p.Token, authtoken.PurposeResetPassword)
	if err != nil {
		writeError(w, http.StatusBadRequest, "This reset link is invalid or has expired", CodeInvalidToken)
		return
	}

	user, err := h.users.FindByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "This reset link is invalid or has expired", CodeInvalidToken)
		return
	}
	// A banned user can never renew their password, even with a valid,
	// unexpired reset token.
	if user.Role == models.GlobalRoleBan {
		writeError(w, http.StatusForbidden, "Your account has been banned by an administrator.", CodeInvalidToken)
		return
	}

	hashed, err := hashPassword(p.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), CodeInternal)
		return
	}

	if _, err := h.users.UpdateProfile(r.Context(), userID, user.Name, user.Surname, &hashed); err != nil {
		writeStoreError(w, err)
		return
	}

	// Attributed to the account whose password changed: the caller proved
	// they hold its emailed reset token, which is all the identity there is
	// on this endpoint.
	h.recordAs(r.Context(), userID, models.AuditAccountPasswordReset, models.AuditSubjectUser, userID,
		map[string]any{"email": user.Email})

	writeJSON(w, http.StatusOK, map[string]string{"message": "Password updated."})
}
