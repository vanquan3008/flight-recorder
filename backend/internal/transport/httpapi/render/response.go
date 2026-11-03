package render

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
)

const RequestIDHeader = "X-Request-ID"

// WriteJSON sends a JSON payload with standard anti-caching and content type headers.
func WriteJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(value)
}

// WriteAPIError sends a standard error envelope containing code, message and request_id.
func WriteAPIError(w http.ResponseWriter, statusCode int, code, message string) {
	WriteJSON(w, statusCode, map[string]any{"error": map[string]string{
		"code":       code,
		"message":    message,
		"request_id": w.Header().Get(RequestIDHeader),
	}})
}

// WriteNotFound answers with a standard 404 resource not found error.
func WriteNotFound(w http.ResponseWriter, resource string) {
	WriteAPIError(w, http.StatusNotFound, "NOT_FOUND", resource+" not found")
}

// WriteInvalidQuery answers with a standard 400 invalid query error.
func WriteInvalidQuery(w http.ResponseWriter, cause error) {
	WriteAPIError(w, http.StatusBadRequest, "INVALID_QUERY", cause.Error())
}

// WriteInternalError logs the underlying cause and answers with a generic 500 or 504.
func WriteInternalError(w http.ResponseWriter, logger *slog.Logger, operation string, cause error) {
	if logger != nil {
		logger.Error(operation, "error", cause)
	}
	if errors.Is(cause, context.DeadlineExceeded) {
		WriteAPIError(w, http.StatusGatewayTimeout, "QUERY_TIMEOUT", "the query took too long; narrow the time range or filters and retry")
		return
	}
	WriteAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}

// DecodeJSONBody safely decodes an incoming JSON object, rejecting unknown fields and multiple documents.
func DecodeJSONBody(r *http.Request, value any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decodeError := decoder.Decode(value); decodeError != nil {
		return decodeError
	}
	if decodeError := decoder.Decode(&struct{}{}); !errors.Is(decodeError, io.EOF) {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}
