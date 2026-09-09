package httpapi

import (
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/vanquan3008/tracepilot/internal/domain/security"
)

func parseSecurityFilter(request *http.Request) (security.Filter, pageCursor, error) {
	values := request.URL.Query()
	limit, parseError := parsePageSize(values.Get("limit"))
	if parseError != nil {
		return security.Filter{}, pageCursor{}, parseError
	}
	from, parseError := parseTime(values.Get("from"))
	if parseError != nil {
		return security.Filter{}, pageCursor{}, parseError
	}
	to, parseError := parseTime(values.Get("to"))
	if parseError != nil {
		return security.Filter{}, pageCursor{}, parseError
	}
	if rangeError := validateTimeRange(from, to); rangeError != nil {
		return security.Filter{}, pageCursor{}, rangeError
	}
	cursor, parseError := parsePageCursor(values.Get("cursor"), validUUID)
	if parseError != nil {
		return security.Filter{}, pageCursor{}, parseError
	}
	for _, address := range []string{values.Get("sourceIp"), values.Get("destinationIp")} {
		if address != "" && net.ParseIP(address) == nil {
			return security.Filter{}, pageCursor{}, errors.New("security IP filters must be valid addresses")
		}
	}
	return security.Filter{
		Kind: strings.TrimSpace(values.Get("kind")), Severity: strings.ToUpper(strings.TrimSpace(values.Get("severity"))),
		SourceIP: strings.TrimSpace(values.Get("sourceIp")), DestinationIP: strings.TrimSpace(values.Get("destinationIp")),
		Outcome: strings.TrimSpace(values.Get("outcome")), From: from, To: to, Limit: limit,
	}, cursor, nil
}
