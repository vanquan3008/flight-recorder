package incident

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

// messageVariables replaces values that differ between occurrences of the same
// failure. Order matters: broader shapes (UUID, timestamp, address) must be
// replaced before the generic number rule splits them into pieces.
var messageVariables = []struct {
	pattern     *regexp.Regexp
	placeholder string
}{
	{regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`), "<uuid>"},
	{regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:?\d{2})?`), "<time>"},
	{regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`), "<email>"},
	{regexp.MustCompile(`\b\d{1,3}(\.\d{1,3}){3}(:\d+)?\b`), "<ip>"},
}

var (
	hexToken = regexp.MustCompile(`\b(0x)?[0-9a-fA-F]{8,}\b`)
	// No trailing boundary: "1.5s" and "200ms" carry a unit after the value.
	numberToken = regexp.MustCompile(`\b\d+(\.\d+)?`)
)

// Fingerprint groups occurrences of one failure. The message is normalized so
// "user 123 not found" and "user 456 not found" open a single incident.
func Fingerprint(event telemetry.Event) string {
	identity := strings.Join([]string{
		event.ServiceName,
		event.Operation,
		event.ErrorType,
		normalizeMessage(event.ErrorMessage),
		strconv.Itoa(event.HTTPStatus),
	}, "\x00")
	digest := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(digest[:])
}

func normalizeMessage(message string) string {
	for _, variable := range messageVariables {
		message = variable.pattern.ReplaceAllString(message, variable.placeholder)
	}
	// A hex run without digits is usually a word ("deadline"), not an identifier.
	message = hexToken.ReplaceAllStringFunc(message, func(token string) string {
		if strings.ContainsAny(token, "0123456789") {
			return "<hex>"
		}
		return token
	})
	message = numberToken.ReplaceAllString(message, "<n>")
	return strings.Join(strings.Fields(message), " ")
}
