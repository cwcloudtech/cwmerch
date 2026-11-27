package models

import "time"

// Audit actions, namespaced by what they are about (the admin screen groups
// and translates them under audit.actions.*). Every recorded act is also
// logged, counted on the metrics and announced on Discord: "a notification
// on every user action". Reads are never audited.
const (
	AuditDatabaseCloned = "database.cloned"
	AuditSignedIn       = "auth.signedIn"
	AuditSignedOut      = "auth.signedOut"

	AuditAccountRegistered    = "account.registered"
	AuditAccountConfirmed     = "account.confirmed"
	AuditAccountPasswordReset = "account.passwordReset"
	AuditAccountUpdated       = "account.updated"
	AuditAddressesUpdated     = "account.addressesUpdated"

	AuditUserRoleChanged    = "user.roleChanged"
	AuditUserUpdatedByAdmin = "user.updatedByAdmin"
	AuditUserDeleted        = "user.deleted"
	AuditUserMFACleared     = "user.mfaCleared"

	AuditMFATOTPEnabled        = "mfa.totpEnabled"
	AuditMFATOTPDisabled       = "mfa.totpDisabled"
	AuditMFAWebAuthnRegistered = "mfa.webauthnRegistered"
	AuditMFAWebAuthnDeleted    = "mfa.webauthnDeleted"
	AuditApiKeyCreated         = "apiKey.created"
	AuditApiKeyDeleted         = "apiKey.deleted"

	AuditProductCreated    = "product.created"
	AuditProductUpdated    = "product.updated"
	AuditProductDeleted    = "product.deleted"
	AuditProductDuplicated = "product.duplicated"
	AuditVariationSaved    = "variation.saved"
	AuditVariationDeleted  = "variation.deleted"
	AuditTermSaved         = "term.saved"
	AuditTermDeleted       = "term.deleted"
	AuditAttributeSaved    = "attribute.saved"
	AuditAttributeDeleted  = "attribute.deleted"
	AuditMediaUploaded     = "media.uploaded"

	AuditOrderPlaced        = "order.placed"
	AuditOrderCreated       = "order.created"
	AuditOrderUpdated       = "order.updated"
	AuditOrderStatusChanged = "order.statusChanged"
	AuditOrderDeleted       = "order.deleted"
	AuditOrderNoteAdded     = "order.noteAdded"
	AuditOrderNoteDeleted   = "order.noteDeleted"
	AuditOrderRefunded      = "order.refunded"
	AuditOrderEmailSent     = "order.emailSent"

	AuditCouponSaved   = "coupon.saved"
	AuditCouponDeleted = "coupon.deleted"

	AuditSettingsUpdated   = "settings.updated"
	AuditTaxSaved          = "tax.saved"
	AuditTaxDeleted        = "tax.deleted"
	AuditShippingSaved     = "shipping.saved"
	AuditShippingDeleted   = "shipping.deleted"
	AuditGatewayUpdated    = "gateway.updated"
	AuditEmailUpdated      = "email.updated"
	AuditAppearanceUpdated = "appearance.updated"

	AuditReviewCreated  = "review.created"
	AuditReviewUpdated  = "review.updated"
	AuditReviewDeleted  = "review.deleted"
	AuditReviewReplied  = "review.replied"
	AuditCustomerUpdate = "customer.updated"

	AuditContactSent = "contact.sent"
	// AuditAICall is a question put to the assistant (or through the CWCloud adapter).
	AuditAICall = "ai.call"
)

// Audit subject types - what the action's subject_id points at.
const (
	AuditSubjectUser         = "user"
	AuditSubjectApiKey       = "apiKey"
	AuditSubjectCredential   = "credential"
	AuditSubjectAIConnection = "aiConnection"
	AuditSubjectProduct      = "product"
	AuditSubjectTerm         = "term"
	AuditSubjectAttribute    = "attribute"
	AuditSubjectMedia        = "media"
	AuditSubjectOrder        = "order"
	AuditSubjectCoupon       = "coupon"
	AuditSubjectSetting      = "setting"
	AuditSubjectReview       = "review"
	AuditSubjectCustomer     = "customer"
)

// AuditActions and AuditSubjectTypes feed the admin screen's filters.
var AuditActions = []string{
	AuditSignedIn, AuditSignedOut, AuditAccountRegistered, AuditAccountConfirmed, AuditAccountPasswordReset, AuditAccountUpdated,
	AuditAddressesUpdated, AuditUserRoleChanged, AuditUserUpdatedByAdmin, AuditUserDeleted, AuditUserMFACleared,
	AuditMFATOTPEnabled, AuditMFATOTPDisabled, AuditMFAWebAuthnRegistered, AuditMFAWebAuthnDeleted, AuditApiKeyCreated, AuditApiKeyDeleted,
	AuditProductCreated, AuditProductUpdated, AuditProductDeleted, AuditProductDuplicated, AuditVariationSaved, AuditVariationDeleted,
	AuditTermSaved, AuditTermDeleted, AuditAttributeSaved, AuditAttributeDeleted, AuditMediaUploaded,
	AuditOrderPlaced, AuditOrderCreated, AuditOrderUpdated, AuditOrderStatusChanged, AuditOrderDeleted, AuditOrderNoteAdded,
	AuditOrderNoteDeleted, AuditOrderRefunded, AuditOrderEmailSent, AuditCouponSaved, AuditCouponDeleted,
	AuditSettingsUpdated, AuditTaxSaved, AuditTaxDeleted, AuditShippingSaved, AuditShippingDeleted, AuditGatewayUpdated,
	AuditEmailUpdated, AuditAppearanceUpdated, AuditReviewCreated, AuditReviewUpdated, AuditReviewDeleted, AuditReviewReplied,
	AuditCustomerUpdate, AuditContactSent, AuditAICall,
}

var AuditSubjectTypes = []string{
	AuditSubjectUser, AuditSubjectApiKey, AuditSubjectCredential, AuditSubjectAIConnection, AuditSubjectProduct, AuditSubjectTerm,
	AuditSubjectAttribute, AuditSubjectMedia, AuditSubjectOrder, AuditSubjectCoupon, AuditSubjectSetting, AuditSubjectReview, AuditSubjectCustomer,
}

// AuditEntry is one recorded act, as the superadmin screen reads it.
type AuditEntry struct {
	ID string `json:"id"`
	// UserID/OrganizationID are empty when the row's reference is null -
	// either the act had no actor/organization, or the row it pointed at has
	// since been deleted (see V10__add_audit_log.sql).
	UserID         string `json:"userId,omitempty"`
	OrganizationID string `json:"organizationId,omitempty"`
	// UserEmail is the acting account's email as it was when the act
	// happened, snapshotted into the entry rather than joined at read time:
	// renaming or deleting an account must not rewrite what the trail says
	// about what it did.
	UserEmail   string         `json:"userEmail,omitempty"`
	Action      string         `json:"action"`
	SubjectType string         `json:"subjectType"`
	SubjectID   string         `json:"subjectId,omitempty"`
	Detail      map[string]any `json:"detail,omitempty"`
	CreatedAt   time.Time      `json:"createdAt"`
}

// AuditPage is one page of the trail plus what the screen needs to render its
// controls: the values its two filters offer, and whether a next page exists.
// The filter values are sent with the page rather than hardcoded in the UI so
// that an older screen against a newer API still offers everything the API
// can filter on.
type AuditPage struct {
	Entries      []AuditEntry `json:"entries"`
	Actions      []string     `json:"actions"`
	SubjectTypes []string     `json:"subjectTypes"`
	HasMore      bool         `json:"hasMore"`
}
