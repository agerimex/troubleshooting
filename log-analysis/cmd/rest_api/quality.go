package main

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"logs-backend/internal/data"
)

// qualityEvaluate checks a test run against the posted rules (quality gate,
// docs/quality-gate.md). Invalid requests are 400; a run that can't be read
// (retention, limits, database) is a 200 answer whose checks are
// "insufficient" with the reason, so the gate is unresolved, never passed.
// With record the evaluation is also stored as the project's history; a
// failed write is reported in record_error and doesn't change the answer.
func (app *application) qualityEvaluate(w http.ResponseWriter, r *http.Request) {
	var req data.QualityRequest
	if !app.readValid(w, r, &req, func() error { return req.Validate() }) {
		return
	}
	res := data.EvaluateQuality(r.Context(), req)
	if req.Record {
		data.RecordQuality(r.Context(), req, &res)
		if res.RecordError != "" {
			app.errorLog.Println(res.RecordError)
		}
	}
	app.writeJSON(r.Context(), w, http.StatusOK, jsonResponse{Message: "success", Data: envelope{"Evaluation": res}})
}

// qualityProjects lists the projects with recorded evaluations.
func (app *application) qualityProjects(w http.ResponseWriter, r *http.Request) {
	list, err := data.ListQualityProjects(r.Context())
	if err != nil {
		app.queryError(w, r, err)
		return
	}
	app.writeJSON(r.Context(), w, http.StatusOK, jsonResponse{Message: "success", Data: envelope{"Projects": list}})
}

// qualityEvaluations lists recorded evaluations, newest first
// (?project=&ref=&before=&limit=); Next is the before of the next page.
func (app *application) qualityEvaluations(w http.ResponseWriter, r *http.Request) {
	f, err := data.ParseQualityHistoryFilter(r.URL.Query().Get, false)
	if err != nil {
		app.errorJSON(r.Context(), w, err)
		return
	}
	list, next, err := data.ListQualityEvaluations(r.Context(), f)
	if err != nil {
		app.queryError(w, r, err)
		return
	}
	app.writeJSON(r.Context(), w, http.StatusOK, jsonResponse{Message: "success", Data: envelope{"Evaluations": list, "Next": next}})
}

// qualityEvaluation returns one recorded evaluation exactly as recorded, with
// its rules.
func (app *application) qualityEvaluation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !data.ValidQualityEvaluationID(id) {
		app.errorJSON(r.Context(), w, errors.New("invalid evaluation id"))
		return
	}
	rec, err := data.LoadQualityEvaluation(r.Context(), id)
	if errors.Is(err, data.ErrQualityEvaluationNotFound) {
		app.errorJSON(r.Context(), w, err, http.StatusNotFound)
		return
	}
	if err != nil {
		app.queryError(w, r, err)
		return
	}
	app.writeJSON(r.Context(), w, http.StatusOK, jsonResponse{Message: "success", Data: envelope{"Record": rec}})
}

// qualityRequirements returns a project's requirements catalogue
// (?project=&ref=): the latest recorded evaluation's requirements, each with
// its history.
func (app *application) qualityRequirements(w http.ResponseWriter, r *http.Request) {
	f, err := data.ParseQualityHistoryFilter(r.URL.Query().Get, true)
	if err != nil {
		app.errorJSON(r.Context(), w, err)
		return
	}
	cat, err := data.QualityRequirementCatalogue(r.Context(), f.Project, f.Ref)
	if err != nil {
		app.queryError(w, r, err)
		return
	}
	app.writeJSON(r.Context(), w, http.StatusOK, jsonResponse{Message: "success", Data: envelope{"Catalogue": cat}})
}
