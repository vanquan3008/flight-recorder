package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

func writeAPIError(responseWriter http.ResponseWriter, statusCode int, code, message string) {
	writeJSONValue(responseWriter, statusCode, map[string]any{"error": map[string]string{
		"code":       code,
		"message":    message,
		"request_id": responseWriter.Header().Get(requestIDHeader),
	}})
}

func writeJSONValue(responseWriter http.ResponseWriter, statusCode int, value any) {
	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.Header().Set("Cache-Control", "no-store")
	responseWriter.WriteHeader(statusCode)
	_ = json.NewEncoder(responseWriter).Encode(value)
}

func decodeJSONBody(request *http.Request, value any) error {
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if decodeError := decoder.Decode(value); decodeError != nil {
		return decodeError
	}
	if decodeError := decoder.Decode(&struct{}{}); !errors.Is(decodeError, io.EOF) {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}

// writeInternalError logs the cause and answers with a generic 500, so storage
// details never reach the client.
func (server *Server) writeInternalError(responseWriter http.ResponseWriter, operation string, cause error) {
	server.logger.Error(operation, "error", cause)
	if errors.Is(cause, context.DeadlineExceeded) {
		writeAPIError(responseWriter, http.StatusGatewayTimeout, "QUERY_TIMEOUT", "the query took too long; narrow the time range or filters and retry")
		return
	}
	writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}

func writeInvalidQuery(responseWriter http.ResponseWriter, cause error) {
	writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", cause.Error())
}

func writeNotFound(responseWriter http.ResponseWriter, resource string) {
	writeAPIError(responseWriter, http.StatusNotFound, "NOT_FOUND", resource+" not found")
}

// writePage answers with one page of items and, when more exist, the cursor
// of the last item returned.
func writePage[T any](responseWriter http.ResponseWriter, items []T, limit int, cursor func(T) pageCursor) {
	writeJSONValue(responseWriter, http.StatusOK, pageResponse(items, limit, cursor))
}
