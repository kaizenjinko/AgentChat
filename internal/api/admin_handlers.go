package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	if s.handleOptions(w, r) {
		return
	}
	st, err := s.Store.SystemStats()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	if s.handleOptions(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		users, err := s.Store.ListUsersWithStats()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"users": users})
	case http.MethodPost:
		raw, err := readBody(w, r)
		if err != nil {
			if err == errTooLarge {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "body too large"})
				return
			}
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
		role := asString(body["role"])
		if username == "" || password == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "username and password required"})
			return
		}
		if role == "" {
			role = "user"
		}
		u, err := s.Store.CreateUser(username, password, role)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				writeJSON(w, http.StatusConflict, map[string]any{"error": "username đã tồn tại"})
				return
			}
			if strings.Contains(err.Error(), "invalid role") ||
				strings.Contains(err.Error(), "username must be") ||
				strings.Contains(err.Error(), "password must be") {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"user": u})
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found", "path": r.URL.Path})
	}
}

func (s *Server) handleAdminUserByID(w http.ResponseWriter, r *http.Request) {
	if s.handleOptions(w, r) {
		return
	}
	raw := strings.TrimPrefix(r.URL.Path, "/api/admin/users/")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
		return
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
		return
	}
	switch r.Method {
	case http.MethodPatch:
		bodyRaw, err := readBody(w, r)
		if err != nil {
			if err == errTooLarge {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "body too large"})
				return
			}
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
			return
		}
		var body map[string]any
		if len(strings.TrimSpace(string(bodyRaw))) == 0 {
			body = map[string]any{}
		} else if err := json.Unmarshal(bodyRaw, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
			return
		}
		action := asString(body["action"])
		switch action {
		case "disable":
			me := s.currentUser(r)
			if me != nil && me.UserID == id {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "cannot disable yourself"})
				return
			}
			if err := s.Store.SetUserStatus(id, "disabled"); err != nil {
				if strings.Contains(err.Error(), "last admin") {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": "cannot disable last admin"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
				return
			}
		case "enable":
			if err := s.Store.SetUserStatus(id, "active"); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
				return
			}
		case "set-role":
			role := asString(body["role"])
			if role != "admin" && role != "user" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid role"})
				return
			}
			me := s.currentUser(r)
			if me != nil && me.UserID == id && role != "admin" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "cannot demote yourself"})
				return
			}
			if err := s.Store.SetUserRole(id, role); err != nil {
				if strings.Contains(err.Error(), "last admin") {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": "cannot demote last admin"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
				return
			}
		case "set-password":
			password := asString(body["password"])
			if len(password) < 8 {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "password must be at least 8 characters"})
				return
			}
			if err := s.Store.AdminSetPassword(id, password, true); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
				return
			}
		default:
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unknown action"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case http.MethodDelete:
		me := s.currentUser(r)
		if me != nil && me.UserID == id {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "cannot delete yourself"})
			return
		}
		if err := s.Store.DeleteUserCascade(id); err != nil {
			if strings.Contains(err.Error(), "last admin") {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "cannot delete last admin"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found", "path": r.URL.Path})
	}
}

func (s *Server) handleAdminAgents(w http.ResponseWriter, r *http.Request) {
	if s.handleOptions(w, r) {
		return
	}
	agents, err := s.Store.ListAgentsAll()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": agents})
}

func (s *Server) handleAdminRooms(w http.ResponseWriter, r *http.Request) {
	if s.handleOptions(w, r) {
		return
	}
	rooms, err := s.Store.ListRoomsAll()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rooms": rooms})
}
