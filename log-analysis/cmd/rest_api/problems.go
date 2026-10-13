package main

import (
	"errors"
	"net/http"
	"strconv"

	"logs-backend/internal/data"
)

// Problem workspace endpoints. A failing source is reported inside a 200
// answer (like statistics); only invalid requests are 400.

func (app *application) problems(w http.ResponseWriter, r *http.Request) {
	var req data.ProblemsRequest
	if !app.readValid(w, r, &req, func() error { return req.Validate() }) {
		return
	}
	list := data.ListProblems(r.Context(), req)
	app.writeJSON(r.Context(), w, http.StatusOK, jsonResponse{Message: "success", Data: envelope{"Problems": list}})
}

func (app *application) problemServices(w http.ResponseWriter, r *http.Request) {
	from, err1 := strconv.ParseInt(r.URL.Query().Get("time_from"), 10, 64)
	to, err2 := strconv.ParseInt(r.URL.Query().Get("time_to"), 10, 64)
	if err1 != nil || err2 != nil || to <= from {
		app.errorJSON(r.Context(), w, errors.New("time_from and time_to must be Unix nanoseconds with time_from < time_to"))
		return
	}
	services, err := data.ListServices(r.Context(), from, to)
	if err != nil {
		app.queryError(w, r, err)
		return
	}
	app.writeJSON(r.Context(), w, http.StatusOK, jsonResponse{Message: "success", Data: envelope{"Services": services}})
}

func (app *application) getDetectionRules(w http.ResponseWriter, r *http.Request) {
	rules, err := data.LoadDetectionRules(r.Context())
	if err != nil {
		app.queryError(w, r, err)
		return
	}
	app.writeJSON(r.Context(), w, http.StatusOK, jsonResponse{Message: "success", Data: envelope{"Rules": rules}})
}

// saveDetectionRules stores a new revision. 400 with field errors when
// invalid, 409 with the current revision when another save came first.
func (app *application) saveDetectionRules(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Rules            data.DetectionRules `json:"rules"`
		ExpectedRevision *int64              `json:"expected_revision"`
	}
	if err := app.readJSON(w, r, &req); err != nil {
		app.errorJSON(r.Context(), w, err)
		return
	}
	if req.ExpectedRevision == nil {
		app.errorJSON(r.Context(), w, errors.New("expected_revision is required"))
		return
	}
	if fields := req.Rules.Validate(); len(fields) > 0 {
		app.writeJSON(r.Context(), w, http.StatusBadRequest, jsonResponse{Error: true, Message: "the rules are not valid",
			Data: envelope{"FieldErrors": fields}})
		return
	}
	saved, err := data.SaveDetectionRules(r.Context(), req.Rules, *req.ExpectedRevision)
	if errors.Is(err, data.ErrRulesConflict) {
		app.writeJSON(r.Context(), w, http.StatusConflict, jsonResponse{Error: true, Message: err.Error(),
			Data: envelope{"Rules": saved}})
		return
	}
	if err != nil {
		app.queryError(w, r, err)
		return
	}
	app.writeJSON(r.Context(), w, http.StatusOK, jsonResponse{Message: "success", Data: envelope{"Rules": saved}})
}
