package api

import (
	"net/http"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render"
)

// Page is the position in a list a request asks for: how many rows, and where the previous page ended.
type Page struct {
	Limit    int
	Before   time.Time
	CursorID string
}

// ListPage reads the request's page parameters, fetches that page, and writes it with the cursor of
// the next one. fail answers a storage error; ListPage returns right after calling it.
func ListPage[T any](responseWriter http.ResponseWriter, request *http.Request, fetch func(Page) ([]T, error), cursorOf func(T) render.PageCursor, fail func(error)) {
	limit, cursor, parseError := render.ParsePagination(request, telemetry.ValidUUID)
	if parseError != nil {
		render.WriteInvalidQuery(responseWriter, parseError)
		return
	}
	items, fetchError := fetch(Page{Limit: limit, Before: cursor.At, CursorID: cursor.ID})
	if fetchError != nil {
		fail(fetchError)
		return
	}
	render.WritePage(responseWriter, items, limit, cursorOf)
}
