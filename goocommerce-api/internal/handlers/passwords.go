package handlers

import "goocommerce-api/internal/wppass"

// passwordAlgo is how new passwords are hashed: "bcrypt" (WordPress >= 6.8
// format, the default) or "phpass" (readable by every WordPress version).
var passwordAlgo = wppass.AlgoWordPressBcrypt

// SetPasswordAlgo picks the hash format (GOOCOMMERCE_PASSWORD_HASH).
func SetPasswordAlgo(algo string) {
	if algo == wppass.AlgoPHPass {
		passwordAlgo = wppass.AlgoPHPass
		return
	}
	passwordAlgo = wppass.AlgoWordPressBcrypt
}

func hashPassword(p string) (string, error) { return wppass.Hash(p, passwordAlgo) }

func checkPassword(p, hash string) bool { return wppass.Check(p, hash) }
