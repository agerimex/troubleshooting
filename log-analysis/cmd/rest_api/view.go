package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"logs-backend/internal/data"
)

func (app *application) selectAllLogs() ([]*data.Log, error) {
	ctx := context.Background()
	return app.models.Log.SelectAllData(ctx)
}

func (app *application) viewLogs(w http.ResponseWriter, r *http.Request) {
	// var users data.User
	all, err := app.selectAllLogs()
	if err != nil {
		app.errorLog.Println(err)
		return
	}

	payload := jsonResponse{
		Error:   false,
		Message: "success",
		Data:    envelope{"logs": all},
	}

	app.writeJSON(w, http.StatusOK, payload)
}

func (app *application) selectAllRootSpans(filter data.SpanFilter /*parent string, rowsPerPage int, timeFrom string, status string*/) ([]*data.Span, error) {
	ctx := context.Background()
	return app.models.Log.SelectRootSpan(ctx, filter)
}

func (app *application) selectCountSpans(filter data.SpanFilter) (uint64, error) {
	ctx := context.Background()
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
		app.errorJSON(w, err)
		return
	}

	if requestPayload.ParentId == "" {
		requestPayload.ParentId = "0000000000000000"
	}

	all, err := app.selectAllRootSpans(requestPayload)
	if err != nil {
		app.errorLog.Println(err)
		return
	}

	payload := jsonResponse{
		Error:   false,
		Message: "success",
		Data:    envelope{"Spans": all},
	}

	app.writeJSON(w, http.StatusOK, payload)
}

func (app *application) countSpans(w http.ResponseWriter, r *http.Request) {
	var requestPayload data.SpanFilter

	err := app.readJSON(w, r, &requestPayload)
	if err != nil {
		app.errorJSON(w, err)
		return
	}

	if requestPayload.ParentId == "" {
		requestPayload.ParentId = "0000000000000000"
	}

	count, err := app.selectCountSpans(requestPayload)
	if err != nil {
		app.errorLog.Println(err)
		return
	}

	payload := jsonResponse{
		Error:   false,
		Message: "success",
		Data:    envelope{"Count": count},
	}

	app.writeJSON(w, http.StatusOK, payload)
}
