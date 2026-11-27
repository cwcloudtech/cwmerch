package store

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"cwmerch-api/internal/db"

	"cwmerch-api/internal/models"
	"cwmerch-api/internal/utils"
)

// defaultAuditPageSize/maxAuditPageSize bound what the screen can ask for.
// The cap is there because the trail only ever grows: without it a client
// could ask for the whole table in one request.
const (
	defaultAuditPageSize = 50
	maxAuditPageSize     = 200
)

// AuditObserver is notified once per entry actually written, so audit events
// can be counted as a metric without this package importing
// internal/metrics - which imports this one. Same shape, and same reason, as
// middleware.EndpointObserver.
type AuditObserver func(action, organizationID, userID string)

// AuditStore is the append-only trail of acts that change who can do what, or
// what this deployment is configured to do. It has no Update and no Delete,
// by construction: a trail that can be rewritten is not one. The only way a
// row ever changes is a reference being nulled when the account or
// organization it points at is deleted (see V10__add_audit_log.sql).
type AuditStore struct {
	db      *db.DB
	observe AuditObserver
	sinks   []AuditSink
}

// AuditSink is told about every entry written, with the actor's email
// (Discord: a notification on every user action).
type AuditSink func(ctx context.Context, e AuditEvent, actorEmail string)

// AddSink attaches a sink (set up once at start-up).
func (s *AuditStore) AddSink(sink AuditSink) { s.sinks = append(s.sinks, sink) }

func NewAuditStore(d *db.DB) *AuditStore {
	return &AuditStore{db: d}
}

// SetObserver attaches the metrics counter. It is a setter rather than a
// constructor argument because metrics.Setup needs the stores to build its
// gauges, so the store necessarily exists first; main calls this once, before
// the server starts serving.
func (s *AuditStore) SetObserver(observe AuditObserver) {
	s.observe = observe
}

// AuditEvent is one act to record. UserID is empty for the acts with no
// authenticated actor, OrganizationID for those that belong to no
// organization; Detail is free-form context for the screen - what changed,
// from what to what - and is never relied on by code.
type AuditEvent struct {
	UserID         string
	OrganizationID string
	Action         string
	SubjectType    string
	SubjectID      string
	Detail         map[string]any
}

// auditData is the shape of the row's data column, following the same
// pattern as the other stores here. It is only ever unmarshalled: the column
// is written by the INSERT in Record, which fills ActorEmail from users
// itself.
type auditData struct {
	ActorEmail string         `json:"actorEmail,omitempty"`
	Detail     map[string]any `json:"detail,omitempty"`
}

// Record writes one entry. It deliberately returns nothing: an audit write
// must never be the reason a user's action fails, so a failure is logged at
// error level and the action stands. Refusing the action instead would make
// the audit trail a single point of failure for the whole app - every login,
// every organization edit would depend on one more successful write.
//
// Every entry also goes out through the process logger at info level. That
// is one slog call, not two sinks to keep in step: internal/telemetry
// installs a multi-handler as the default logger, so the same record reaches
// stdout and - when CWMERCH_OTEL_ENDPOINT is set - the OTLP collector.
func (s *AuditStore) Record(ctx context.Context, e AuditEvent) {
	// An entry with no detail stores an empty object rather than JSON null,
	// so data->>'detail' is always an object for anything reading the table
	// directly with SQL.
	detail := []byte("{}")
	if len(e.Detail) > 0 {
		encoded, err := json.Marshal(e.Detail)
		if err != nil {
			slog.Error("audit detail could not be encoded", "audit_action", e.Action, "error", err)
			return
		}
		detail = encoded
	}

	// WithoutCancel, not ctx itself: Record is always called after the act
	// has already succeeded, so a client that hangs up at that moment would
	// otherwise cancel the write and leave the act unrecorded. The context's
	// values (the request's trace span) are kept, only its cancellation is
	// dropped.
	ctx = context.WithoutCancel(ctx)

	// INSERT ... SELECT so the acting account's email is snapshotted from
	// users in the same statement, without a second round trip: the trail
	// must keep naming who acted after that account is renamed or deleted.
	var actorEmail string
	if uid := userIDArg(e.UserID); uid != 0 {
		_ = s.db.QueryRowContext(ctx, `SELECT user_email FROM `+s.db.T("users")+` WHERE ID = ?`, uid).Scan(&actorEmail)
	}
	var detailMap map[string]any
	_ = json.Unmarshal(detail, &detailMap)
	if utils.IsNotBlank(e.OrganizationID) {
		if detailMap == nil {
			detailMap = map[string]any{}
		}
		detailMap["organizationId"] = e.OrganizationID
	}
	data, _ := json.Marshal(auditData{ActorEmail: actorEmail, Detail: detailMap})
	if _, err := s.db.ExecContext(ctx, `INSERT INTO `+s.db.T("cwmerch_audit_log")+
		` (id, user_id, action, subject_type, subject_id, data) VALUES (?, ?, ?, ?, ?, ?)`,
		newID(), nullUserID(e.UserID), e.Action, e.SubjectType, e.SubjectID, string(data)); err != nil {
		slog.Error("audit entry could not be written", "audit_action", e.Action,
			"user", e.UserID, "error", err)
		return
	}

	slog.Info("audit",
		"audit_action", e.Action,
		"user", e.UserID,
		"organization", e.OrganizationID,
		"subject_type", e.SubjectType,
		"subject_id", e.SubjectID,
		"detail", string(detail),
	)

	if s.observe != nil {
		s.observe(e.Action, e.OrganizationID, e.UserID)
	}
	for _, sink := range s.sinks {
		sink(ctx, e, actorEmail)
	}
}

