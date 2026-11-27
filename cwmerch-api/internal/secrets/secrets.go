// Package secrets seals credentials (payment providers' keys) before they are
// stored in the database, with AES-256-GCM under CWMERCH_SECRET_KEY. A
// database dump alone then never yields a usable Stripe/Konnect/Flouci key.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
)

// prefix marks a sealed value ("v1:" + base64(nonce|ciphertext)).
const prefix = "sealed:v1:"

// ErrNoKey is returned when sealing without a configured key.
var ErrNoKey = errors.New("no CWMERCH_SECRET_KEY is configured: secrets cannot be stored")

// Sealer encrypts and decrypts secrets.
type Sealer struct {
	aead    cipher.AEAD
	derived bool
}

// New builds a sealer from a base64 32-byte key; any other non-empty value is
// hashed with SHA-256 (works, but a random key is stronger). Blank = disabled.
func New(key string) (*Sealer, error) {
	key = strings.TrimSpace(key)
	if key == "" || key == "changeit" {
		return &Sealer{}, nil
	}
	raw, err := base64.StdEncoding.DecodeString(key)
	derived := false
	if err != nil || len(raw) != 32 {
		sum := sha256.Sum256([]byte(key))
		raw, derived = sum[:], true
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Sealer{aead: aead, derived: derived}, nil
}

// Configured reports whether secrets can be sealed.
func (s *Sealer) Configured() bool { return s != nil && s.aead != nil }

// Derived reports a key that was hashed rather than decoded.
func (s *Sealer) Derived() bool { return s != nil && s.derived }

// Seal encrypts a secret ("" stays "").
func (s *Sealer) Seal(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	if !s.Configured() {
		return "", ErrNoKey
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := s.aead.Seal(nonce, nonce, []byte(plain), nil)
	return prefix + base64.StdEncoding.EncodeToString(out), nil
}

// Open decrypts a sealed secret; an unsealed value is returned as is.
func (s *Sealer) Open(sealed string) (string, error) {
	if !strings.HasPrefix(sealed, prefix) {
		return sealed, nil
	}
	if !s.Configured() {
		return "", ErrNoKey
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(sealed, prefix))
	if err != nil {
		return "", err
	}
	n := s.aead.NonceSize()
	if len(raw) < n {
		return "", errors.New("sealed secret too short")
	}
	plain, err := s.aead.Open(nil, raw[:n], raw[n:], nil)
	if err != nil {
		return "", errors.New("the secret cannot be opened with this CWMERCH_SECRET_KEY")
	}
	return string(plain), nil
}
