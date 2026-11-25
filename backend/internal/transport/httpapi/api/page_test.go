package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render"
)

type row struct {
	At time.Time
	ID string
}

func rowCursor(item row) render.PageCursor { return render.PageCursor{At: item.At, ID: item.ID} }

func TestListPageFetchesTheRequestedPageAndWritesTheNextCursor(t *testing.T) {
	at := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	cursor := render.EncodeCursor(render.PageCursor{At: at, ID: "01900000-0000-7000-8000-000000000001"})
	var asked Page
	response := httptest.NewRecorder()
	ListPage(response, httptest.NewRequest(http.MethodGet, "/?limit=2&cursor="+cursor, nil), func(page Page) ([]row, error) {
		asked = page
		return []row{{at, "a"}, {at, "b"}, {at, "c"}}, nil
	}, rowCursor, func(error) { t.Fatal("unexpected failure") })
	if asked.Limit != 2 || !asked.Before.Equal(at) || asked.CursorID != "01900000-0000-7000-8000-000000000001" {
		t.Fatalf("page = %+v", asked)
	}
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"next_cursor"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body)
	}
}

func TestListPageRejectsABadCursorBeforeFetching(t *testing.T) {
	response := httptest.NewRecorder()
	ListPage(response, httptest.NewRequest(http.MethodGet, "/?cursor=garbage", nil), func(Page) ([]row, error) {
		t.Fatal("fetched with a malformed cursor")
		return nil, nil
	}, rowCursor, func(error) { t.Fatal("unexpected failure") })
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestListPageHandsAFetchErrorToFail(t *testing.T) {
	var failed error
	response := httptest.NewRecorder()
	ListPage(response, httptest.NewRequest(http.MethodGet, "/", nil), func(Page) ([]row, error) {
		return nil, errors.New("storage down")
	}, rowCursor, func(err error) { failed = err })
	if failed == nil || response.Body.Len() != 0 {
		t.Fatalf("fail = %v, body = %q", failed, response.Body)
	}
}
