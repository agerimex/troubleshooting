package main

import (
	"testing"
)

// The errors check and exclude_failed are read strictly like every key.
func TestErrorsCheckRules(t *testing.T) {
	path := writeFile(t, "quality.yaml", "version: 1\nentry: {service: a, operation: b}\nchecks:\n  - {id: no-failures, type: errors, max_failed: 0}\n  - {id: p95, type: latency, max_ms: 5000, exclude_failed: true}\n")
	r, _, err := loadRules(path)
	if err != nil || len(r.Checks) != 2 || r.Checks[0].MaxFailed == nil || *r.Checks[0].MaxFailed != 0 || !r.Checks[1].ExcludeFailed {
		t.Errorf("rules %+v, %v", r.Checks, err)
	}
}
