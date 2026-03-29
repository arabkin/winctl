package server

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"
	"winctl/config"
	"winctl/scheduler"
	"winctl/state"
	"winctl/web"
)

func New(cfg *config.Config, st *state.State, sched *scheduler.Scheduler) *http.Server {
	h := &handlers{state: st, scheduler: sched}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", h.status)
	mux.HandleFunc("/api/restart/once", h.restartOnce)
	mux.HandleFunc("/api/restart/schedule", h.restartSchedule)
	mux.HandleFunc("/api/lock/once", h.lockOnce)
	mux.HandleFunc("/api/lock/schedule", h.lockSchedule)
	mux.HandleFunc("/api/reset", h.reset)

	staticFS, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		log.Fatalf("failed to create sub filesystem: %v", err)
	}
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	return &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      basicAuth(cfg, mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

func Run(srv *http.Server, ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		log.Printf("HTTP server listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
