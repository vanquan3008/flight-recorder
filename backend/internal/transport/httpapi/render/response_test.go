package render

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteAPIErrorIncludesRequestID(t *testing.T) {
	response := httptest.NewRecorder()
	response.Header().Set(RequestIDHeader, "request-123")
	WriteAPIError(response, http.StatusBadRequest, "INVALID_REQUEST", "bad input")

	var body struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if decodeError := json.NewDecoder(response.Body).Decode(&body); decodeError != nil {
		t.Fatal(decodeError)
	}
	if body.Error.Code != "INVALID_REQUEST" || body.Error.Message != "bad input" || body.Error.RequestID != "request-123" {
		t.Fatalf("error response = %#v", body.Error)
	}
}
