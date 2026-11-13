package httpapi

import (
	"net/http"
	"time"

	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render"
)

type pageCursor = render.PageCursor

const (
	defaultPageSize   = render.DefaultPageSize
	maxPageSize       = render.MaxPageSize
	maxLogSearchRange = 24 * time.Hour
)

func pageResponse[T any](items []T, limit int, cursor func(T) pageCursor) map[string]any {
	return render.PageResponse(items, limit, cursor)
}

var (
	parsePagination    = render.ParsePagination
	parsePageCursor    = render.ParsePageCursor
	parsePageSize      = render.ParsePageSize
	decodeCursor       = render.DecodeCursor
	encodeCursor       = render.EncodeCursor
	validateTimeRange  = render.ValidateTimeRange
	parseTime          = render.ParseTime
	parseDurationNanos = render.ParseDurationNanos
	validHex           = render.ValidHex
	validUUID          = render.ValidUUID
	requireUUID        = render.RequireUUID
)

func writeAPIError(responseWriter http.ResponseWriter, statusCode int, code, message string) {
	render.WriteAPIError(responseWriter, statusCode, code, message)
}

func writeJSONValue(responseWriter http.ResponseWriter, statusCode int, value any) {
	render.WriteJSON(responseWriter, statusCode, value)
}

func decodeJSONBody(request *http.Request, value any) error {
	return render.DecodeJSONBody(request, value)
}

func (server *Server) writeInternalError(responseWriter http.ResponseWriter, operation string, cause error) {
	render.WriteInternalError(responseWriter, server.logger, operation, cause)
}

func writeInvalidQuery(responseWriter http.ResponseWriter, cause error) {
	render.WriteInvalidQuery(responseWriter, cause)
}

func writeNotFound(responseWriter http.ResponseWriter, resource string) {
	render.WriteNotFound(responseWriter, resource)
}

func writePage[T any](responseWriter http.ResponseWriter, items []T, limit int, cursor func(T) pageCursor) {
	render.WritePage(responseWriter, items, limit, cursor)
}
