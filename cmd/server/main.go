package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jeretmccoy/kelma-sync/internal/db"
	"github.com/jeretmccoy/kelma-sync/internal/handler"
	"github.com/jeretmccoy/kelma-sync/internal/storage"
)

func main() {
	ctx := context.Background()

	pool, err := db.Connect(ctx)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	migrationsDir := os.Getenv("MIGRATIONS_DIR")
	if migrationsDir == "" {
		migrationsDir = "migrations"
	}
	if err := db.Migrate(ctx, pool, migrationsDir); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	var store storage.Storage
	switch {
	case os.Getenv("R2_ACCOUNT_ID") != "":
		store, err = storage.NewR2(ctx)
		if err != nil {
			log.Fatalf("storage: %v", err)
		}
		log.Printf("storage: R2")
	default:
		// Filesystem storage persists media across restarts. In-memory storage
		// silently lost every blob on restart while the DB still claimed the
		// files existed, which surfaced to clients as "storage read failed".
		mediaDir := os.Getenv("MEDIA_DIR")
		if mediaDir == "" {
			mediaDir = "media_store"
		}
		store, err = storage.NewFS(mediaDir)
		if err != nil {
			log.Fatalf("storage: %v", err)
		}
		log.Printf("storage: filesystem at %s", mediaDir)
	}

	h := handler.New(pool, store)
	mux := http.NewServeMux()
	h.Routes(mux)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := fmt.Sprintf(":%s", port)
	srv := &http.Server{
		Addr:    addr,
		Handler: handler.Middleware(mux),
		// Generous but bounded: media uploads are capped at 100 MiB and a
		// first-time sync can push thousands of records, but a connection should
		// not be able to hang forever. No global write timeout, since large
		// media downloads over slow links legitimately take a while; rely on
		// ReadHeaderTimeout + IdleTimeout to shed dead connections.
		ReadHeaderTimeout: 30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Graceful shutdown: finish in-flight requests on SIGTERM/SIGINT so a
	// deploy/restart doesn't drop a sync mid-flight.
	idleClosed := make(chan struct{})
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		log.Printf("shutting down…")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("graceful shutdown error: %v", err)
		}
		close(idleClosed)
	}()

	log.Printf("KelmaSync listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server: %v", err)
	}
	<-idleClosed
}
