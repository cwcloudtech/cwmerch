package middleware

import (
	"encoding/json"
	"net/http"

	"cwmerch-api/internal/utils"
)

func jsonError(w http.ResponseWriter, status int, message string) {
	jsonErrorCode(w, status, message, utils.EMPTY)
}

// jsonErrorCode writes {"message", "i18n_code"} like handlers.errorBody.
func jsonErrorCode(w http.ResponseWriter, status int, message, i18nCode string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body := map[string]string{"message": message}
	if utils.IsNotBlank(i18nCode) {
		body["i18n_code"] = i18nCode
	}
	_ = json.NewEncoder(w).Encode(body)
}
