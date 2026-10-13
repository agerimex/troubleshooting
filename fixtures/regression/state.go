package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// state is what a fresh load records so later checks and replays use the
// same run: the run id and base time (the payload is rebuilt from them,
// byte for byte), and how far the RF05 late delivery has got.
type state struct {
	Version      int    `json:"version"`
	Run          string `json:"run"`
	BaseUnixNano int64  `json:"base_unix_nano"`
	Loaded       string `json:"loaded"` // RFC 3339, for people
	LateSent     bool   `json:"late_sent"`
	ParitySent   bool   `json:"parity_sent"`
	InvalidSent  bool   `json:"invalid_sent"`
	// Milestone 1: the REL-003 parent was delivered; the REF-003 outage log
	// was confirmed.
	M1LateSent    bool `json:"m1_late_sent"`
	OutageLogSent bool `json:"outage_log_sent"`
	// Milestone 2: saved comparisons and the records sent after load.
	M2Regression          string `json:"m2_regression"`
	M2RegressionGenerated string `json:"m2_regression_generated"`
	M2Reviewed            string `json:"m2_reviewed"`
	M2RepSent             bool   `json:"m2_rep_sent"`
	M2LateLogSent         bool   `json:"m2_late_log_sent"`
	M2ProvSent            bool   `json:"m2_prov_sent"`
	M2FailSent            bool   `json:"m2_fail_sent"`
	M2LateSent            bool   `json:"m2_late_sent"`
}

// fixtureVersion changes whenever the dataset changes; a state file of
// another version must be loaded again.
const fixtureVersion = 3

func newState() (*state, error) {
	run := make([]byte, 4)
	if _, err := rand.Read(run); err != nil {
		return nil, err
	}
	base := time.Now().UTC().Truncate(time.Second)
	return &state{Version: fixtureVersion, Run: hex.EncodeToString(run), BaseUnixNano: base.UnixNano(), Loaded: base.Format(time.RFC3339)}, nil
}

func (s *state) dataset() dataset {
	return dataset{run: s.Run, base: time.Unix(0, s.BaseUnixNano).UTC()}
}

func loadState(path string) (*state, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s (run \"load\" first): %w", path, err)
	}
	var s state
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if s.Version != fixtureVersion {
		return nil, fmt.Errorf("%s is from fixture version %d, this is %d: run \"load\" again", path, s.Version, fixtureVersion)
	}
	return &s, nil
}

func (s *state) save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
