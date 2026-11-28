package rootuser

import (
	"context"
	"strings"
	"testing"

	"cwmerch-api/internal/models"
	"cwmerch-api/internal/store"
)

type fakeUsers struct{ users []models.User }

func (f *fakeUsers) FindByUserLogin(_ context.Context, login string) (models.User, error) {
	for _, u := range f.users {
		if u.Login == login {
			return u, nil
		}
	}
	return models.User{}, store.ErrNotFound
}

func (f *fakeUsers) FindByEmail(_ context.Context, email string) (models.User, error) {
	for _, u := range f.users {
		if strings.EqualFold(u.Email, email) {
			return u, nil
		}
	}
	return models.User{}, store.ErrNotFound
}

func (f *fakeUsers) CreateAdministrator(_ context.Context, login, email, hash string) (models.User, error) {
	if hash == "" || hash == "secret" {
		panic("password not hashed")
	}
	u := models.User{ID: "9", Login: login, Email: email}
	f.users = append(f.users, u)
	return u, nil
}

func TestCheck(t *testing.T) {
	cfg := Config{Username: "root", Email: "root@example.com", Password: "secret"}
	cases := []struct {
		name     string
		cfg      Config
		existing []models.User
		want     Outcome
		created  bool
	}{
		{"missing password", Config{Username: "root", Email: "root@example.com"}, nil, Disabled, false},
		{"missing email", Config{Username: "root", Password: "x"}, nil, Disabled, false},
		{"missing username", Config{Email: "root@example.com", Password: "x"}, nil, Disabled, false},
		{"created", cfg, nil, Created, true},
		{"present (email case)", cfg, []models.User{{Login: "root", Email: "Root@Example.com"}}, Present, false},
		{"login with another email", cfg, []models.User{{Login: "root", Email: "other@example.com"}}, Conflict, false},
		{"email with another login", cfg, []models.User{{Login: "bob", Email: "root@example.com"}}, Conflict, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			users := &fakeUsers{users: c.existing}
			got, _ := New(c.cfg, users, nil, nil, nil).Check(context.Background())
			if got != c.want {
				t.Fatalf("outcome %q, want %q", got, c.want)
			}
			if created := len(users.users) > len(c.existing); created != c.created {
				t.Fatalf("created = %v, want %v", created, c.created)
			}
		})
	}
}