// AuditFilter narrows the trail for the superadmin screen. An empty field
// means "no filter on this field", so a single query shape serves every
// combination of filters the screen can ask for. Page is 1-based.
type AuditFilter struct {
	Action      string
	SubjectType string
	SubjectID   string
	From        string // YYYY-MM-DD, inclusive lower bound on created_at
	To          string // YYYY-MM-DD, inclusive upper bound (the whole day)
	UserID      string
	Page        int
	Size        int
}

// limitOffset clamps the requested page and asks for one row more than the
// page needs, which is how List knows whether a next page exists without a
// second COUNT query over a table that only grows.
func (f AuditFilter) limitOffset() (int, int) {
	size := f.Size
	if size <= 0 {
		size = defaultAuditPageSize
	}
	if size > maxAuditPageSize {
		size = maxAuditPageSize
	}
	page := f.Page
	if page < 1 {
		page = 1
	}
	return size + 1, (page - 1) * size
}

// List returns one page of the trail, newest first, plus the distinct actions
// present in the whole table so the screen can offer them as a filter without
// a hardcoded list that drifts from models.Audit* .
func (s *AuditStore) List(ctx context.Context, f AuditFilter) (models.AuditPage, error) {
	limit, offset := f.limitOffset()

	conds := []string{"1=1"}
	args := []any{}
	add := func(cond string, v any) { conds = append(conds, cond); args = append(args, v) }
	if f.Action != "" {
		add("action = ?", f.Action)
	}
	if f.SubjectType != "" {
		add("subject_type = ?", f.SubjectType)
	}
	if f.SubjectID != "" {
		add("subject_id = ?", f.SubjectID)
	}
	if f.UserID != "" {
		add("user_id = ?", userIDArg(f.UserID))
	}
	if f.From != "" {
		add("created_at >= ?", f.From)
	}
	if f.To != "" {
		add("created_at < DATE_ADD(?, INTERVAL 1 DAY)", f.To)
	}
	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, `SELECT id, COALESCE(user_id, 0), action, subject_type, subject_id, data, created_at FROM `+
		s.db.T("cwmerch_audit_log")+` WHERE `+strings.Join(conds, " AND ")+` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return models.AuditPage{}, err
	}
	defer rows.Close()

	page := models.AuditPage{Entries: []models.AuditEntry{}, Actions: []string{}}
	for rows.Next() {
		var e models.AuditEntry
		var raw string
		var uid uint64
		if err := rows.Scan(&e.ID, &uid,
			&e.Action, &e.SubjectType, &e.SubjectID, &raw, &e.CreatedAt); err != nil {
			return models.AuditPage{}, err
		}
		var d auditData
		_ = json.Unmarshal([]byte(raw), &d)
		e.UserID = idString(uid)
		e.UserEmail = d.ActorEmail
		e.Detail = d.Detail
		page.Entries = append(page.Entries, e)
	}
	if err := rows.Err(); err != nil {
		return models.AuditPage{}, err
	}

	// The extra row fetched by limitOffset is the answer to "is there a next
	// page?", and is not part of this one.
	if len(page.Entries) == limit {
		page.HasMore = true
		page.Entries = page.Entries[:limit-1]
	}

	// The filter values come from the code's own lists, not from a DISTINCT
	// over audit_log: the table now grows with every time record, and the
	// screen should not pay for a scan of it on each page load. See
	// models.AuditActions.
	page.Actions = models.AuditActions
	page.SubjectTypes = models.AuditSubjectTypes
	return page, nil
}

// nullUUID turns an empty id into a SQL NULL, so the nullable references in
// audit_log can be left unset without the caller building a *string.
