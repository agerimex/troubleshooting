package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"logs-backend/internal/data"
	"logs-backend/internal/driver"

	sender "github.com/agerimex/troubleshooting/log-sender"
)

type application struct {
	serviceConfig ServiceConfig
	infoLog       *log.Logger
	errorLog      *log.Logger
	environment   string
	models        data.Models
}

type ServiceConfig struct {
	listenAddr     string
	allowedOrigins []string
}

func getenv(key, def string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return def
}

// initOpenTelemetry sends this service's own traces to the receiver.
// TROUBLESHOOTING_TOKEN, when set, is picked up by log-sender itself.
func initOpenTelemetry() {
	_, err := sender.NewTracer("LogAnalysis", sender.WithAddress(getenv("TRACE_RECEIVER_ADDR", "log-receiver-compose:50055")))
	if err != nil {
		fmt.Println("Where is receiver of traces")
	}
}

func main() {
	cfg := ServiceConfig{
		listenAddr: getenv("LISTEN_ADDR", ":8094"),
		// In docker-compose the UI calls the API through Caddy on the same
		// origin, so CORS only matters for `npm run dev` (Vite on port 5173).
		allowedOrigins: strings.Split(getenv("CORS_ALLOWED_ORIGINS", "http://localhost:5173,http://127.0.0.1:5173"), ","),
	}

	infoLog := log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime|log.Lshortfile)
	errorLog := log.New(os.Stdout, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)

	db, err := driver.Connect(driver.ConfigFromEnv())

	if err != nil {
		log.Fatal(err)
	}

	initOpenTelemetry()

	app := &application{
		serviceConfig: cfg,
		infoLog:       infoLog,
		errorLog:      errorLog,
		environment:   os.Getenv("APP_ENV"),
		models:        data.New(db),
	}

	err = app.serve()
	if err != nil {
		log.Fatal(err)
	}
}

// serve runs until the server fails or the process gets SIGINT/SIGTERM; then it
// finishes in-flight requests and flushes this service's buffered spans.
func (app *application) serve() error {

	app.infoLog.Println("API listening on", app.serviceConfig.listenAddr)

	srv := &http.Server{
		Addr:    app.serviceConfig.listenAddr,
		Handler: app.routers(),
	}

	stop, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.ListenAndServe() }()

	select {
	case err := <-serveErr:
		return err
	case <-stop.Done():
	}

	app.infoLog.Println("Shutting down")
	ctx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := srv.Shutdown(ctx); err != nil {
		return err
	}
	if err := sender.Shutdown(ctx); err != nil {
		app.errorLog.Println("flush traces:", err)
	}
	return nil
}
