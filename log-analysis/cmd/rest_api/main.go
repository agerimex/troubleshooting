package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
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
	// queryTimeout is ClickHouse max_execution_time; requests get a deadline
	// derived from it (see requestDeadline).
	queryTimeout time.Duration
	// testControls enables the clock and failure controls of the regression
	// fixture (TEST_CLOCK_CONTROL=1); never on a real installation.
	testControls bool
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

// queryLimitsFromEnv reads QUERY_TIMEOUT (a Go duration), QUERY_MAX_ROWS_TO_READ,
// QUERY_MAX_MEMORY_BYTES and MAX_SEARCH_WINDOW (a Go duration); unset ones keep
// data.DefaultQueryLimits.
func queryLimitsFromEnv() (data.QueryLimits, error) {
	limits := data.DefaultQueryLimits
	for _, d := range []struct {
		env string
		dst *time.Duration
	}{{"QUERY_TIMEOUT", &limits.Timeout}, {"MAX_SEARCH_WINDOW", &limits.MaxWindow}} {
		if value := os.Getenv(d.env); value != "" {
			parsed, err := time.ParseDuration(value)
			if err != nil || parsed < time.Second {
				return limits, fmt.Errorf("%s=%q: want a duration of at least 1s, such as 10s", d.env, value)
			}
			*d.dst = parsed
		}
	}
	for _, n := range []struct {
		env string
		dst *uint64
	}{{"QUERY_MAX_ROWS_TO_READ", &limits.MaxRowsToRead}, {"QUERY_MAX_MEMORY_BYTES", &limits.MaxMemoryBytes}} {
		if value := os.Getenv(n.env); value != "" {
			parsed, err := strconv.ParseUint(value, 10, 64)
			if err != nil || parsed == 0 {
				return limits, fmt.Errorf("%s=%q: want a positive number", n.env, value)
			}
			*n.dst = parsed
		}
	}
	return limits, nil
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

	queryLimits, err := queryLimitsFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	infoLog.Printf("query limits: timeout %s, %d rows read, %d MiB memory, window %s",
		queryLimits.Timeout, queryLimits.MaxRowsToRead, queryLimits.MaxMemoryBytes>>20, queryLimits.MaxWindow)

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
		models:        data.New(db, queryLimits),
		queryTimeout:  queryLimits.Timeout,
		testControls:  os.Getenv("TEST_CLOCK_CONTROL") == "1",
	}
	if app.testControls {
		infoLog.Println("TEST_CLOCK_CONTROL=1: the test clock and failure controls are enabled")
	}

	statsConfig, err := statsConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	data.SetStatsConfig(statsConfig)
	// The retention ClickHouse applies is read from the tables (the receiver
	// sets it), so the report and statistics can't disagree with it.
	if policy, err := data.LoadPolicy(context.Background()); err != nil {
		errorLog.Println("read retention (statistics stay off until it can be read):", err)
	} else if !policy.Compatible {
		errorLog.Println("statistics are off:", policy.Problem)
	} else {
		infoLog.Printf("retention: traces %ds, logs %ds, statistics %ds; allowed lateness %s",
			policy.TracesSeconds, policy.LogsSeconds, policy.StatsSeconds, statsConfig.AllowedLateness)
	}
	if os.Getenv("STATS_REFRESH") != "off" {
		go data.StatsRefresher().Run(context.Background(), statsConfig.RefreshInterval)
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
		Addr:              app.serviceConfig.listenAddr,
		Handler:           app.routers(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      rebuildTimeout + 5*time.Second,
		IdleTimeout:       time.Minute,
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

// statsConfigFromEnv reads STATS_ALLOWED_LATENESS (default 24h; it must match
// the receiver's), STATS_REFRESH_INTERVAL (default 1m) and
// SAMPLING_POLICY_DEFAULT (the policy of spans that declare none, e.g.
// "unsampled"; empty means unknown).
func statsConfigFromEnv() (data.StatsConfig, error) {
	c := data.DefaultStatsConfig
	for _, d := range []struct {
		env string
		dst *time.Duration
	}{{"STATS_ALLOWED_LATENESS", &c.AllowedLateness}, {"STATS_REFRESH_INTERVAL", &c.RefreshInterval}} {
		if value := os.Getenv(d.env); value != "" {
			parsed, err := time.ParseDuration(value)
			if err != nil || parsed < time.Second {
				return c, fmt.Errorf("%s=%q: want a duration of at least 1s, such as 24h", d.env, value)
			}
			*d.dst = parsed
		}
	}
	c.DefaultSampling = os.Getenv("SAMPLING_POLICY_DEFAULT")
	return c, nil
}
