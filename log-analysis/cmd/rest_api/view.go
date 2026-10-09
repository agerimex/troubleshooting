package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"logs-backend/internal/data"
)

func (app *application) selectAllLogs(ctx context.Context) ([]*data.Log, error) {
	return app.models.Log.SelectAllData(ctx)
}

func (app *application) viewLogs(w http.ResponseWriter, r *http.Request) {
	// var users data.User
	all, err := app.selectAllLogs(r.Context())
	if err != nil {
		app.errorLog.Println(err)
		app.errorJSON(r.Context(), w, err, http.StatusInternalServerError)
		return
	}

	payload := jsonResponse{
		Error:   false,
		Message: "success",
		Data:    envelope{"logs": all},
	}

	app.writeJSON(r.Context(), w, http.StatusOK, payload)
}

func (app *application) selectAllRootSpans(ctx context.Context, filter data.SpanFilter) ([]*data.Span, error) {
	return app.models.Log.SelectRootSpan(ctx, filter)
}

func (app *application) selectCountSpans(ctx context.Context, filter data.SpanFilter) (uint64, error) {
	return app.models.Log.SelectCountSpans(ctx, filter)
}

func getStringFromQuery(query url.Values, key string, def string) string {
	result := query.Get(key)
	fmt.Println("Raw value from query:", result)

	if len(result) == 0 {
		fmt.Println("Empty value, returning default:", def)
		return def
	}

	return result
}

func getIntFromQuery(query url.Values, key string, def int) int {
	result := query.Get(key)
	fmt.Println("Raw value from query:", result)

	if len(result) == 0 {
		fmt.Println("Empty value, returning default:", def)
		return def
	}

	value, err := strconv.Atoi(result)
	if err != nil {
		fmt.Println("Error converting to int:", err)
		fmt.Println("Returning default value")
		return def
	}

	return value
}

func (app *application) viewSpans(w http.ResponseWriter, r *http.Request) {

	var requestPayload data.SpanFilter

	err := app.readJSON(w, r, &requestPayload)
	if err != nil {
		app.errorJSON(r.Context(), w, err)
		return
	}

	if requestPayload.ParentId == "" {
		requestPayload.ParentId = "0000000000000000"
	}

	all, err := app.selectAllRootSpans(r.Context(), requestPayload)
	if err != nil {
		app.errorLog.Println(err)
		app.errorJSON(r.Context(), w, err, http.StatusInternalServerError)
		return
	}

	payload := jsonResponse{
		Error:   false,
		Message: "success",
		Data:    envelope{"Spans": all},
	}

	app.writeJSON(r.Context(), w, http.StatusOK, payload)
}

func (app *application) countSpans(w http.ResponseWriter, r *http.Request) {
	var requestPayload data.SpanFilter

	err := app.readJSON(w, r, &requestPayload)
	if err != nil {
		app.errorJSON(r.Context(), w, err)
		return
	}

	if requestPayload.ParentId == "" {
		requestPayload.ParentId = "0000000000000000"
	}

	count, err := app.selectCountSpans(r.Context(), requestPayload)
	if err != nil {
		app.errorLog.Println(err)
		app.errorJSON(r.Context(), w, err, http.StatusInternalServerError)
		return
	}

	payload := jsonResponse{
		Error:   false,
		Message: "success",
		Data:    envelope{"Count": count},
	}

	app.writeJSON(r.Context(), w, http.StatusOK, payload)
}
