package main

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"logs-backend/internal/data"

	"github.com/go-chi/chi/v5"
)

// Statistics endpoints answer 200 with a status inside the result when a
// source fails, so an unavailable value is never shown as zero; only invalid
// requests are 400.

func (app *application) callStats(w http.ResponseWriter, r *http.Request) {
	var req data.StatsRequest
	if !app.readValid(w, r, &req, func() error { return req.Population.Validate() }) {
		return
	}
	stats := data.ComputeStats(r.Context(), req)
	app.writeJSON(r.Context(), w, http.StatusOK, jsonResponse{Message: "success", Data: envelope{"Stats": stats}})
}

func (app *application) errorGroups(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Population data.Population `json:"population"`
	}
	if !app.readValid(w, r, &req, func() error { return req.Population.Validate() }) {
		return
	}
	groups := data.ComputeErrorGroups(r.Context(), req.Population)
	app.writeJSON(r.Context(), w, http.StatusOK, jsonResponse{Message: "success", Data: envelope{"ErrorGroups": groups}})
}

// shapes answers the sequence library: execution shapes of a window with their
// counts, duration percentiles and representative executions.
func (app *application) shapes(w http.ResponseWriter, r *http.Request) {
	var req data.ShapeFilter
	if !app.readValid(w, r, &req, func() error { return req.Validate() }) {
		return
	}
	app.writeJSON(r.Context(), w, http.StatusOK, jsonResponse{Message: "success", Data: envelope{"Library": data.ComputeShapes(r.Context(), req)}})
}

func (app *application) examples(w http.ResponseWriter, r *http.Request) {
	var req data.ExamplesRequest
	if !app.readValid(w, r, &req, func() error { return req.Validate() }) {
		return
	}
	result := data.FindExamples(r.Context(), req)
	app.writeJSON(r.Context(), w, http.StatusOK, jsonResponse{Message: "success", Data: envelope{"Examples": result}})
}

func (app *application) compare(w http.ResponseWriter, r *http.Request) {
	var req data.CompareRequest
	if !app.readValid(w, r, &req, func() error { return req.Validate() }) {
		return
	}
	analysis, err := data.Compare(r.Context(), req)
	if err != nil {
		// Not stored: it couldn't be referenced or exported later.
		app.queryError(w, r, err)
		return
	}
	app.writeJSON(r.Context(), w, http.StatusOK, jsonResponse{Message: "success", Data: envelope{"Analysis": analysis}})
}

func (app *application) listAnalyses(w http.ResponseWriter, r *http.Request) {
	list, err := data.ListAnalyses(r.Context(), 100)
	if err != nil {
		app.queryError(w, r, err)
		return
	}
	app.writeJSON(r.Context(), w, http.StatusOK, jsonResponse{Message: "success", Data: envelope{"Analyses": list}})
}

func (app *application) getAnalysis(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !data.ValidAnalysisID(id) {
		app.errorJSON(r.Context(), w, errors.New("invalid analysis id"))
		return
	}
	body, err := data.LoadAnalysis(r.Context(), id)
	if errors.Is(err, data.ErrAnalysisNotFound) {
		app.errorJSON(r.Context(), w, err, http.StatusNotFound)
		return
	}
	if err != nil {
		app.queryError(w, r, err)
		return
	}
	app.writeJSON(r.Context(), w, http.StatusOK, jsonResponse{Message: "success", Data: envelope{"Analysis": body}})
}

func (app *application) statsStatus(w http.ResponseWriter, r *http.Request) {
	if _, err := data.LoadPolicy(r.Context()); err != nil {
		app.errorLog.Println("read retention:", err)
	}
	app.writeJSON(r.Context(), w, http.StatusOK, jsonResponse{Message: "success", Data: envelope{"Status": data.StatsRefresher().Status()}})
}

// rebuildStats refreshes (or backfills) the buckets of a range now and
// reports each bucket's outcome. Rerunning it is idempotent.
func (app *application) rebuildStats(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TimeFrom string `json:"time_from"`
		TimeTo   string `json:"time_to"`
	}
	if err := app.readJSON(w, r, &req); err != nil {
		app.errorJSON(r.Context(), w, err)
		return
	}
	from, err1 := strconv.ParseInt(req.TimeFrom, 10, 64)
	to, err2 := strconv.ParseInt(req.TimeTo, 10, 64)
	if err1 != nil || err2 != nil || to <= from || time.Duration(to-from) > data.MaxStatsWindow {
		app.errorJSON(r.Context(), w, errors.New("time_from and time_to must be Unix nanoseconds with time_from < time_to, at most 92 days apart"))
		return
	}
	if _, err := data.LoadPolicy(r.Context()); err != nil {
		app.queryError(w, r, err)
		return
	}
	outcomes, err := data.StatsRefresher().RefreshRange(r.Context(), from, to, "rebuild")
	if err != nil {
		app.queryError(w, r, err)
		return
	}
	app.writeJSON(r.Context(), w, http.StatusOK, jsonResponse{Message: "success", Data: envelope{"Buckets": outcomes}})
}

func (app *application) declareCoverage(w http.ResponseWriter, r *http.Request) {
	var d data.Declaration
	if !app.readValid(w, r, &d, func() error { return d.Validate() }) {
		return
	}
	if err := data.Declare(r.Context(), d); err != nil {
		app.queryError(w, r, err)
		return
	}
	app.writeJSON(r.Context(), w, http.StatusOK, jsonResponse{Message: "success", Data: envelope{"Declaration": d}})
}

func (app *application) retention(w http.ResponseWriter, r *http.Request) {
	policy, err := data.LoadPolicy(r.Context())
	if err != nil {
		app.queryError(w, r, err)
		return
	}
	app.writeJSON(r.Context(), w, http.StatusOK, jsonResponse{Message: "success", Data: envelope{"Policy": policy}})
}

// Test controls, only with TEST_CLOCK_CONTROL=1 (the regression fixture's
// stack): move the application clock, make bucket commits fail.

func (app *application) setClock(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OffsetSeconds int64 `json:"offset_seconds"`
	}
	if err := app.readJSON(w, r, &req); err != nil {
		app.errorJSON(r.Context(), w, err)
		return
	}
	data.AppClock().SetOffset(time.Duration(req.OffsetSeconds) * time.Second)
	app.infoLog.Printf("test clock offset set to %s", data.AppClock().Offset())
	app.writeJSON(r.Context(), w, http.StatusOK, jsonResponse{Message: "success", Data: envelope{"OffsetSeconds": req.OffsetSeconds}})
}

func (app *application) failNextCommits(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Count int `json:"count"`
	}
	if err := app.readJSON(w, r, &req); err != nil {
		app.errorJSON(r.Context(), w, err)
		return
	}
	data.StatsRefresher().FailNext(req.Count)
	app.writeJSON(r.Context(), w, http.StatusOK, jsonResponse{Message: "success", Data: envelope{"Count": req.Count}})
}

// readValid reads a JSON body into dst and validates it; it answers 400 and
// returns false on failure.
func (app *application) readValid(w http.ResponseWriter, r *http.Request, dst any, validate func() error) bool {
	if err := app.readJSON(w, r, dst); err != nil {
		app.errorJSON(r.Context(), w, err)
		return false
	}
	if err := validate(); err != nil {
		app.errorJSON(r.Context(), w, err)
		return false
	}
	return true
}
