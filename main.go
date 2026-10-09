package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"agentchat/internal/api"
	"agentchat/internal/hub"
	"agentchat/internal/store"
)

func main() {
	port := env("PORT", "8086")
	host := env("HOST", "0.0.0.0")
	dbPath := env("DB_PATH", defaultDBPath())
	uiDir := env("UI_DIR", ".")

	authRequired := env("AUTH_REQUIRED", "1")
	bcryptCost := envInt("BCRYPT_COST", 12)
	if bcryptCost < 10 {
		bcryptCost = 12
	}
	_ = env("TLS", "0")

	st, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	if err := st.SeedAdmin(); err != nil {
		log.Fatalf("seed admin: %v", err)
	}

	log.Printf("auth_required=%s", authRequired)
	log.Printf("bcrypt_cost=%d", bcryptCost)
	if authRequired == "0" {
		log.Printf("WARNING: AUTH_REQUIRED=0 — authentication is DISABLED (debug only). Set AUTH_REQUIRED=1 for production.")
		log.Printf("WARNING: /api/admin/* is blocked unless DEV_ALLOW_ADMIN=1 is also set.")
	}

	h := hub.New()
	srv := api.New(st, h, uiDir)

	mux := http.NewServeMux()
	srv.Register(mux)

	httpServer := &http.Server{
		Addr:    host + ":" + port,
		Handler: mux,
	}

	go func() {
		log.Printf("AgentChat listening on http://%s:%s", host, port)
		log.Printf("DB: %s", dbPath)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return n
}

// defaultDBPath returns chat.db next to the executable.
func defaultDBPath() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "chat.db")
	}
	return "chat.db"
}
