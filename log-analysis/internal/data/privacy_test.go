package data

import (
	"regexp"
	"strings"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

func TestPrivacyRequest(t *testing.T) {
	if err := (&PrivacyRequest{}).Validate(); err == nil {
		t.Error("a scan needs a run or a service")
	}
	q := PrivacyRequest{Service: " app "}
	if err := q.Validate(); err != nil || q.Service != "app" || q.TimeFrom == "" {
		t.Errorf("default window: %v %+v", err, q)
	}
	for _, build := range []func(PrivacyRequest) (string, []any){privacySpanQuery, privacyLogQuery} {
		query, params := build(PrivacyRequest{Run: "r", Service: "s", TimeFrom: "1", TimeTo: "2"})
		bound := map[string]bool{}
		for _, p := range params {
			bound[p.(driver.NamedValue).Name] = true
		}
		for _, m := range namedPlaceholder.FindAllStringSubmatch(query, -1) {
			if !bound[m[1]] {
				t.Errorf("placeholder %q is used but not bound", m[1])
			}
		}
		if strings.Contains(query, "'r'") || strings.Contains(query, "'s'") {
			t.Errorf("run or service inlined:\n%s", query)
		}
	}
}

// The patterns find the leaks of a real integration and leave clean text
// alone (ClickHouse's match is RE2, as Go's regexp).
func TestPrivacyPatterns(t *testing.T) {
	hits := map[string][]string{
		"url_credentials": {"['rtsp://root:pw@10.0.0.5/live']", "postgres://app:s3cret@db/app"},
		"query_secret":    {"/ws?token=SECRET123", "/x?a=1&api_key=k"},
		"bearer_token":    {"Bearer abcdefghijklmnopqrstuvwxyz"},
		"password_hash":   {"$2a$10$abcdefghijklmnopqrstuv", "$argon2id$v=19$m=65536"},
	}
	clean := []string{"rtsp://10.0.0.5/live", "/ws?room=1", "mail a@b.c", "bearer of news", "select * from t where id = ?"}
	for _, k := range privacyKinds {
		re := regexp.MustCompile(k.pattern)
		for _, s := range hits[k.kind] {
			if !re.MatchString(s) {
				t.Errorf("%s misses %q", k.kind, s)
			}
		}
		for _, s := range clean {
			if re.MatchString(s) {
				t.Errorf("%s matches clean %q", k.kind, s)
			}
		}
	}
}
