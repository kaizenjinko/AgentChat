package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) handleKeys(w http.ResponseWriter, r *http.Request) {
	if s.handleOptions(w, r) {
		return
	}
	id := s.currentUser(r)
	if id == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		keys, err := s.Store.ListAPIKeys(id.UserID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"keys": keys})
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
		name := asString(body["name"])
		agentName := asString(body["agent_name"])
		plain, prefix, err := s.Store.CreateAPIKey(id.UserID, name, agentName)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"key": map[string]any{
				"plain":      plain,
				"prefix":     prefix,
				"name":       name,
				"agent_name": agentName,
			},
			"plain":  plain,
			"prefix": prefix,
		})
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found", "path": r.URL.Path})
	}
}

func (s *Server) handleKeyByID(w http.ResponseWriter, r *http.Request) {
	if s.handleOptions(w, r) {
		return
	}
	id := s.currentUser(r)
	if id == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found", "path": r.URL.Path})
		return
	}
	raw := strings.TrimPrefix(r.URL.Path, "/api/keys/")
	raw = strings.TrimSpace(raw)
	keyID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
		return
	}
	n, err := s.Store.RevokeAPIKey(id.UserID, keyID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if n == 0 {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": n})
}

func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	if s.handleOptions(w, r) {
		return
	}
	id := s.currentUser(r)
	if id == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found", "path": r.URL.Path})
		return
	}
	agents, err := s.Store.ListAgents(id.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": agents})
}

func (s *Server) handleUserStats(w http.ResponseWriter, r *http.Request) {
	if s.handleOptions(w, r) {
		return
	}
	id := s.currentUser(r)
	if id == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	st, err := s.Store.UserStats(id.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, st)
}
