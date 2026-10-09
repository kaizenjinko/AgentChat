package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
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
	Store   *store.Store
	Hub     *hub.Hub
	UIDir   string
	Limiter *loginLimiter
}

// New creates a Server. uiDir is the directory holding index.html and GUIDE.md.
func New(st *store.Store, h *hub.Hub, uiDir string) *Server {
	return &Server{Store: st, Hub: h, UIDir: uiDir, Limiter: NewLoginLimiter()}
}

// Register wires all routes onto the given mux.
func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("/health", s.method(http.MethodGet, s.handleHealth))
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/guide", s.method(http.MethodGet, s.handleGuide))
	mux.HandleFunc("/api/rooms", s.requireUser(s.handleRooms))
	mux.HandleFunc("/api/rooms/", s.requireUser(s.handleRooms))
	mux.HandleFunc("/api/messages", s.requireUser(s.handleMessages))
	mux.HandleFunc("/api/stream", s.method(http.MethodGet, s.requireUser(s.handleStream)))
	mux.HandleFunc("/api/auth/login", s.method(http.MethodPost, s.handleLogin))
	mux.HandleFunc("/api/auth/logout", s.method(http.MethodPost, s.handleLogout))
	mux.HandleFunc("/api/auth/me", s.method(http.MethodGet, s.requireUser(s.handleMe)))
	mux.HandleFunc("/api/auth/change-password", s.method(http.MethodPost, s.requireUser(s.handleChangePassword)))
	mux.HandleFunc("/api/keys", s.requireUser(s.handleKeys))
	mux.HandleFunc("/api/keys/", s.requireUser(s.handleKeyByID))
	mux.HandleFunc("/api/agents", s.method(http.MethodGet, s.requireUser(s.handleAgents)))
	mux.HandleFunc("/api/stats", s.method(http.MethodGet, s.requireUser(s.handleUserStats)))
	mux.HandleFunc("/api/admin/stats", s.method(http.MethodGet, s.requireAdmin(s.handleAdminStats)))
	mux.HandleFunc("/api/admin/users", s.requireAdmin(s.handleAdminUsers))
	mux.HandleFunc("/api/admin/users/", s.requireAdmin(s.handleAdminUserByID))
	mux.HandleFunc("/api/admin/agents", s.method(http.MethodGet, s.requireAdmin(s.handleAdminAgents)))
	mux.HandleFunc("/api/admin/rooms", s.method(http.MethodGet, s.requireAdmin(s.handleAdminRooms)))
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
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found", "path": r.URL.Path})
		return
	}
	if r.URL.Path == "/" {
		http.Redirect(w, r, "/login.html", http.StatusFound)
		return
	}
	if !s.serveStatic(w, r) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found", "path": r.URL.Path})
	}
}

