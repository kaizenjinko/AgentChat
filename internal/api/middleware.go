package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
)

type Identity struct {
	UserID   int64
	Username string
	Role     string
	Via      string
}

type ctxKey int

const identityKey ctxKey = 0

var errAuthRequired = errors.New("auth required")
var errForbidden = errors.New("forbidden")

func IdentityFrom(ctx context.Context) (*Identity, bool) {
	id, ok := ctx.Value(identityKey).(*Identity)
	return id, ok
}

func withIdentity(r *http.Request, id *Identity) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), identityKey, id))
}

func (s *Server) resolveIdentity(r *http.Request) (*Identity, error) {
	if c, err := r.Cookie("ac_session"); err == nil && c.Value != "" {
		if u, err := s.Store.GetSession(c.Value); err == nil && u != nil {
			return &Identity{UserID: u.ID, Username: u.Username, Role: u.Role, Via: "cookie"}, nil
		}
	}
	if key := r.Header.Get("X-API-Key"); key != "" {
		if u, err := s.Store.UserByAPIKey(key); err == nil && u != nil {
			return &Identity{UserID: u.ID, Username: u.Username, Role: u.Role, Via: "apikey"}, nil
		}
	}
	return nil, errAuthRequired
}

func (s *Server) authRequired() bool {
	return os.Getenv("AUTH_REQUIRED") != "0"
}

func (s *Server) currentUser(r *http.Request) *Identity {
	id, ok := IdentityFrom(r.Context())
	if !ok {
		return nil
	}
	return id
}

// isStateChanging reports whether the method can mutate server state.
func isStateChanging(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete:
		return true
	default:
		return false
	}
}

// sameOrigin reports whether an Origin/Referer header matches the request host.
// Missing headers (non-browser clients) are treated as same-origin so that
// API-key and CLI clients keep working; browsers always send Origin on
// cross-site mutations and SameSite=Lax covers the rest.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = r.Header.Get("Referer")
	}
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

// requireUser wraps a handler, enforcing authentication (and CSRF for
// cookie-authenticated mutations).
func (s *Server) requireUser(next handlerFunc) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.handleOptions(w, r) {
			return
		}
		id, err := s.resolveIdentity(r)
		if err != nil {
			if !s.authRequired() {
				id = &Identity{UserID: 0, Username: "local", Role: "admin", Via: "disabled"}
			} else {
				writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
				return
			}
		}
		if id.Via == "cookie" && isStateChanging(r.Method) && !sameOrigin(r) {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "cross-origin request blocked"})
			return
		}
		next(w, withIdentity(r, id))
	}
}

// requireAdmin wraps a handler, enforcing an authenticated admin (and CSRF for
// cookie-authenticated mutations).
func (s *Server) requireAdmin(next handlerFunc) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.handleOptions(w, r) {
			return
		}
		id, err := s.resolveIdentity(r)
		if err != nil {
			if !s.authRequired() {
				if os.Getenv("DEV_ALLOW_ADMIN") != "1" {
					writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
					return
				}
				id = &Identity{UserID: 0, Username: "local", Role: "admin", Via: "disabled"}
			} else {
				writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
				return
			}
		}
		if id.Role != "admin" {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
			return
		}
		if id.Via == "cookie" && isStateChanging(r.Method) && !sameOrigin(r) {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "cross-origin request blocked"})
			return
		}
		next(w, withIdentity(r, id))
	}
}
