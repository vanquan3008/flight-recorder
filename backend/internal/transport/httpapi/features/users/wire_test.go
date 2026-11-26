package users

import (
	"testing"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render/rendertest"
)

func TestResponsesKeepWireFormat(t *testing.T) {
	rendertest.AssertWireFormat(t, map[string][2]any{
		"created user without generated password": {createUserResponse{User: auth.User{ID: "u"}}, map[string]any{"user": auth.User{ID: "u"}}},
		"created user with generated password":    {createUserResponse{User: auth.User{ID: "u"}, TemporaryPassword: "pw"}, map[string]any{"user": auth.User{ID: "u"}, "temporary_password": "pw"}},
		"temporary password":                      {temporaryPasswordResponse{TemporaryPassword: "pw"}, map[string]any{"temporary_password": "pw"}},
	})
}
