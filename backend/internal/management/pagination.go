package management

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/postgres"
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

func parseTraceFilter(request *http.Request) (postgres.TraceFilter, pageCursor, error) {
	query := request.URL.Query()
	limit, parseError := parsePageSize(query.Get("limit"))
	if parseError != nil {
		return postgres.TraceFilter{}, pageCursor{}, parseError
	}
	from, parseError := parseTime(query.Get("from"))
	if parseError != nil {
		return postgres.TraceFilter{}, pageCursor{}, parseError
	}
	to, parseError := parseTime(query.Get("to"))
	if parseError != nil {
		return postgres.TraceFilter{}, pageCursor{}, parseError
	}
	if rangeError := validateTimeRange(from, to); rangeError != nil {
		return postgres.TraceFilter{}, pageCursor{}, rangeError
	}
	cursor, parseError := decodeCursor(query.Get("cursor"))
	if parseError != nil {
		return postgres.TraceFilter{}, pageCursor{}, parseError
	}
	if parseError := validateCursorID(cursor, validTraceID); parseError != nil {
		return postgres.TraceFilter{}, pageCursor{}, parseError
	}
	traceID := strings.TrimSpace(query.Get("traceId"))
	if traceID != "" && !validTraceID(traceID) {
		return postgres.TraceFilter{}, pageCursor{}, errors.New("traceId must be 16 bytes of hexadecimal")
	}
	return postgres.TraceFilter{Service: query.Get("service"), Operation: query.Get("operation"), Status: query.Get("status"), TraceID: traceID, From: from, To: to, Limit: limit}, cursor, nil
}

func parseLogFilter(request *http.Request) (postgres.LogFilter, pageCursor, error) {
	query := request.URL.Query()
	limit, parseError := parsePageSize(query.Get("limit"))
	if parseError != nil {
		return postgres.LogFilter{}, pageCursor{}, parseError
	}
	from, parseError := parseTime(query.Get("from"))
	if parseError != nil {
		return postgres.LogFilter{}, pageCursor{}, parseError
	}
	to, parseError := parseTime(query.Get("to"))
	if parseError != nil {
		return postgres.LogFilter{}, pageCursor{}, parseError
	}
	if rangeError := validateTimeRange(from, to); rangeError != nil {
		return postgres.LogFilter{}, pageCursor{}, rangeError
	}
	search := strings.TrimSpace(query.Get("search"))
	if search != "" && (from.IsZero() || to.IsZero()) {
		return postgres.LogFilter{}, pageCursor{}, errors.New("log content search requires from and to")
	}
	if search != "" && to.Sub(from) > maxLogSearchRange {
		return postgres.LogFilter{}, pageCursor{}, errors.New("log content search range must not exceed 24 hours")
	}
	cursor, parseError := decodeCursor(query.Get("cursor"))
	if parseError != nil {
		return postgres.LogFilter{}, pageCursor{}, parseError
	}
	if parseError := validateCursorID(cursor, validUUID); parseError != nil {
		return postgres.LogFilter{}, pageCursor{}, parseError
	}
	traceID := strings.TrimSpace(query.Get("traceId"))
	if traceID != "" && !validTraceID(traceID) {
		return postgres.LogFilter{}, pageCursor{}, errors.New("traceId must be 16 bytes of hexadecimal")
	}
	spanID := strings.TrimSpace(query.Get("spanId"))
	if spanID != "" && !validSpanID(spanID) {
		return postgres.LogFilter{}, pageCursor{}, errors.New("spanId must be 8 bytes of hexadecimal")
	}
	return postgres.LogFilter{Service: query.Get("service"), Severity: query.Get("severity"), TraceID: traceID, SpanID: spanID, Search: search, From: from, To: to, Limit: limit}, cursor, nil
}

func parseIncidentFilter(request *http.Request) (postgres.IncidentFilter, pageCursor, error) {
	query := request.URL.Query()
	limit, parseError := parsePageSize(query.Get("limit"))
	if parseError != nil {
		return postgres.IncidentFilter{}, pageCursor{}, parseError
	}
	from, parseError := parseTime(query.Get("from"))
	if parseError != nil {
		return postgres.IncidentFilter{}, pageCursor{}, parseError
	}
	to, parseError := parseTime(query.Get("to"))
	if parseError != nil {
		return postgres.IncidentFilter{}, pageCursor{}, parseError
	}
	if rangeError := validateTimeRange(from, to); rangeError != nil {
		return postgres.IncidentFilter{}, pageCursor{}, rangeError
	}
	status := strings.ToUpper(strings.TrimSpace(query.Get("status")))
	if status != "" {
		if _, valid := postgres.NormalizeIncidentStatus(status); !valid {
			return postgres.IncidentFilter{}, pageCursor{}, errors.New("incident status is invalid")
		}
	}
	cursor, parseError := decodeCursor(query.Get("cursor"))
	if parseError != nil {
		return postgres.IncidentFilter{}, pageCursor{}, parseError
	}
	if parseError := validateCursorID(cursor, validUUID); parseError != nil {
		return postgres.IncidentFilter{}, pageCursor{}, parseError
	}
	return postgres.IncidentFilter{
		Service: strings.TrimSpace(query.Get("service")), Severity: strings.ToUpper(strings.TrimSpace(query.Get("severity"))),
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

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	_, decodeError := hex.DecodeString(strings.ReplaceAll(value, "-", ""))
	return decodeError == nil
}

func validTraceID(value string) bool {
	if len(value) != 32 {
		return false
	}
	_, decodeError := hex.DecodeString(value)
	return decodeError == nil
}

func validSpanID(value string) bool {
	if len(value) != 16 {
		return false
	}
	_, decodeError := hex.DecodeString(value)
	return decodeError == nil
}

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
