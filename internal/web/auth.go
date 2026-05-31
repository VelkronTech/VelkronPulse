package web

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.authenticateRequest(r) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("WWW-Authenticate", `Bearer realm="Velkron Pulse"`)
		writeError(w, http.StatusUnauthorized, "Unauthorized")
	})
}

func (s *Server) authenticateRequest(r *http.Request) bool {
	token := extractToken(r)
	if token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.token)) == 1
}

func extractToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(auth[len("Bearer "):])
	}
	if cookie, err := r.Cookie("pulse_token"); err == nil {
		return cookie.Value
	}
	if q := r.URL.Query().Get("token"); q != "" {
		return q
	}
	return ""
}
