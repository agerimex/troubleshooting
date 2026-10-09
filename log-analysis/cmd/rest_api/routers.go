package main

import (
	"net/http"

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

	mux.Get("/api/v1/view-logs", app.viewLogs)
	mux.Post("/api/v1/view-spans", app.viewSpans)
	mux.Post("/api/v1/count-spans", app.countSpans)

	return mux
}
