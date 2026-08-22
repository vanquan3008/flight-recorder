package capture

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type Request struct {
	Method string
	Target string
	Header http.Header
	Body   []byte
}

func Sanitize(method, target string, headers http.Header, body []byte, maxBodyBytes int64) (Request, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet && method != http.MethodHead && method != http.MethodPost && method != http.MethodPut && method != http.MethodPatch {
		return Request{}, errors.New("replay method is not allowed")
	}
	if int64(len(body)) > maxBodyBytes {
		return Request{}, errors.New("replay request body exceeds the configured limit")
	}
	request := Request{Method: method, Target: target, Header: make(http.Header)}
	for _, name := range []string{"Accept", "Content-Type", "X-Request-ID"} {
		if value := headers.Get(name); value != "" {
			request.Header.Set(name, value)
		}
	}
	if len(body) == 0 || !strings.HasPrefix(strings.ToLower(request.Header.Get("Content-Type")), "application/json") {
		return request, nil
	}
	var value any
	if decodeError := json.Unmarshal(body, &value); decodeError != nil {
		return Request{}, errors.New("replay JSON body is invalid")
	}
	redact(value)
	encoded, encodeError := json.Marshal(value)
	if encodeError != nil {
		return Request{}, errors.New("replay JSON body cannot be captured")
	}
	request.Body = encoded
	return request, nil
}

func redact(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if sensitive(key) {
				typed[key] = "[REDACTED]"
				continue
			}
			redact(child)
		}
	case []any:
		for _, child := range typed {
			redact(child)
		}
	}
}

func sensitive(key string) bool {
	key = strings.ToLower(key)
	return strings.Contains(key, "password") || strings.Contains(key, "secret") || strings.Contains(key, "token") || strings.Contains(key, "authorization") || strings.Contains(key, "api_key")
}
