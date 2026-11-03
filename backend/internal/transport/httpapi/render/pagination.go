package render

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultPageSize = 50
	MaxPageSize     = 100
)

// PageCursor holds the timestamp and unique ID used for cursor pagination.
type PageCursor struct {
	At time.Time `json:"at"`
	ID string    `json:"id"`
}

// ParsePageSize validates and parses a requested page limit.
func ParsePageSize(value string) (int, error) {
	if value == "" {
		return DefaultPageSize, nil
	}
	limit, parseError := strconv.Atoi(value)
	if parseError != nil || limit < 1 || limit > MaxPageSize {
		return 0, errors.New("limit must be between 1 and 100")
	}
	return limit, nil
}

// ParsePagination parses limit and cursor from an HTTP request.
func ParsePagination(request *http.Request, validID func(string) bool) (int, PageCursor, error) {
	limit, parseError := ParsePageSize(request.URL.Query().Get("limit"))
	if parseError != nil {
		return 0, PageCursor{}, parseError
	}
	cursor, parseError := ParsePageCursor(request.URL.Query().Get("cursor"), validID)
	if parseError != nil {
		return 0, PageCursor{}, parseError
	}
	return limit, cursor, nil
}

// ParsePageCursor decodes and verifies a base64 encoded cursor.
func ParsePageCursor(value string, validID func(string) bool) (PageCursor, error) {
	cursor, parseError := DecodeCursor(value)
	if parseError != nil {
		return PageCursor{}, parseError
	}
	if cursor.ID != "" && validID != nil && !validID(cursor.ID) {
		return PageCursor{}, errors.New("cursor is invalid")
	}
	return cursor, nil
}

// DecodeCursor decodes a base64-encoded URL safe page cursor.
func DecodeCursor(value string) (PageCursor, error) {
	if value == "" {
		return PageCursor{}, nil
	}
	decoded, decodeError := base64.RawURLEncoding.DecodeString(value)
	if decodeError != nil {
		return PageCursor{}, errors.New("cursor is invalid")
	}
	var cursor PageCursor
	if jsonError := json.Unmarshal(decoded, &cursor); jsonError != nil || cursor.At.IsZero() || cursor.ID == "" {
		return PageCursor{}, errors.New("cursor is invalid")
	}
	return cursor, nil
}

// EncodeCursor turns a cursor into a base64 URL-safe token.
func EncodeCursor(cursor PageCursor) string {
	encoded, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(encoded)
}

// PageResponse constructs a paginated envelope response with next_cursor when more items follow.
func PageResponse[T any](items []T, limit int, cursor func(T) PageCursor) map[string]any {
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	response := map[string]any{"data": items}
	if hasMore {
		response["next_cursor"] = EncodeCursor(cursor(items[len(items)-1]))
	}
	return response
}

// WritePage writes a paginated JSON response.
func WritePage[T any](w http.ResponseWriter, items []T, limit int, cursor func(T) PageCursor) {
	WriteJSON(w, http.StatusOK, PageResponse(items, limit, cursor))
}

// ValidateTimeRange ensures from timestamp does not exceed to timestamp.
func ValidateTimeRange(from, to time.Time) error {
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return errors.New("from must not be after to")
	}
	return nil
}

// ParseTime parses an RFC3339 string.
func ParseTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, parseError := time.Parse(time.RFC3339Nano, value)
	if parseError != nil {
		return time.Time{}, errors.New("time filters must use RFC3339")
	}
	return parsed, nil
}

// ParseDurationNanos parses a human-readable duration up to 24 hours into nanoseconds.
func ParseDurationNanos(value string) (*int64, error) {
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

// ValidHex checks length and valid hex encoding.
func ValidHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	_, decodeError := hex.DecodeString(value)
	return decodeError == nil
}

// ValidUUID checks whether a string is a valid UUID format.
func ValidUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	return ValidHex(strings.ReplaceAll(value, "-", ""), 32)
}

// RequireUUID validates an identifier is a UUID, returning false and answering 400 if invalid.
func RequireUUID(w http.ResponseWriter, value string) bool {
	if ValidUUID(value) {
		return true
	}
	WriteAPIError(w, http.StatusBadRequest, "INVALID_PATH", "path identifier must be a UUID")
	return false
}