// serveStatic serves the whitelisted UI files (HTML pages and assets/) from
// UIDir. It returns false when the path is not an allowed static asset.
func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request) bool {
	rel := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if rel == "" || rel == "." {
		return false
	}
	allow := rel == "login.html" || rel == "app.html" ||
		rel == "index.html" || rel == "favicon.ico" ||
		strings.HasPrefix(rel, "assets/")
	if !allow {
		return false
	}
	abs := filepath.Join(s.UIDir, filepath.FromSlash(rel))
	info, err := os.Stat(abs)
	if err != nil || info.IsDir() {
		return false
	}
	if r.URL.Query().Get("v") == "" {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeFile(w, r, abs)
	return true
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

// guideHTML renders GUIDE.md as a polished HTML page (marked + DOMPurify + KaTeX).
func guideHTML(md string) string {
	mdJSON, _ := json.Marshal(md)
	safeJSON := strings.ReplaceAll(string(mdJSON), "</script", "<\\/script")
	return guideHead + `<body>` +
		`<header class="topbar">` +
		`<a class="brand" href="/">` +
		`<span class="logo">AC</span>` +
		`<span class="brand-text"><b>AgentChat</b><i>Hướng dẫn &amp; API cho agent</i></span>` +
		`</a>` +
		`<div class="top-actions">` +
		`<a class="btn" href="/app.html">Mở ứng dụng</a>` +
		`<button class="btn icon" id="theme-toggle" type="button" title="Chế độ sáng/tối" aria-label="Chế độ sáng/tối">` +
		`<svg viewBox="0 0 24 24" width="17" height="17" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"/></svg>` +
		`</button>` +
		`</div>` +
		`</header>` +
		`<main class="wrap"><article class="doc" id="c"></article></main>` +
		`<footer class="foot">AgentChat · tài liệu tự động từ <code>GUIDE.md</code></footer>` +
		`<script>` +
		`const md=` + safeJSON + `;` +
		`function render(){` +
		`if(window.marked&&window.markedKatex&&window.katex){try{marked.use(markedKatex({throwOnError:false,output:"html"}));}catch(e){}}` +
		`const el=document.getElementById("c");` +
		`if(window.marked&&window.DOMPurify){el.innerHTML=DOMPurify.sanitize(marked.parse(md));}` +
		`else{const p=document.createElement("pre");p.textContent=md;el.appendChild(p);}` +
		`el.querySelectorAll("a[href^='http']").forEach(a=>{a.target="_blank";a.rel="noopener noreferrer";});` +
		`el.querySelectorAll("table").forEach(t=>{if(t.parentElement&&t.parentElement.classList.contains("table-scroll"))return;const w=document.createElement("div");w.className="table-scroll";t.parentNode.insertBefore(w,t);w.appendChild(t);});` +
		`}` +
		`if(document.readyState==="loading"){document.addEventListener("DOMContentLoaded",render);}else{render();}` +
		`const tg=document.getElementById("theme-toggle");` +
		`if(tg){tg.addEventListener("click",()=>{const next=document.documentElement.dataset.theme==="dark"?"light":"dark";document.documentElement.dataset.theme=next;try{localStorage.setItem("theme",next);}catch(e){}});}` +
		`</script></body></html>`
}

const guideHead = `<!DOCTYPE html><html lang="vi"><head><meta charset="utf-8">` +
	`<meta name="viewport" content="width=device-width, initial-scale=1">` +
	`<title>AgentChat · Hướng dẫn</title>` +
	`<link rel="icon" href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'%3E%3Crect width='32' height='32' rx='8' fill='%231f2937'/%3E%3Ctext x='16' y='21' font-family='Inter,sans-serif' font-size='13' font-weight='700' fill='%23fff' text-anchor='middle'%3EAC%3C/text%3E%3C/svg%3E">` +
	`<script>try{const t=localStorage.getItem('theme')||(matchMedia('(prefers-color-scheme:dark)').matches?'dark':'light');document.documentElement.dataset.theme=t;}catch(e){}</script>` +
	`<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/katex@0.16/dist/katex.min.css">` +
	`<script defer src="https://cdn.jsdelivr.net/npm/marked/marked.min.js"></script>` +
	`<script defer src="https://cdn.jsdelivr.net/npm/dompurify@3/dist/purify.min.js"></script>` +
	`<script defer src="https://cdn.jsdelivr.net/npm/katex@0.16/dist/katex.min.js"></script>` +
	`<script defer src="https://cdn.jsdelivr.net/npm/marked-katex-extension@5/lib/index.umd.js"></script>` +
	`<style>` +
	`:root{--bg:#eef1ec;--surface:#ffffff;--surface-2:#f4f5f3;--border:#e7e9e6;--text:#14161a;--muted:#8b9089;--accent:#1f2937;--accent-soft:#eef2ff;--code-bg:#f4f5f3;--shadow:0 24px 60px rgba(20,22,26,.10),0 2px 8px rgba(20,22,26,.05);--r-md:12px;--r-lg:20px;--font:Inter,ui-sans-serif,system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;--font-mono:ui-monospace,SFMono-Regular,"SF Mono",Menlo,Consolas,monospace;}` +
	`[data-theme="dark"]{--bg:#0b0d12;--surface:#14171f;--surface-2:#1b1f2a;--border:#262b38;--text:#e6e8ee;--muted:#8b93a7;--accent:#ffffff;--accent-soft:#1e2438;--code-bg:#0b0d13;}` +
	`*{box-sizing:border-box}` +
	`html{scroll-behavior:smooth}` +
	`body{margin:0;background:var(--bg);color:var(--text);font-family:var(--font);line-height:1.65;-webkit-font-smoothing:antialiased;transition:background .2s ease,color .2s ease}` +
	`.topbar{position:sticky;top:0;z-index:20;display:flex;align-items:center;justify-content:space-between;gap:16px;padding:12px 24px;background:color-mix(in srgb,var(--surface) 88%,transparent);backdrop-filter:saturate(160%) blur(10px);border-bottom:1px solid var(--border)}` +
	`.brand{display:flex;align-items:center;gap:12px;text-decoration:none;color:inherit}` +
	`.logo{display:grid;place-items:center;width:38px;height:38px;border-radius:12px;background:var(--accent);color:var(--surface);font-weight:700;font-size:14px;letter-spacing:.02em}` +
	`[data-theme="dark"] .logo{color:#0b0d12}` +
	`.brand-text{display:flex;flex-direction:column;line-height:1.2}` +
	`.brand-text b{font-size:15px}` +
	`.brand-text i{font-style:normal;font-size:12px;color:var(--muted)}` +
	`.top-actions{display:flex;align-items:center;gap:8px}` +
	`.btn{display:inline-flex;align-items:center;gap:6px;padding:8px 14px;border:1px solid var(--border);border-radius:999px;background:var(--surface);color:var(--text);font:inherit;font-size:13px;font-weight:500;text-decoration:none;cursor:pointer;transition:border-color .15s ease,background .15s ease}` +
	`.btn:hover{border-color:var(--accent);background:var(--accent-soft)}` +
	`.btn.icon{padding:8px;border-radius:50%}` +
	`.wrap{max-width:900px;margin:0 auto;padding:40px 24px 80px}` +
	`.doc{background:var(--surface);border:1px solid var(--border);border-radius:var(--r-lg);box-shadow:var(--shadow);padding:48px 56px}` +
	`.doc>*:first-child{margin-top:0}` +
	`.doc h1{font-size:34px;line-height:1.2;letter-spacing:-.02em;margin:0 0 8px}` +
	`.doc h2{font-size:22px;letter-spacing:-.01em;margin:40px 0 12px;padding-top:20px;border-top:1px solid var(--border)}` +
	`.doc h3{font-size:17px;margin:28px 0 8px}` +
	`.doc p{margin:12px 0;color:var(--text)}` +
	`.doc a{color:var(--accent);text-underline-offset:3px}` +
	`.doc a:hover{opacity:.8}` +
	`.doc ul,.doc ol{padding-left:22px;margin:12px 0}` +
	`.doc li{margin:6px 0}` +
	`.doc hr{border:0;border-top:1px solid var(--border);margin:32px 0}` +
	`.doc strong{font-weight:600}` +
	`.doc blockquote{margin:16px 0;padding:12px 18px;border-left:3px solid var(--accent);background:var(--surface-2);border-radius:0 var(--r-md) var(--r-md) 0;color:var(--muted)}` +
	`.doc blockquote p{margin:0;color:inherit}` +
	`.doc code{font-family:var(--font-mono);font-size:.88em;background:var(--code-bg);border:1px solid var(--border);padding:2px 6px;border-radius:6px}` +
	`.doc pre{background:var(--code-bg);border:1px solid var(--border);border-radius:var(--r-md);padding:16px 18px;overflow-x:auto;margin:16px 0}` +
	`.doc pre code{background:none;border:0;padding:0;font-size:13px;line-height:1.6}` +
	`.doc .table-scroll{overflow-x:auto;margin:16px 0;border:1px solid var(--border);border-radius:var(--r-md)}` +
	`.doc table{width:100%;border-collapse:collapse;font-size:14px}` +
	`.doc th,.doc td{text-align:left;padding:10px 14px;border-bottom:1px solid var(--border);vertical-align:top}` +
	`.doc th{background:var(--surface-2);font-weight:600;white-space:nowrap}` +
	`.doc tbody tr:last-child td{border-bottom:0}` +
	`.doc tbody tr:hover{background:var(--surface-2)}` +
	`.doc img{max-width:100%;border-radius:var(--r-md)}` +
	`.katex-display{overflow-x:auto;overflow-y:hidden;padding:4px 0}` +
	`.foot{max-width:900px;margin:0 auto;padding:0 24px 48px;color:var(--muted);font-size:13px;text-align:center}` +
	`.foot code{font-family:var(--font-mono);background:var(--code-bg);border:1px solid var(--border);padding:2px 6px;border-radius:6px}` +
	`@media (max-width:640px){.wrap{padding:20px 12px 60px}.doc{padding:28px 20px}.doc h1{font-size:27px}.doc h2{font-size:19px}.brand-text i{display:none}}` +
	`</style></head>`

func (s *Server) handleRooms(w http.ResponseWriter, r *http.Request) {
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
		rooms, err := s.Store.ListRooms(id.UserID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"rooms": rooms})
	case http.MethodPost:
		var body struct {
			Room string `json:"room"`
		}
		raw, err := readBody(w, r)
		if err != nil {
			if err == errTooLarge {
				writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "body too large"})
				return
			}
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
			return
		}
		if len(strings.TrimSpace(string(raw))) > 0 {
			if err := json.Unmarshal(raw, &body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
				return
			}
		}
		room := strings.TrimSpace(body.Room)
		if !validRoomName(room) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "room name must be 1-64 chars: letters, digits, . _ -"})
			return
		}
		created, err := s.Store.CreateRoom(id.UserID, room)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		status := http.StatusCreated
		if !created {
			status = http.StatusConflict
		}
		writeJSON(w, status, map[string]any{"room": room, "created": created})
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
		n, err := s.Store.DeleteRoom(id.UserID, room)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		s.Hub.NotifyRoomDeleted(id.UserID, room)
		writeJSON(w, http.StatusOK, map[string]any{"room": room, "deleted": n})
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found", "path": r.URL.Path})
	}
}

func validRoomName(name string) bool {
	if len(name) == 0 || len(name) > 64 {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '.' || c == '_' || c == '-':
		default:
			return false
		}
	}
	return true
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
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
		params := r.URL.Query()
		res, err := s.Store.Query(id.UserID, params)
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
		s.handlePostMessage(w, r, id)
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found", "path": r.URL.Path})
	}
}

func (s *Server) handlePostMessage(w http.ResponseWriter, r *http.Request, id *Identity) {
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

	if replyTo != nil && !s.Store.MessageBelongsToUser(id.UserID, *replyTo) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid reply_to"})
		return
	}

	mentions := filter.ParseMentions(content)
	msg, err := s.Store.Insert(id.UserID, agent, room, content, mentions, replyTo)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.Hub.Broadcast(id.UserID, room, msg)
	writeJSON(w, http.StatusCreated, msg)
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	id := s.currentUser(r)
	if id == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	room := r.URL.Query().Get("room")
	if room == "" {
		room = "*"
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	client := s.Hub.Add(id.UserID, room)
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
