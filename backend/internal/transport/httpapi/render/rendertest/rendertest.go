// Package rendertest reads the responses render writes, for the tests of the API packages.
package rendertest

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

// ErrorCode is the code of the error envelope in a response.
func ErrorCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct{ Code string } `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("response %q is not an error body: %v", recorder.Body.String(), err)
	}
	return body.Error.Code
}

// AssertWireFormat proves each response type encodes exactly the JSON the value it replaced did.
func AssertWireFormat(t *testing.T, cases map[string][2]any) {
	t.Helper()
	for name, pair := range cases {
		typed, typedError := json.Marshal(pair[0])
		old, oldError := json.Marshal(pair[1])
		if typedError != nil || oldError != nil {
			t.Fatalf("%s: marshal errors %v %v", name, typedError, oldError)
		}
		if string(typed) != string(old) {
			t.Errorf("%s: typed %s, previous %s", name, typed, old)
		}
	}
}
