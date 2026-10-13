package data

import (
	"regexp"
	"strconv"
	"strings"
)

// Log groups the queries of the API.
type Log struct{}

var StatusCodeMap = map[string]string{
	"unset": "0",
	"error": "1",
	"ok":    "2",
}

// ReverseStatusCodeMap is built once at startup and only read afterwards, so
// concurrent requests can use it safely. (It used to be filled lazily on the
// first call, which raced when requests arrived together.)
var ReverseStatusCodeMap = reverseMap(StatusCodeMap)

func reverseMap(m map[string]string) map[string]string {
	reversed := make(map[string]string, len(m))
	for key, value := range m {
		reversed[value] = key
	}
	return reversed
}

func StatusCodeFromString(status string) string {
	return StatusCodeMap[status]
}

func StatusCodeToString(code string) string {
	return ReverseStatusCodeMap[code]
}

var sqlPlaceholder = regexp.MustCompile(`\$(\d+)`)

// fillSQLArgs replaces $N placeholders with quoted argument values in a single
// pass, so $1 never matches inside $10 and values are never re-substituted.
func fillSQLArgs(statement string, args map[int]string) string {
	return sqlPlaceholder.ReplaceAllStringFunc(statement, func(placeholder string) string {
		argNum, err := strconv.Atoi(placeholder[1:])
		if err != nil {
			return placeholder
		}
		value, ok := args[argNum]
		if !ok {
			return placeholder
		}
		return "'" + value + "'"
	})
}

// arrayLiteral formats values as a ClickHouse Array(String) literal such as
// ['a','b']. The driver only accepts strings for query parameters.
func arrayLiteral(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = quoteString(v)
	}
	return "[" + strings.Join(quoted, ",") + "]"
}

// pairsLiteral formats pairs as a ClickHouse Array(Tuple(String, String))
// literal such as [('a','b')].
func pairsLiteral(pairs [][2]string) string {
	quoted := make([]string, len(pairs))
	for i, p := range pairs {
		quoted[i] = "(" + quoteString(p[0]) + "," + quoteString(p[1]) + ")"
	}
	return "[" + strings.Join(quoted, ",") + "]"
}

var stringEscaper = strings.NewReplacer(`\`, `\\`, `'`, `\'`)

func quoteString(v string) string {
	return "'" + stringEscaper.Replace(v) + "'"
}
