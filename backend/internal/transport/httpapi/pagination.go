package httpapi

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/incident"
	"github.com/vanquan3008/tracepilot/internal/query"
)

const (
	defaultPageSize   = 50
	maxPageSize       = 100
	maxLogSearchRange = 24 * time.Hour
)

type pageCursor struct {
	At time.Time `json:"at"`
	ID string    `json:"id"`
}

func parseTraceFilter(request *http.Request) (query.TraceFilter, pageCursor, error) {
	values := request.URL.Query()
	limit, parseError := parsePageSize(values.Get("limit"))
	if parseError != nil {
		return query.TraceFilter{}, pageCursor{}, parseError
	}
	from, parseError := parseTime(values.Get("from"))
	if parseError != nil {
		return query.TraceFilter{}, pageCursor{}, parseError
	}
	to, parseError := parseTime(values.Get("to"))
	if parseError != nil {
		return query.TraceFilter{}, pageCursor{}, parseError
	}
	if rangeError := validateTimeRange(from, to); rangeError != nil {
		return query.TraceFilter{}, pageCursor{}, rangeError
	}
	minimumDuration, parseError := parseDurationNanos(values.Get("minDuration"))
	if parseError != nil {
		return query.TraceFilter{}, pageCursor{}, parseError
	}
	maximumDuration, parseError := parseDurationNanos(values.Get("maxDuration"))
	if parseError != nil {
		return query.TraceFilter{}, pageCursor{}, parseError
	}
	if minimumDuration != nil && maximumDuration != nil && *minimumDuration > *maximumDuration {
		return query.TraceFilter{}, pageCursor{}, errors.New("minDuration must not exceed maxDuration")
	}
	cursor, parseError := parsePageCursor(values.Get("cursor"), validTraceID)
	if parseError != nil {
		return query.TraceFilter{}, pageCursor{}, parseError
	}
	traceID := strings.TrimSpace(values.Get("traceId"))
	if traceID != "" && !validTraceID(traceID) {
		return query.TraceFilter{}, pageCursor{}, errors.New("traceId must be 16 bytes of hexadecimal")
	}
	return query.TraceFilter{Service: values.Get("service"), Operation: values.Get("operation"), Status: values.Get("status"), TraceID: traceID, From: from, To: to, MinDurationNs: minimumDuration, MaxDurationNs: maximumDuration, Limit: limit}, cursor, nil
}

func parseLogFilter(request *http.Request) (query.LogFilter, pageCursor, error) {
	values := request.URL.Query()
	limit, parseError := parsePageSize(values.Get("limit"))
	if parseError != nil {
		return query.LogFilter{}, pageCursor{}, parseError
	}
	from, parseError := parseTime(values.Get("from"))
	if parseError != nil {
		return query.LogFilter{}, pageCursor{}, parseError
	}
	to, parseError := parseTime(values.Get("to"))
	if parseError != nil {
		return query.LogFilter{}, pageCursor{}, parseError
	}
	if rangeError := validateTimeRange(from, to); rangeError != nil {
		return query.LogFilter{}, pageCursor{}, rangeError
	}
	search := strings.TrimSpace(values.Get("search"))
	if search != "" && (from.IsZero() || to.IsZero()) {
		return query.LogFilter{}, pageCursor{}, errors.New("log content search requires from and to")
	}
	if search != "" && to.Sub(from) > maxLogSearchRange {
		return query.LogFilter{}, pageCursor{}, errors.New("log content search range must not exceed 24 hours")
	}
	cursor, parseError := parsePageCursor(values.Get("cursor"), validUUID)
	if parseError != nil {
		return query.LogFilter{}, pageCursor{}, parseError
	}
	traceID := strings.TrimSpace(values.Get("traceId"))
	if traceID != "" && !validTraceID(traceID) {
		return query.LogFilter{}, pageCursor{}, errors.New("traceId must be 16 bytes of hexadecimal")
	}
	spanID := strings.TrimSpace(values.Get("spanId"))
	if spanID != "" && !validSpanID(spanID) {
		return query.LogFilter{}, pageCursor{}, errors.New("spanId must be 8 bytes of hexadecimal")
	}
	return query.LogFilter{Service: values.Get("service"), Severity: values.Get("severity"), TraceID: traceID, SpanID: spanID, Search: search, From: from, To: to, Limit: limit}, cursor, nil
}

