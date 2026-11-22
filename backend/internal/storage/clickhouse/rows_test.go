package clickhouse

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestDecodeJSONRowsReportsTruncationAfterCompleteRows(t *testing.T) {
	type row struct {
		ID int `json:"id"`
	}
	var rows []*row
	var readErrors int
	source := strings.NewReader("{\"id\":1}\n{\"id\":2}\n{\"id\":")
	for entry, err := range decodeJSONRows[row](source) {
		if err != nil {
			if !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("decode error = %v, want unexpected EOF", err)
			}
			readErrors++
			continue
		}
		rows = append(rows, entry)
	}
	if len(rows) != 2 || readErrors != 1 {
		t.Fatalf("decoded rows = %d, errors = %d, want 2 and 1", len(rows), readErrors)
	}
	if rows[0].ID != 1 || rows[1].ID != 2 {
		t.Fatalf("decoded IDs = %d, %d, want 1 and 2", rows[0].ID, rows[1].ID)
	}
}
