package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Review of b7c2e87, finding 2: when the API sent the evaluation to the
// history but couldn't read the stored record back, the CLI still said
// "Recorded as evaluation". It keeps the id and says the record is not
// confirmed; with an API that sends only record_error the same holds.
func TestUnconfirmedRecord(t *testing.T) {
	for name, flag := range map[string]bool{"record_unconfirmed": true, "only record_error": false} {
		t.Run(name, func(t *testing.T) {
			var env map[string]any
			if err := json.Unmarshal([]byte(recordedAnswer(t, "")), &env); err != nil {
				t.Fatal(err)
			}
			ev := env["data"].(map[string]any)["Evaluation"].(map[string]any)
			ev["record_error"] = "the evaluation 0123456789abcdef01234567 was sent to the history, but reading it back failed (timeout): this is the local verdict"
			if flag {
				ev["record_unconfirmed"] = true
			}
			body, _ := json.Marshal(env)
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
			defer s.Close()
			dir := t.TempDir()
			rules := writeFile(t, "quality.yaml", requirementsRules)
			var out, errOut bytes.Buffer
			code := run([]string{"--url", s.URL, "--rules", rules, "--run", "pr-42", "--record", "--ui", "http://ui.example", "--json", filepath.Join(dir, "r.json")}, &out, &errOut, fixedNow)
			if code != 1 {
				t.Fatalf("exit %d: the exit code follows this run's gate", code)
			}
			text := out.String()
			if strings.Contains(text, "Recorded as evaluation") || !strings.Contains(text, "Not confirmed: the evaluation was sent to the history as 0123456789abcdef01234567, but the API couldn't verify what is stored") ||
				!strings.Contains(text, "#/quality?evaluation=0123456789abcdef01234567") {
				t.Errorf("stdout:\n%s", text)
			}
			if !strings.Contains(errOut.String(), "warning: the evaluation 0123456789abcdef01234567 was sent to the history") {
				t.Errorf("stderr %q", errOut.String())
			}
			var saved Evaluation
			b, _ := os.ReadFile(filepath.Join(dir, "r.json"))
			if err := json.Unmarshal(b, &saved); err != nil || saved.ID == "" || saved.RecordUnconfirmed != flag {
				t.Errorf("json keeps the id and the flag: %v %+v", err, saved.RecordUnconfirmed)
			}
		})
	}
}

// child_counts in quality.yaml is read strictly like every key.
func TestChildCountsRule(t *testing.T) {
	path := writeFile(t, "quality.yaml", strings.Replace(requirementsRules, "checks:\n", "run: {child_counts: required}\nchecks:\n", 1))
	r, _, err := loadRules(path)
	if err != nil || r.Run == nil || r.Run.ChildCounts != "required" {
		t.Errorf("rules %+v, %v", r.Run, err)
	}
}
