package wppass

import "testing"

// Vectors produced by PHP itself: WordPress 6.8+'s wp_hash_password, and a
// phpass hash as WordPress < 6.8 wrote it (from the phpass test suite).
func TestCheckWordPressHashes(t *testing.T) {
	cases := []struct{ password, hash string }{
		{"S3cret!pw", `$wp$2y$12$e3FenEMIT82PlmTNt15t8OoXmGfzuNXM2.oL2jECXxnIY9ytG2XF.`},
		{"test12345", "$P$9IQRaTwmfeRo7ud9Fh4E2PdI0S3r.L0"},
	}
	for _, c := range cases {
		if !Check(c.password, c.hash) {
			t.Errorf("Check(%q, %q) = false", c.password, c.hash)
		}
		if Check(c.password+"x", c.hash) {
			t.Errorf("wrong password accepted for %q", c.hash)
		}
	}
}

func TestHashRoundTrip(t *testing.T) {
	for _, algo := range []string{AlgoWordPressBcrypt, AlgoPHPass} {
		h, err := Hash("Pa55word!", algo)
		if err != nil {
			t.Fatal(err)
		}
		if !Check("Pa55word!", h) || Check("nope", h) {
			t.Errorf("%s round trip failed: %s", algo, h)
		}
	}
}
