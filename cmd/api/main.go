package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"

	"github.com/progerman666/url-shortener/internal/handler"
	"github.com/progerman666/url-shortener/internal/repository"
	"github.com/progerman666/url-shortener/internal/service"
	"github.com/progerman666/url-shortener/internal/worker"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf("warning: .env not loaded: %v", err)
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}
	httpAddr := os.Getenv("HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = ":8081"
	}

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("db ping: %v", err)
	}
	log.Println("DB connected")

	repo := repository.NewLinkRepository(db)
	svc := service.NewLinkService(repo)

	clicksWorker := worker.NewClickWorker(repo, 1000)
	clicksWorker.Start()
	log.Println("click worker started")

	h := handler.NewLinkHandler(svc, clicksWorker, "http://localhost"+httpAddr)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("POST /links", h.Create)
	mux.HandleFunc("GET /links/{code}/stats", h.Stats)
	mux.HandleFunc("GET /{code}", h.Redirect)

	srv := &http.Server{
		Addr:              httpAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Ожидание сигнала завершения (Ctrl+C / SIGTERM) в отдельной горутине
	shutdownDone := make(chan struct{})
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		sig := <-sigCh
		log.Printf("received signal %v, shutting down...", sig)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("http shutdown: %v", err)
		}
		close(shutdownDone)
	}()

	log.Printf("listening on %s", httpAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}

	// Ждём завершения HTTP (если ещё идёт), затем гасим воркер
	<-shutdownDone
	log.Println("http server stopped, draining click worker...")
	clicksWorker.Stop()
	log.Println("bye")
}