package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// troubleshooting-check comment posts the Markdown summary as one pull
// request comment and updates that comment on later runs, so retries never
// add a second, contradicting comment. It never posts a result for a commit
// that is no longer the pull request's head.
//
//	troubleshooting-check comment --markdown summary.md --repo owner/name --pr 42 \
//	    [--commit <tested sha>] [--marker quality-report] [--api https://api.github.com]
//
// The token is read from GITHUB_TOKEN. Exit 0 posted or updated, 2 skipped
// (no permission, e.g. a fork pull request, or a stale commit), 1 error.

const (
	commentPosted  = 0
	commentError   = 1
	commentSkipped = 2

	// GitHub's limit is 65 536 characters; keep room for the marker and note.
	maxCommentBody = 60000
	maxCommentPage = 10 // pages of 100 comments searched for our comment
)

type commentOptions struct {
	markdown, repo, marker, api, commit, token string
	pr                                         int
	timeout                                    time.Duration
}

type ghComment struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
	User struct {
		Login string `json:"login"`
		Type  string `json:"type"`
	} `json:"user"`
}

var errNoPermission = errors.New("no permission")

func runComment(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("troubleshooting-check comment", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var o commentOptions
	fs.StringVar(&o.markdown, "markdown", "", "the Markdown summary to post (written by --markdown)")
	fs.StringVar(&o.repo, "repo", os.Getenv("GITHUB_REPOSITORY"), "repository owner/name")
	fs.IntVar(&o.pr, "pr", 0, "pull request number")
	fs.StringVar(&o.commit, "commit", "", "the tested commit: nothing is posted if the pull request's head is another commit")
	fs.StringVar(&o.marker, "marker", "quality-report", "identifies this gate's comment (one comment per marker)")
	fs.StringVar(&o.api, "api", envOr("GITHUB_API_URL", "https://api.github.com"), "GitHub API URL")
	fs.DurationVar(&o.timeout, "timeout", time.Minute, "timeout for all requests")
	if err := fs.Parse(args); err != nil {
		return commentError
	}
	o.token = os.Getenv("GITHUB_TOKEN")
	if o.markdown == "" || o.pr <= 0 || !strings.Contains(o.repo, "/") || o.token == "" {
		fmt.Fprintln(stderr, "error: --markdown, --pr, --repo (owner/name) and GITHUB_TOKEN are required")
		return commentError
	}
	md, err := os.ReadFile(o.markdown)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return commentError
	}
	ctx, cancel := context.WithTimeout(context.Background(), o.timeout)
	defer cancel()
	msg, code := postComment(ctx, o, commentBody(o.marker, string(md)))
	if code == commentError {
		fmt.Fprintln(stderr, "error:", msg)
	} else {
		fmt.Fprintln(stdout, msg)
	}
	return code
}

func markerLine(marker string) string {
	return "<!-- troubleshooting-quality-gate:" + strings.NewReplacer("-->", "", "\n", " ").Replace(marker) + " -->"
}

// commentBody is the marker line plus the summary, cut to GitHub's limit at
// a line end.
func commentBody(marker, md string) string {
	if len(md) > maxCommentBody {
		cut := strings.LastIndex(md[:maxCommentBody], "\n")
		if cut < 0 {
			cut = maxCommentBody
		}
		md = md[:cut] + "\n\n… cut to fit a comment; the full report is in the job summary and the artifact.\n"
	}
	return markerLine(marker) + "\n" + md
}

func postComment(ctx context.Context, o commentOptions, body string) (string, int) {
	gh := ghClient{api: strings.TrimRight(o.api, "/"), token: o.token}
	if o.commit != "" {
		var pr struct {
			Head struct {
				SHA string `json:"sha"`
			} `json:"head"`
		}
		if err := gh.do(ctx, http.MethodGet, "/repos/"+o.repo+"/pulls/"+strconv.Itoa(o.pr), nil, &pr); err != nil {
			return "reading the pull request: " + err.Error(), commentError
		}
		if pr.Head.SHA != "" && !strings.EqualFold(pr.Head.SHA, o.commit) {
			return fmt.Sprintf("not posted: the result is for commit %s, the pull request's head is now %s", short(o.commit), short(pr.Head.SHA)), commentSkipped
		}
	}
	existing, err := gh.findComment(ctx, o.repo, o.pr, markerLine(o.marker), gh.login(ctx))
	if err != nil {
		return "listing comments: " + err.Error(), commentError
	}
	payload := map[string]string{"body": body}
	if existing != 0 {
		err = gh.do(ctx, http.MethodPatch, "/repos/"+o.repo+"/issues/comments/"+strconv.FormatInt(existing, 10), payload, nil)
		if err == nil {
			return fmt.Sprintf("updated comment %d on #%d", existing, o.pr), commentPosted
		}
		if !errors.Is(err, errNoPermission) {
			return "updating the comment: " + err.Error(), commentError
		}
	}
	err = gh.do(ctx, http.MethodPost, "/repos/"+o.repo+"/issues/"+strconv.Itoa(o.pr)+"/comments", payload, nil)
	switch {
	case err == nil:
		return "posted a comment on #" + strconv.Itoa(o.pr), commentPosted
	case errors.Is(err, errNoPermission):
		return "not posted: the token may not comment (a pull request from a fork gets a read-only token; grant pull-requests: write otherwise). The summary is in the job summary.", commentSkipped
	}
	return "posting the comment: " + err.Error(), commentError
}

type ghClient struct{ api, token string }

// login is the token's own user: a personal access token can read it, the
// workflow's GITHUB_TOKEN can't ("", the comments are then github-actions').
func (g ghClient) login(ctx context.Context) string {
	var u struct {
		Login string `json:"login"`
	}
	if err := g.do(ctx, http.MethodGet, "/user", nil, &u); err != nil {
		return ""
	}
	return u.Login
}

// findComment returns the id of our comment that starts with the marker line,
// or 0. Ours is the token's own user's (login), or a bot's when the token has
// no user (GITHUB_TOKEN). Comments by anyone else are never edited, even with
// the marker.
func (g ghClient) findComment(ctx context.Context, repo string, pr int, marker, login string) (int64, error) {
	for page := 1; page <= maxCommentPage; page++ {
		var list []ghComment
		path := fmt.Sprintf("/repos/%s/issues/%d/comments?per_page=100&page=%d", repo, pr, page)
		if err := g.do(ctx, http.MethodGet, path, nil, &list); err != nil {
			return 0, err
		}
		for _, c := range list {
			ours := c.User.Type == "Bot"
			if login != "" {
				ours = strings.EqualFold(c.User.Login, login)
			}
			if ours && strings.HasPrefix(c.Body, marker) {
				return c.ID, nil
			}
		}
		if len(list) < 100 {
			break
		}
	}
	return 0, nil
}

func (g ghClient) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, g.api+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "troubleshooting-check")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("%w: HTTP %d", errNoPermission, resp.StatusCode)
	}
	if resp.StatusCode/100 != 2 {
		var m struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &m)
		return fmt.Errorf("HTTP %d %s", resp.StatusCode, m.Message)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("not a GitHub API answer: %w", err)
		}
	}
	return nil
}
