package store

import (
	"database/sql"
	"errors"
	"strconv"

	"github.com/google/uuid"

	"cwmerch-api/internal/utils"
)

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("not found")

// ErrDuplicateEmail is returned when an email (or login) is already taken.
var ErrDuplicateEmail = errors.New("duplicate email")

// ErrDuplicateLogin: the user_login is already taken.
var ErrDuplicateLogin = errors.New("duplicate login")

func newID() string { return uuid.NewString() }

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// userIDArg turns cwmerch's string user id into WordPress' numeric ID
// (0 when blank or not a number).
func userIDArg(id string) uint64 {
	if utils.IsBlank(id) {
		return 0
	}
	n, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func nullUserID(id string) any {
	n := userIDArg(id)
	if n == 0 {
		return nil
	}
	return n
}

func idString(n uint64) string {
	if n == 0 {
		return ""
	}
	return strconv.FormatUint(n, 10)
}
