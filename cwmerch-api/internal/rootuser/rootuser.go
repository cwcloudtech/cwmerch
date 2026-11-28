// Package rootuser keeps the root administrator of CWMERCH_ROOT_USERNAME /
// CWMERCH_ROOT_EMAIL / CWMERCH_ROOT_PASSWORD: a background job (hourly by
// default, CWMERCH_ROOT_USER_CHECK cron expression) checks that a WordPress
// user with that user_login and that email exists, and creates it as an
// administrator when neither exists.
//
// It never modifies an existing account: a password changed since, or a login
// or email already used by someone else, is left alone (a warning is logged).
// Nothing runs unless the three variables are set.
package rootuser

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/robfig/cron/v3"

	"cwmerch-api/internal/models"
	"cwmerch-api/internal/store"
	"cwmerch-api/internal/wppass"
)

// DefaultSchedule is the check's default cron expression (every hour).
const DefaultSchedule = "@hourly"

// Config is the root account wanted, and when to check it.
type Config struct {
	Username, Email, Password string
	// Schedule is a cron expression (5 fields or a descriptor such as
	// "@hourly", "@every 30m"); empty = DefaultSchedule.
	Schedule string
	// PasswordAlgo: wppass.AlgoWordPressBcrypt (default) or wppass.AlgoPHPass.
	PasswordAlgo string
}

// Enabled reports whether the three variables are set.
func (c Config) Enabled() bool {
	return strings.TrimSpace(c.Username) != "" && strings.TrimSpace(c.Email) != "" && c.Password != ""
}

// Users is the part of the user store the job uses.
type Users interface {
	FindByUserLogin(ctx context.Context, login string) (models.User, error)
	FindByEmail(ctx context.Context, email string) (models.User, error)
	CreateAdministrator(ctx context.Context, login, email, passwordHash string) (models.User, error)
}

// Job checks and creates the root account.
type Job struct {
	cfg    Config
	users  Users
	audit  *store.AuditStore
	log    *slog.Logger
	notify func(string)
}

func New(cfg Config, users Users, audit *store.AuditStore, log *slog.Logger, notify func(string)) *Job {
	if log == nil {
		log = slog.Default()
	}
	if notify == nil {
		notify = func(string) {}
	}
	return &Job{cfg: cfg, users: users, audit: audit, log: log, notify: notify}
}

// Outcome of one check.
type Outcome string

const (
	Disabled Outcome = "disabled" // a variable is missing
	Present  Outcome = "present"  // the user exists with that login and email
	Created  Outcome = "created"
	Conflict Outcome = "conflict" // the login or email belongs to another account
)

// Check runs one check.
func (j *Job) Check(ctx context.Context) (Outcome, error) {
	if !j.cfg.Enabled() {
		return Disabled, nil
	}
	login, email := strings.TrimSpace(j.cfg.Username), strings.TrimSpace(j.cfg.Email)

	byLogin, err := j.users.FindByUserLogin(ctx, login)
	switch {
	case err == nil:
		if strings.EqualFold(strings.TrimSpace(byLogin.Email), email) {
			return Present, nil
		}
		return Conflict, fmt.Errorf("the user %q exists with another email (%s)", login, byLogin.Email)
	case !errors.Is(err, store.ErrNotFound):
		return "", err
	}
	if byEmail, err := j.users.FindByEmail(ctx, email); err == nil {
		return Conflict, fmt.Errorf("the email %s belongs to another user (%q)", email, byEmail.Login)
	} else if !errors.Is(err, store.ErrNotFound) {
		return "", err
	}

	hash, err := wppass.Hash(j.cfg.Password, j.cfg.PasswordAlgo)
	if err != nil {
		return "", err
	}
	u, err := j.users.CreateAdministrator(ctx, login, email, hash)
	if err != nil {
		if errors.Is(err, store.ErrDuplicateEmail) || errors.Is(err, store.ErrDuplicateLogin) {
			// Created meanwhile (another instance): checked again next run.
			return Conflict, err
		}
		return "", err
	}
	if j.audit != nil {
		j.audit.Record(ctx, store.AuditEvent{Action: models.AuditRootUserCreated, SubjectType: "user", SubjectID: u.ID,
			Detail: map[string]any{"username": login, "email": email}})
	}
	return Created, nil
}

func (j *Job) run(ctx context.Context) {
	outcome, err := j.Check(ctx)
	switch {
	case err != nil && outcome == Conflict:
		j.log.Warn("root user not created", "username", j.cfg.Username, "reason", err)
	case err != nil:
		j.log.Error("root user check failed", "error", err)
	case outcome == Created:
		j.log.Info("root user created", "username", j.cfg.Username, "email", j.cfg.Email)
		j.notify(fmt.Sprintf("[ROOT] :key: the root administrator %q (%s) was missing and has been created", j.cfg.Username, j.cfg.Email))
	}
}

// Start checks now, then on the schedule until ctx is done. It returns at
// once (the job runs in the background); nothing is scheduled when a variable
// is missing.
func (j *Job) Start(ctx context.Context) {
	if !j.cfg.Enabled() {
		j.log.Info("no root user to check (CWMERCH_ROOT_USERNAME, CWMERCH_ROOT_EMAIL and CWMERCH_ROOT_PASSWORD are not all set)")
		return
	}
	spec := strings.TrimSpace(j.cfg.Schedule)
	if spec == "" {
		spec = DefaultSchedule
	}
	c := cron.New()
	if _, err := c.AddFunc(spec, func() { j.run(ctx) }); err != nil {
		j.log.Error("invalid CWMERCH_ROOT_USER_CHECK cron expression, checking hourly", "spec", spec, "error", err)
		spec = DefaultSchedule
		_, _ = c.AddFunc(spec, func() { j.run(ctx) })
	}
	j.log.Info("root user check scheduled", "username", j.cfg.Username, "schedule", spec)
	go j.run(ctx)
	c.Start()
	go func() {
		<-ctx.Done()
		c.Stop()
	}()
}
