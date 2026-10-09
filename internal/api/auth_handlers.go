package api

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"agentchat/internal/store"
)

const (
	sessionCookie = "ac_session"
	sessionTTL    = 168 * time.Hour
)

func (s *Server) setSessionCookie(w http.ResponseWriter, id string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   os.Getenv("TLS") == "1",
		MaxAge:   maxAge,
	})
}

func userPublic(u *store.User) map[string]any {
	if u == nil {
		return map[string]any{}
	}
	return map[string]any{
		"id":                   u.ID,
		"username":             u.Username,
		"role":                 u.Role,
		"status":               u.Status,
		"must_change_password": u.MustChangePassword,
	}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	raw, err := readBody(w, r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
		return
	}
	var body map[string]any
	if len(strings.TrimSpace(string(raw))) == 0 {
		body = map[string]any{}
	} else if err := json.Unmarshal(raw, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
		return
	}

	username := strings.TrimSpace(asString(body["username"]))
	password := asString(body["password"])
	if username == "" || password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "username and password required"})
		return
	}

	key := loginKey(r, username)
	if ok, retry := s.Limiter.Allow(key); !ok {
		if retry > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(retry))
		}
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": "too many attempts"})
		return
	}

	user, err := s.Store.GetUserByUsername(username)
	if err != nil || user.Status != "active" || !store.CheckPassword(user.PasswordHash, password) {
		if err != nil {
			store.CheckPassword(store.DummyHash(), password)
		}
		s.Limiter.Fail(key)
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid credentials"})
		return
	}

	s.Limiter.Reset(key)
	id, err := s.Store.CreateSession(user.ID, sessionTTL)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not create session"})
		return
	}
	s.Store.TouchLastLogin(user.ID)
	s.setSessionCookie(w, id, int(sessionTTL/time.Second))
	writeJSON(w, http.StatusOK, map[string]any{"user": userPublic(user)})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		s.Store.DeleteSession(c.Value)
	}
	s.setSessionCookie(w, "", -1)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	id := s.currentUser(r)
	if id == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	u, err := s.Store.GetUserByID(id.UserID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"user": map[string]any{
			"id":       id.UserID,
			"username": id.Username,
			"role":     id.Role,
		}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": userPublic(u)})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	id := s.currentUser(r)
	if id == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}

	raw, err := readBody(w, r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
		return
	}
	var body map[string]any
	if len(strings.TrimSpace(string(raw))) == 0 {
		body = map[string]any{}
	} else if err := json.Unmarshal(raw, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
		return
	}

	oldPassword := asString(body["old_password"])
	newPassword := asString(body["new_password"])
	if len(newPassword) < 8 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "password too short"})
		return
	}

	user, err := s.Store.GetUserByID(id.UserID)
	if err != nil || !store.CheckPassword(user.PasswordHash, oldPassword) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid credentials"})
		return
	}

	if err := s.Store.AdminSetPassword(id.UserID, newPassword, false); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	sid, err := s.Store.CreateSession(id.UserID, sessionTTL)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not create session"})
		return
	}
	s.setSessionCookie(w, sid, int(sessionTTL/time.Second))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
