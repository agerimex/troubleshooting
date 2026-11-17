package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeGitHub keeps the comments of pull request 42 of o/r.
type fakeGitHub struct {
	mu       sync.Mutex
	head     string
	comments []ghComment
	users    map[int64]string // comment id -> user type
	readOnly bool
	calls    []string
	nextID   int64
}

func (f *fakeGitHub) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("authorization %q", r.Header.Get("Authorization"))
		}
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
		var in struct {
			Body string `json:"body"`
		}
		if r.Body != nil {
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &in)
		}
		write := func(v any) { _ = json.NewEncoder(w).Encode(v) }
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/42":
			write(map[string]any{"head": map[string]string{"sha": f.head}})
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/issues/42/comments":
			out := []map[string]any{}
			if r.URL.Query().Get("page") == "1" {
				for _, c := range f.comments {
					out = append(out, map[string]any{"id": c.ID, "body": c.Body, "user": map[string]string{"type": f.users[c.ID]}})
				}
			}
			write(out)
		case f.readOnly:
			w.WriteHeader(http.StatusForbidden)
			write(map[string]string{"message": "Resource not accessible by integration"})
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/issues/42/comments":
			f.nextID++
			f.comments = append(f.comments, ghComment{ID: f.nextID, Body: in.Body})
			f.users[f.nextID] = "Bot"
			w.WriteHeader(http.StatusCreated)
			write(map[string]any{"id": f.nextID})
		case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/repos/o/r/issues/comments/"):
			for i := range f.comments {
				if r.URL.Path == fmt.Sprintf("/repos/o/r/issues/comments/%d", f.comments[i].ID) {
					f.comments[i].Body = in.Body
					write(map[string]any{"id": f.comments[i].ID})
					return
				}
			}
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusTeapot)
		}
	})
}

func commentRun(t *testing.T, api string, md string, extra ...string) (int, string) {
	t.Helper()
	t.Setenv("GITHUB_TOKEN", "tok")
	args := append([]string{"comment", "--api", api, "--repo", "o/r", "--pr", "42", "--markdown", writeFile(t, "s.md", md)}, extra...)
	var out, errOut bytes.Buffer
	code := run(args, &out, &errOut, fixedNow)
	return code, out.String() + errOut.String()
}

// One comment per marker: the first run posts it, later runs update it, and
// a person's comment with the same marker is never edited.
func TestCommentPostsThenUpdates(t *testing.T) {
	f := &fakeGitHub{head: "abc123", users: map[int64]string{}, nextID: 100}
	f.comments = []ghComment{{ID: 7, Body: markerLine("quality-report") + "\nquoted by a person"}}
	f.users[7] = "User"
	s := httptest.NewServer(f.handler(t))
	defer s.Close()

	if code, msg := commentRun(t, s.URL, "### ❌ Quality gate: Failed\n", "--commit", "ABC123"); code != commentPosted || !strings.Contains(msg, "posted") {
		t.Fatalf("first run: %d %s", code, msg)
	}
	if code, msg := commentRun(t, s.URL, "### ✅ Quality gate: Passed\n", "--commit", "abc123"); code != commentPosted || !strings.Contains(msg, "updated comment 101") {
		t.Fatalf("second run: %d %s", code, msg)
	}
	if len(f.comments) != 2 || f.comments[0].Body != markerLine("quality-report")+"\nquoted by a person" ||
		f.comments[1].Body != markerLine("quality-report")+"\n### ✅ Quality gate: Passed\n" {
		t.Errorf("comments: %+v", f.comments)
	}
	// Another marker is another comment.
	if code, _ := commentRun(t, s.URL, "x\n", "--marker", "api"); code != commentPosted || len(f.comments) != 3 {
		t.Errorf("second marker: %+v", f.comments)
	}
}

// A result for a commit that is no longer the head is not posted; a
// read-only token (a fork pull request) skips without an error.
func TestCommentSkips(t *testing.T) {
	f := &fakeGitHub{head: "newhead", users: map[int64]string{}}
	s := httptest.NewServer(f.handler(t))
	defer s.Close()
	if code, msg := commentRun(t, s.URL, "x\n", "--commit", "oldcommit"); code != commentSkipped || !strings.Contains(msg, "oldcommit") || !strings.Contains(msg, "newhead") {
		t.Errorf("stale: %d %s", code, msg)
	}
	f.readOnly = true
	if code, msg := commentRun(t, s.URL, "x\n"); code != commentSkipped || !strings.Contains(msg, "fork") {
		t.Errorf("read-only: %d %s", code, msg)
	}
	if len(f.comments) != 0 {
		t.Errorf("posted: %+v", f.comments)
	}
	// Missing inputs and a broken API are errors.
	t.Setenv("GITHUB_TOKEN", "")
	var out bytes.Buffer
	if code := run([]string{"comment", "--repo", "o/r", "--pr", "42", "--markdown", writeFile(t, "s.md", "x")}, &out, &out, fixedNow); code != commentError {
		t.Errorf("no token: %d", code)
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	defer bad.Close()
	if code, _ := commentRun(t, bad.URL, "x\n"); code != commentError {
		t.Errorf("bad gateway: %d", code)
	}
}

func TestCommentBodyFits(t *testing.T) {
	md := strings.Repeat("| row | of a long table |\n", 4000)
	b := commentBody("m", md)
	if len(b) > 65536 || !strings.HasPrefix(b, markerLine("m")+"\n") || !strings.Contains(b, "cut to fit a comment") {
		t.Errorf("body of %d bytes", len(b))
	}
	if markerLine("a-->b\nc") != "<!-- troubleshooting-quality-gate:ab c -->" {
		t.Errorf("marker: %s", markerLine("a-->b\nc"))
	}
}
