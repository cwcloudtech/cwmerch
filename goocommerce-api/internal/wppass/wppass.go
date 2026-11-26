// Package wppass checks and creates WordPress password hashes, so a customer
// keeps one password whether they sign in on WordPress or on goocommerce.
//
// Recognised: WordPress 6.8+ "$wp$2y$..." (bcrypt over a base64 HMAC-SHA384
// pre-hash), plain bcrypt "$2y$"/"$2a$"/"$2b$", phpass portable "$P$"/"$H$"
// (WordPress < 6.8), and the legacy 32-hex MD5.
package wppass

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// Algorithms a new hash can be written with.
const (
	AlgoWordPressBcrypt = "bcrypt" // WordPress >= 6.8
	AlgoPHPass          = "phpass" // every WordPress version
)

const itoa64 = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

func wpPrehash(password string) []byte {
	mac := hmac.New(sha512.New384, []byte("wp-sha384"))
	mac.Write([]byte(password))
	return []byte(base64.StdEncoding.EncodeToString(mac.Sum(nil)))
}

func normalizeBcrypt(hash string) string {
	// Go's bcrypt reads $2a$/$2b$; PHP writes $2y$ (same algorithm).
	if strings.HasPrefix(hash, "$2y$") {
		return "$2a$" + hash[4:]
	}
	return hash
}

// Check reports whether password matches the stored WordPress hash.
func Check(password, hash string) bool {
	switch {
	case hash == "":
		return false
	case strings.HasPrefix(hash, "$wp$"):
		return bcrypt.CompareHashAndPassword([]byte(normalizeBcrypt(hash[3:])), wpPrehash(password)) == nil
	case strings.HasPrefix(hash, "$2y$"), strings.HasPrefix(hash, "$2a$"), strings.HasPrefix(hash, "$2b$"):
		return bcrypt.CompareHashAndPassword([]byte(normalizeBcrypt(hash)), []byte(password)) == nil
	case strings.HasPrefix(hash, "$P$"), strings.HasPrefix(hash, "$H$"):
		computed := phpassCrypt(password, hash)
		return computed != "" && subtle.ConstantTimeCompare([]byte(computed), []byte(hash)) == 1
	case len(hash) == 32:
		sum := md5.Sum([]byte(password))
		return subtle.ConstantTimeCompare([]byte(hex.EncodeToString(sum[:])), []byte(strings.ToLower(hash))) == 1
	}
	return false
}

// NeedsRehash reports a hash weaker than the one Hash(algo) writes.
func NeedsRehash(hash, algo string) bool {
	if algo == AlgoPHPass {
		return !strings.HasPrefix(hash, "$P$")
	}
	return !strings.HasPrefix(hash, "$wp$")
}

// Hash writes a new hash with algo (bcrypt = WordPress 6.8 format).
func Hash(password, algo string) (string, error) {
	if algo == AlgoPHPass {
		return phpassHash(password)
	}
	h, err := bcrypt.GenerateFromPassword(wpPrehash(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	s := string(h)
	if strings.HasPrefix(s, "$2a$") {
		s = "$2y$" + s[4:]
	}
	return "$wp" + s, nil
}

func encode64(input []byte, count int) string {
	var out strings.Builder
	i := 0
	for i < count {
		value := int(input[i])
		i++
		out.WriteByte(itoa64[value&0x3f])
		if i < count {
			value |= int(input[i]) << 8
		}
		out.WriteByte(itoa64[(value>>6)&0x3f])
		if i >= count {
			break
		}
		i++
		if i < count {
			value |= int(input[i]) << 16
		}
		out.WriteByte(itoa64[(value>>12)&0x3f])
		if i >= count {
			break
		}
		i++
		out.WriteByte(itoa64[(value>>18)&0x3f])
	}
	return out.String()
}

func phpassCrypt(password, setting string) string {
	if len(setting) < 12 {
		return ""
	}
	countLog2 := strings.IndexByte(itoa64, setting[3])
	if countLog2 < 7 || countLog2 > 30 {
		return ""
	}
	count := 1 << countLog2
	salt := setting[4:12]
	sum := md5.Sum([]byte(salt + password))
	hash := sum[:]
	for ; count > 0; count-- {
		s := md5.Sum(append(append([]byte{}, hash...), password...))
		hash = s[:]
	}
	return setting[:12] + encode64(hash, 16)
}

func phpassHash(password string) (string, error) {
	random := make([]byte, 6)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	// WordPress uses 8 as the iteration count log2.
	setting := "$P$" + string(itoa64[8+5]) + encode64(random, 6)
	return phpassCrypt(password, setting), nil
}
