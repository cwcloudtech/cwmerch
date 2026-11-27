package secrets

import "testing"

func TestSealOpen(t *testing.T) {
	s, err := New("a passphrase that is not base64")
	if err != nil || !s.Configured() || !s.Derived() {
		t.Fatal("sealer not configured")
	}
	sealed, err := s.Seal("sk_test_123")
	if err != nil || sealed == "sk_test_123" {
		t.Fatalf("seal: %v %q", err, sealed)
	}
	if plain, err := s.Open(sealed); err != nil || plain != "sk_test_123" {
		t.Fatalf("open: %v %q", err, plain)
	}
	other, _ := New("another key")
	if _, err := other.Open(sealed); err == nil {
		t.Fatal("a different key must not open the secret")
	}
	none, _ := New("")
	if _, err := none.Seal("x"); err != ErrNoKey {
		t.Fatal("sealing without a key must fail")
	}
}
