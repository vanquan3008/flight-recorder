package httpapi

import (
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
