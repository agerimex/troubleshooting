package main

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/riandyrn/otelchi"
)

func (app *application) routers() http.Handler {
	mux := chi.NewRouter()
	mux.Use(middleware.Recoverer)
	// mux.Use(app.Logging)
	mux.Use(otelchi.Middleware("LOG", otelchi.WithChiRoutes(mux)))
	// No AllowCredentials: the UI sends no cookies, and basic auth is handled by
	// Caddy on the same origin.
	mux.Use(cors.Handler(cors.Options{
		AllowedOrigins: app.serviceConfig.allowedOrigins,
		AllowedMethods: []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders: []string{"Accept", "Authorization", "Content-Type"},
		MaxAge:         300,
	}))

	mux.Group(func(r chi.Router) {
		r.Use(app.requestDeadline(app.requestTimeout()))
		r.Post("/api/v1/search-logs", app.searchLogs)
		r.Post("/api/v1/trace-report", app.traceReport)
		r.Post("/api/v1/view-spans", app.viewSpans)
		r.Post("/api/v1/count-spans", app.countSpans)
	})

	// Statistics run several bounded queries one after another.
	mux.Group(func(r chi.Router) {
		r.Use(app.requestDeadline(app.statsTimeout()))
		r.Post("/api/v1/stats/calls", app.callStats)
		r.Post("/api/v1/stats/error-groups", app.errorGroups)
		r.Post("/api/v1/stats/examples", app.examples)
		r.Post("/api/v1/stats/shapes", app.shapes)
		r.Get("/api/v1/stats/status", app.statsStatus)
		r.Post("/api/v1/compare", app.compare)
		r.Get("/api/v1/analyses", app.listAnalyses)
		r.Get("/api/v1/analyses/{id}", app.getAnalysis)
		r.Post("/api/v1/coverage", app.declareCoverage)
		r.Get("/api/v1/retention", app.retention)
		r.Post("/api/v1/problems", app.problems)
		r.Get("/api/v1/problems/services", app.problemServices)
		r.Get("/api/v1/detection-rules", app.getDetectionRules)
		r.Post("/api/v1/detection-rules", app.saveDetectionRules)
	})

	mux.Group(func(r chi.Router) {
		r.Use(app.requestDeadline(rebuildTimeout))
		r.Post("/api/v1/stats/rebuild", app.rebuildStats)
	})

	if app.testControls {
		mux.Post("/api/v1/admin/clock", app.setClock)
		mux.Post("/api/v1/admin/stats/fail-next", app.failNextCommits)
	}

	return mux
}

// requestTimeout is the deadline of one investigation request: a span page
// runs two queries (rows, then child counts), each bounded by queryTimeout.
func (app *application) requestTimeout() time.Duration {
	return 2*app.queryTimeout + 2*time.Second
}

// statsTimeout bounds a statistics request or comparison: a comparison runs
// up to six bounded queries in sequence per population.
func (app *application) statsTimeout() time.Duration {
	return 6*app.queryTimeout + 2*time.Second
}

// rebuildTimeout bounds a synchronous rebuild of a range of buckets.
const rebuildTimeout = 5 * time.Minute

// requestDeadline cancels the request context, and with it the ClickHouse
// queries, after timeout or when the client goes away.
func (app *application) requestDeadline(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
