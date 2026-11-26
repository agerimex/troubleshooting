package otlp

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"log-receiver/internal/data"

	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
)

// Logs converts an OTLP logs export request into log records for
// InsertLogRecords. One rule per field, nothing inferred:
//   - UnixTime: time_unix_nano, else observed_time_unix_nano; neither: rejected;
//   - TraceId, SpanId: bytes as lower-case hex; empty or all zeros (a record
//     written outside a span) is "not recorded"; any other wrong length:
//     rejected;
//   - ServiceName: the resource's service.name, the same value its spans get
//     (unknown_service when the SDK sends none);
//   - Severity: severity_text normalized (data.NormalizeSeverity, which knows
//     Serilog's and .NET's names); empty text: from severity_number by the OTLP
//     ranges; 0: empty;
//   - Body: a string as is, any other value as text (maps and arrays as JSON);
//   - Attributes: the record's own, as strings (maps and arrays as JSON);
//     resource attributes other than service.name are not added;
//   - RecordId: the log.record.uid attribute if present, otherwise a hash of
//     the record (RecordIDOf), so an exporter's retry of the same batch is
//     read once. Two truly identical records with the same nanosecond are
//     therefore also read as one.
func Logs(resourceLogs []*logspb.ResourceLogs) (records []data.LogRecord, rejected int64) {
	for _, rl := range resourceLogs {
		service := serviceNameOf(rl.GetResource())
		for _, sl := range rl.GetScopeLogs() {
			for _, lr := range sl.GetLogRecords() {
				record, ok := logRecord(lr, service)
				if !ok {
					rejected++
					continue
				}
				records = append(records, record)
			}
		}
	}
	return records, rejected
}

// LogRecordUIDAttribute is the OpenTelemetry attribute that identifies a log
// record (semantic conventions: log.record.uid).
const LogRecordUIDAttribute = "log.record.uid"

func logRecord(lr *logspb.LogRecord, service string) (data.LogRecord, bool) {
	t := lr.GetTimeUnixNano()
	if t == 0 {
		t = lr.GetObservedTimeUnixNano()
	}
	if t == 0 || t > 1<<63-1 {
		return data.LogRecord{}, false
	}
	traceID, ok := logID(lr.GetTraceId(), 16)
	if !ok {
		return data.LogRecord{}, false
	}
	spanID, ok := logID(lr.GetSpanId(), 8)
	if !ok {
		return data.LogRecord{}, false
	}
	attributes := make(map[string]string, len(lr.GetAttributes()))
	for _, kv := range lr.GetAttributes() {
		attributes[cleanUTF8(kv.GetKey())] = cleanUTF8(valueToString(kv.GetValue()))
	}
	r := data.LogRecord{
		UnixTime:    int64(t),
		TraceId:     traceID,
		SpanId:      spanID,
		Severity:    severityOf(lr),
		ServiceName: cleanUTF8(service),
		Body:        cleanUTF8(valueToString(lr.GetBody())),
		Attributes:  attributes,
	}
	r.RecordId = attributes[LogRecordUIDAttribute]
	if r.RecordId == "" || len(r.RecordId) > 128 {
		r.RecordId = RecordIDOf(r)
	}
	return r, true
}

// logID formats a trace (16 bytes) or span (8 bytes) id. Empty and all-zero
// ids mean the record has none; any other length is invalid.
func logID(id []byte, size int) (string, bool) {
	if len(id) == 0 {
		return "", true
	}
	if len(id) != size {
		return "", false
	}
	for _, b := range id {
		if b != 0 {
			return hex.EncodeToString(id), true
		}
	}
	return "", true
}

// severityNames are the OTLP severity number ranges (1-4 TRACE ... 21-24 FATAL).
var severityNames = []string{"TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL"}

func severityOf(lr *logspb.LogRecord) string {
	if text := strings.TrimSpace(lr.GetSeverityText()); text != "" {
		return data.NormalizeSeverity(cleanUTF8(text))
	}
	n := int(lr.GetSeverityNumber())
	if n < 1 || n > 24 {
		return ""
	}
	return severityNames[(n-1)/4]
}

// RecordIDOf is a record's identity when the sender gave none: SHA-256 (hex,
// first 32 characters) over service, time, trace id, span id, severity, body
// and the attributes sorted by key. An exporter that retries a batch sends the
// same records, which get the same ids, and reads keep one row per id.
func RecordIDOf(r data.LogRecord) string {
	h := sha256.New()
	write := func(s string) {
		h.Write([]byte(strconv.Itoa(len(s))))
		h.Write([]byte{':'})
		h.Write([]byte(s))
	}
	write(r.ServiceName)
	write(strconv.FormatInt(r.UnixTime, 10))
	write(r.TraceId)
	write(r.SpanId)
	write(r.Severity)
	write(r.Body)
	keys := make([]string, 0, len(r.Attributes))
	for k := range r.Attributes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		write(k)
		write(r.Attributes[k])
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}

func cleanUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return strings.ToValidUTF8(s, "�")
}
