package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

func (app *application) routers() http.Handler {
	mux := chi.NewRouter()
	mux.Use(middleware.Recoverer)
	// mux.Use(app.Logging)
	// mux.Use(otelchi.Middleware("LOGS", otelchi.WithChiRoutes(mux)))
	mux.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://*", "http://*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// mux.Post("/api/v1/login", app.Login)
	// mux.Post("/api/v1/logout", app.Logout)

	mux.Route("/api/v1/privy", func(mux chi.Router) {
		// mux.Use(app.AuthTokenMiddleware)
	})

	// mux.Post("/api/v1/validate-token", app.ValidateToken)
	mux.Get("/api/v1/view-logs", app.viewLogs)
	mux.Post("/api/v1/view-spans", app.viewSpans)
	mux.Post("/api/v1/count-spans", app.countSpans)

	return mux
}