func parseIncidentFilter(request *http.Request) (incident.Filter, pageCursor, error) {
	values := request.URL.Query()
	limit, parseError := parsePageSize(values.Get("limit"))
	if parseError != nil {
		return incident.Filter{}, pageCursor{}, parseError
	}
	from, parseError := parseTime(values.Get("from"))
	if parseError != nil {
		return incident.Filter{}, pageCursor{}, parseError
	}
	to, parseError := parseTime(values.Get("to"))
	if parseError != nil {
		return incident.Filter{}, pageCursor{}, parseError
	}
	if rangeError := validateTimeRange(from, to); rangeError != nil {
		return incident.Filter{}, pageCursor{}, rangeError
	}
	status := strings.ToUpper(strings.TrimSpace(values.Get("status")))
	if status != "" {
		if _, valid := incident.NormalizeStatus(status); !valid {
			return incident.Filter{}, pageCursor{}, errors.New("incident status is invalid")
		}
	}
	cursor, parseError := parsePageCursor(values.Get("cursor"), validUUID)
	if parseError != nil {
		return incident.Filter{}, pageCursor{}, parseError
	}
	return incident.Filter{
		Service: strings.TrimSpace(values.Get("service")), Severity: strings.ToUpper(strings.TrimSpace(values.Get("severity"))),
		Status: status, From: from, To: to, Limit: limit,
	}, cursor, nil
}

func validateTimeRange(from, to time.Time) error {
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return errors.New("from must not be after to")
	}
	return nil
}

func parsePageSize(value string) (int, error) {
	if value == "" {
		return defaultPageSize, nil
	}
	limit, parseError := strconv.Atoi(value)
	if parseError != nil || limit < 1 || limit > maxPageSize {
		return 0, errors.New("limit must be between 1 and 100")
	}
	return limit, nil
}

func parsePagination(request *http.Request, validID func(string) bool) (int, pageCursor, error) {
	limit, parseError := parsePageSize(request.URL.Query().Get("limit"))
	if parseError != nil {
		return 0, pageCursor{}, parseError
	}
	cursor, parseError := parsePageCursor(request.URL.Query().Get("cursor"), validID)
	if parseError != nil {
		return 0, pageCursor{}, parseError
	}
	return limit, cursor, nil
}

func parseTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, parseError := time.Parse(time.RFC3339Nano, value)
	if parseError != nil {
		return time.Time{}, errors.New("time filters must use RFC3339")
	}
	return parsed, nil
}

func parseDurationNanos(value string) (*int64, error) {
	if value == "" {
		return nil, nil
	}
	duration, parseError := time.ParseDuration(value)
	if parseError != nil || duration < 0 || duration > 24*time.Hour {
		return nil, errors.New("duration filters must use a non-negative value up to 24h (for example 250ms)")
	}
	nanos := duration.Nanoseconds()
	return &nanos, nil
}

func decodeCursor(value string) (pageCursor, error) {
	if value == "" {
		return pageCursor{}, nil
	}
	decoded, decodeError := base64.RawURLEncoding.DecodeString(value)
	if decodeError != nil {
		return pageCursor{}, errors.New("cursor is invalid")
	}
	var cursor pageCursor
	if jsonError := json.Unmarshal(decoded, &cursor); jsonError != nil || cursor.At.IsZero() || cursor.ID == "" {
		return pageCursor{}, errors.New("cursor is invalid")
	}
	return cursor, nil
}

func parsePageCursor(value string, validID func(string) bool) (pageCursor, error) {
	cursor, parseError := decodeCursor(value)
	if parseError != nil {
		return pageCursor{}, parseError
	}
	if parseError := validateCursorID(cursor, validID); parseError != nil {
		return pageCursor{}, parseError
	}
	return cursor, nil
}

func validateCursorID(cursor pageCursor, valid func(string) bool) error {
	if cursor.ID != "" && !valid(cursor.ID) {
		return errors.New("cursor is invalid")
	}
	return nil
}

func encodeCursor(cursor pageCursor) string {
	encoded, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func pageResponse[T any](items []T, limit int, cursor func(T) pageCursor) map[string]any {
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	response := map[string]any{"data": items}
	if hasMore {
		response["next_cursor"] = encodeCursor(cursor(items[len(items)-1]))
	}
	return response
}

func validHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	_, decodeError := hex.DecodeString(value)
	return decodeError == nil
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	return validHex(strings.ReplaceAll(value, "-", ""), 32)
}

func validTraceID(value string) bool { return validHex(value, 32) }

func validSpanID(value string) bool { return validHex(value, 16) }

func requireUUID(responseWriter http.ResponseWriter, value string) bool {
	if validUUID(value) {
		return true
	}
	writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_PATH", "path identifier must be a UUID")
	return false
}

func requireTraceID(responseWriter http.ResponseWriter, value string) bool {
	if validTraceID(value) {
		return true
	}
	writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_PATH", "trace identifier must be 16 bytes of hexadecimal")
	return false
}
