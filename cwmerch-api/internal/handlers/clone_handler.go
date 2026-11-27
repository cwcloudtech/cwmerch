package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"cwmerch-api/internal/dbclone"
	"cwmerch-api/internal/middleware"
	"cwmerch-api/internal/models"
	"cwmerch-api/internal/store"
)

// CloneHandler is the "clone data" feature (WordPress administrators): copy
// the main database to another one (export) or a remote one into the main
// one (import), MySQL/MariaDB ↔ PostgreSQL.
type CloneHandler struct {
	clone *dbclone.Service
	audit auditor
}

func NewCloneHandler(c *dbclone.Service, audit *store.AuditStore) *CloneHandler {
	return &CloneHandler{clone: c, audit: newAuditor(audit)}
}

type clonePayload struct {
	Direction string `json:"direction"` // export | import
	// Conflict: overwrite (the source wins, default) or skip (keep the
	// destination's rows, only add the missing ones).
	Conflict string         `json:"conflict,omitempty"`
	Remote   dbclone.Remote `json:"remote"`
	// Confirm must be true to start (the destination's rows are overwritten).
	Confirm bool `json:"confirm"`
}

type cloneStatus struct {
	Main dbclone.Info `json:"main"`
	Job  *dbclone.Job `json:"job"`
}

// Status is GET /v1/admin/clone: the main database and the running/last job.
func (h *CloneHandler) Status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, cloneStatus{Main: h.clone.MainInfo(r.Context()), Job: h.clone.Current()})
}

// Job is GET /v1/admin/clone/job: the running/last job only (polling).
func (h *CloneHandler) Job(w http.ResponseWriter, r *http.Request) {
	j := h.clone.Current()
	if j == nil {
		writeError(w, http.StatusNotFound, "No clone has been run", "errors.cloneNotFound")
		return
	}
	writeJSON(w, http.StatusOK, j)
}

func direction(d string) string {
	if d == dbclone.Import {
		return d
	}
	return dbclone.Export
}

// Test is POST /v1/admin/clone/test: connects to the remote database and
// checks both sides are migrated.
func (h *CloneHandler) Test(w http.ResponseWriter, r *http.Request) {
	var p clonePayload
	if !decodeJSON(w, r, &p) {
		return
	}
	res, err := h.clone.Test(r.Context(), p.Remote, direction(p.Direction))
	if err != nil {
		writeCloneError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// Start is POST /v1/admin/clone: starts the copy (202, then poll /job).
func (h *CloneHandler) Start(w http.ResponseWriter, r *http.Request) {
	var p clonePayload
	if !decodeJSON(w, r, &p) {
		return
	}
	if !p.Confirm {
		writeError(w, http.StatusBadRequest, "Please confirm: the destination database's matching rows will be overwritten", "errors.cloneConfirmRequired")
		return
	}
	if p.Direction != dbclone.Export && p.Direction != dbclone.Import {
		writeError(w, http.StatusBadRequest, "The direction must be export or import", "errors.cloneInvalidDirection")
		return
	}
	uid, _ := middleware.UserIDFromContext(r.Context())
	by, _ := strconv.ParseUint(uid, 10, 64)
	job, err := h.clone.Start(r.Context(), p.Remote, p.Direction, p.Conflict, by)
	if err != nil {
		writeCloneError(w, err)
		return
	}
	h.audit.record(r, models.AuditDatabaseCloned, models.AuditSubjectSetting, "database", map[string]any{
		"direction": p.Direction, "conflict": job.Conflict, "remoteType": job.Target.Type, "remoteHost": p.Remote.Host, "remoteName": p.Remote.Name, "job": job.ID})
	writeJSON(w, http.StatusAccepted, job)
}

func writeCloneError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, dbclone.ErrRunning):
		writeError(w, http.StatusConflict, err.Error(), "errors.cloneRunning")
	case errors.Is(err, dbclone.ErrSameDB):
		writeError(w, http.StatusBadRequest, err.Error(), "errors.cloneSameDatabase")
	case errors.Is(err, dbclone.ErrNotReady):
		writeError(w, http.StatusConflict, err.Error(), "errors.cloneNotReady")
	default:
		// connection problems are the admin's input, not a server fault
		writeError(w, http.StatusBadRequest, err.Error(), "errors.cloneConnection")
	}
}
