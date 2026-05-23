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

	"runway/reviews/internal/api"
	"runway/reviews/internal/env"
	"runway/reviews/internal/feed"
	"runway/reviews/internal/poller"
	"runway/reviews/internal/store"
)

func main() {
	port := env.GetString("PORT", "8080")
	dataDir := env.GetString("DATA_DIR", "./data")
	pollMinutes := env.GetInt("POLL_INTERVAL_MINUTES", 15)

	// Apple's customer-reviews RSS feed practically caps at 10 pages.
	// Operators can lower this for test envs or raise it if Apple ever
	// loosens the cap, without recompiling.
	feed.MaxPages = env.GetInt("MAX_PAGES", 10)

	pollInterval := time.Duration(pollMinutes) * time.Minute

	s := store.New(dataDir)
	if err := s.Load(); err != nil {
		log.Fatalf("[package:main][method:main][message:failed to load store: %v]", err)
	}
	log.Printf("[package:main][method:main][message:store loaded from %s (%d apps)]", dataDir, len(s.Apps()))

	p := poller.New(s, pollInterval)
	p.Start()

	mux := http.NewServeMux()
	h := api.NewHandler(s, p)
	h.RegisterRoutes(mux)

	addr := fmt.Sprintf(":%s", port)
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
		// ReadHeaderTimeout caps how long a client has to send request headers.
		// Without this, a slowloris-style attacker can pin goroutines by
		// dripping bytes indefinitely. 5 s is generous for legitimate traffic.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// shutdownDone is closed once BOTH phases of the graceful shutdown have
	// finished. main MUST wait on this before exiting — srv.ListenAndServe
	// returns ErrServerClosed the moment Shutdown closes the listener, NOT
	// when Shutdown finishes draining in-flight requests. Without the wait,
	// the process can exit while a worker is mid-MergeReviews (abandoned
	// disk write) or while srv.Shutdown is still draining.
	shutdownDone := make(chan struct{})

	go func() {
		defer close(shutdownDone)
		sigC := make(chan os.Signal, 1)
		signal.Notify(sigC, syscall.SIGINT, syscall.SIGTERM)
		<-sigC
		log.Printf("[package:main][method:main][message:shutdown signal received]")

		// Two-phase shutdown, both phases drain their in-flight work before returning.
		//
		// Phase 1 — srv.Shutdown waits (up to 10 s) for all in-flight HTTP requests to
		// finish. This must come first: any concurrent POST /api/apps could call
		// p.PollNow while we're tearing down. After Shutdown returns, the HTTP listener
		// is closed and no further requests can reach the handler.
		//
		// Phase 2 — p.Stop cancels the poller's context and blocks until both the
		// ticker loop and any in-flight PollNow goroutines have exited, guaranteeing
		// no in-flight MergeReviews (disk write) is abandoned mid-way.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("[package:main][method:main][message:graceful shutdown error: %v]", err)
		}

		p.Stop()
	}()

	log.Printf("[package:main][method:main][message:server listening on %s]", addr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("[package:main][method:main][message:server error: %v]", err)
	}
	// Block until the shutdown goroutine has finished both phases. Without
	// this, the process exits as soon as ListenAndServe returns, which is
	// before p.Stop has had a chance to run.
	<-shutdownDone
	log.Printf("[package:main][method:main][message:server shut down gracefully]")
}
