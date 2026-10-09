package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"agentchat/internal/filter"
	"agentchat/internal/hub"
	"agentchat/internal/store"
)

// Server bundles dependencies for the HTTP handlers.
type Server struct {
	Store *store.Store
	Hub   *hub.Hub
	UIDir string
}

// New creates a Server. uiDir is the directory holding index.html and GUIDE.md.
func New(st *store.Store, h *hub.Hub, uiDir string) *Server {
	return &Server{Store: st, Hub: h, UIDir: uiDir}
}

// Register wires all routes onto the given mux.
func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("/health", s.method(http.MethodGet, s.handleHealth))
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/guide", s.method(http.MethodGet, s.handleGuide))
	mux.HandleFunc("/api/rooms", s.handleRooms)
	mux.HandleFunc("/api/rooms/", s.handleRooms)
	mux.HandleFunc("/api/messages", s.handleMessages)
	mux.HandleFunc("/api/stream", s.method(http.MethodGet, s.handleStream))
}

type handlerFunc func(http.ResponseWriter, *http.Request)

// method wraps a handler, enforcing an allowed method and handling OPTIONS/CORS.
func (s *Server) method(m string, h handlerFunc) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.handleOptions(w, r) {
			return
		}
		if r.Method != m {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found", "path": r.URL.Path})
			return
		}
		h(w, r)
	}
}

// handleOptions replies to OPTIONS preflight and returns true when handled.
// It also sets CORS headers on every response.
func (s *Server) handleOptions(w http.ResponseWriter, r *http.Request) bool {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Headers", "*")
	h.Set("Access-Control-Allow-Methods", "GET,POST,DELETE,OPTIONS")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.handleOptions(w, r)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"time": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if s.handleOptions(w, r) {
		return
	}
	if r.URL.Path != "/" {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found", "path": r.URL.Path})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found", "path": r.URL.Path})
		return
	}
	htmlPath := filepath.Join(s.UIDir, "index.html")
	data, err := os.ReadFile(htmlPath)
	if err != nil {
		data = []byte("<h1>index.html missing</h1>")
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

func (s *Server) handleGuide(w http.ResponseWriter, r *http.Request) {
	s.handleOptions(w, r)
	guidePath := filepath.Join(s.UIDir, "GUIDE.md")
	raw, err := os.ReadFile(guidePath)
	if err != nil || len(raw) == 0 {
		raw = []byte("# Guide missing")
	}
	md := string(raw)

	if strings.Contains(r.Header.Get("Accept"), "text/html") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(guideHTML(md)))
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Write(raw)
}

// guideHTML renders GUIDE.md as an HTML page (marked + DOMPurify + KaTeX).
func guideHTML(md string) string {
	mdJSON, _ := json.Marshal(md)
	return `<!DOCTYPE html><html><head><meta charset="utf-8"><title>AgentChat Guide</title>` +
		`<script src="https://cdn.jsdelivr.net/npm/marked/marked.min.js"></script>` +
		`<script src="https://cdn.jsdelivr.net/npm/dompurify@3/dist/purify.min.js"></script>` +
		`<script src="https://cdn.jsdelivr.net/npm/katex@0.16/dist/katex.min.js"></script>` +
		`<script src="https://cdn.jsdelivr.net/npm/marked-katex-extension@5/lib/index.umd.js"></script>` +
		`<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/katex@0.16/dist/katex.min.css">` +
		`<style>body{max-width:820px;margin:40px auto;padding:0 20px;font-family:ui-sans-serif,system-ui,sans-serif;background:#0f1117;color:#e6e8ee;line-height:1.6}` +
		`pre{background:#0b0d13;padding:12px;border-radius:8px;overflow-x:auto}code{background:#0b0d13;padding:2px 5px;border-radius:4px}` +
		`pre code{padding:0;background:none}a{color:#4f8cff}h1,h2,h3{border-bottom:1px solid #262b38;padding-bottom:6px}` +
		`.katex-display{overflow-x:auto;overflow-y:hidden}</style></head>` +
		`<body><div id="c"></div><script>` +
		`const md=` + string(mdJSON) + `;` +
		`if(window.marked&&window.markedKatex&&window.katex){marked.use(markedKatex({throwOnError:false,output:"html"}));}` +
		`document.getElementById("c").innerHTML=(window.marked&&window.DOMPurify)?DOMPurify.sanitize(marked.parse(md)):` +
		`"<pre>"+md.replace(/[&<>]/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;"}[c]))+"</pre>";` +
		`<\/script></body></html>`
}

