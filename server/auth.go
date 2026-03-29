package server

import (
	"crypto/subtle"
	"net/http"
	"winctl/config"
)

func basicAuth(cfg *config.Config, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok ||
			subtle.ConstantTimeCompare([]byte(user), []byte(cfg.Username)) != 1 ||
			subtle.ConstantTimeCompare([]byte(pass), []byte(cfg.Password())) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="winctl"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
