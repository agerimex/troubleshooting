package main

import (
	"net/http"

	"logs-backend/internal/data"
)

// qualityEvaluate checks a test run against the posted rules (quality gate,
// docs/quality-gate.md). Invalid requests are 400; a run that can't be read
// (retention, limits, database) is a 200 answer whose checks are
// "insufficient" with the reason, so the gate is unresolved, never passed.
func (app *application) qualityEvaluate(w http.ResponseWriter, r *http.Request) {
	var req data.QualityRequest
	if !app.readValid(w, r, &req, func() error { return req.Validate() }) {
		return
	}
	res := data.EvaluateQuality(r.Context(), req)
	app.writeJSON(r.Context(), w, http.StatusOK, jsonResponse{Message: "success", Data: envelope{"Evaluation": res}})
}