func (s *Server) handleRooms(w http.ResponseWriter, r *http.Request) {
	if s.handleOptions(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		rooms, err := s.Store.ListRooms()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"rooms": rooms})
	case http.MethodDelete:
		var room string
		if strings.HasPrefix(r.URL.Path, "/api/rooms/") {
			room = r.URL.Path[len("/api/rooms/"):]
			if dec, err := decodePath(room); err == nil {
				room = dec
			}
		} else {
			room = r.URL.Query().Get("room")
		}
		room = strings.TrimSpace(room)
		if room == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "room required"})
			return
		}
		n, err := s.Store.DeleteRoom(room)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		s.Hub.NotifyRoomDeleted(room)
		writeJSON(w, http.StatusOK, map[string]any{"room": room, "deleted": n})
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found", "path": r.URL.Path})
	}
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	if s.handleOptions(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		params := r.URL.Query()
		res, err := s.Store.Query(params)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"filters":  paramsToMap(params),
			"count":    len(res.Messages),
			"limit":    res.Limit,
			"offset":   res.Offset,
			"order":    res.Order,
			"messages": res.Messages,
		})
	case http.MethodPost:
		s.handlePostMessage(w, r)
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found", "path": r.URL.Path})
	}
}

func (s *Server) handlePostMessage(w http.ResponseWriter, r *http.Request) {
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

	agent := asString(body["agent"])
	if agent == "" {
		agent = r.Header.Get("X-Agent-Name")
	}
	if agent == "" {
		agent = "anonymous"
	}
	agent = strings.TrimSpace(agent)

	room := strings.TrimSpace(asString(body["room"]))
	if room == "" {
		room = "general"
	}

	content := asString(body["content"])

	var replyTo *int64
	if v, ok := body["reply_to"]; ok && v != nil {
		n, ok := asNumber(v)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "reply_to must be a number"})
			return
		}
		replyTo = &n
	}

	if strings.TrimSpace(content) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "content required"})
		return
	}

	mentions := filter.ParseMentions(content)
	msg, err := s.Store.Insert(agent, room, content, mentions, replyTo)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.Hub.Broadcast(room, msg)
	writeJSON(w, http.StatusCreated, msg)
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	room := r.URL.Query().Get("room")
	if room == "" {
		room = "*"
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	client := s.Hub.Add(room)
	s.Hub.Stream(w, r, client)
}

// --- helpers ---

var errTooLarge = errBody("body too large")

type errBody string

func (e errBody) Error() string { return string(e) }

func readBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, 2*1024*1024)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, errTooLarge
	}
	return data, nil
}

func writeJSON(w http.ResponseWriter, code int, obj any) {
	body, err := json.Marshal(obj)
	if err != nil {
		http.Error(w, `{"error":"encode error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET,POST,DELETE,OPTIONS")
	w.WriteHeader(code)
	w.Write(body)
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asNumber(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return i, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		if err != nil {
			return 0, false
		}
		return int64(f), true
	default:
		return 0, false
	}
}

func decodePath(s string) (string, error) {
	return url.PathUnescape(s)
}

// paramsToMap flattens url.Values to a string map, last value wins.
func paramsToMap(v url.Values) map[string]string {
	out := make(map[string]string, len(v))
	for k, vals := range v {
		if len(vals) > 0 {
			out[k] = vals[len(vals)-1]
		}
	}
	return out
}
