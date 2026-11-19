package clickhouse

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
)

// queryRows runs a JSONEachRow query and decodes every row.
func queryRows[T any](ctx context.Context, store *Store, sql string, parameters map[string]string) ([]T, error) {
	data, err := store.execute(ctx, sql, parameters, nil, false)
	if err != nil {
		return nil, err
	}
	decoder, rows := json.NewDecoder(bytes.NewReader(data)), []T{}
	for {
		var row T
		if err := decoder.Decode(&row); err != nil {
			if errors.Is(err, io.EOF) {
				return rows, nil
			}
			return nil, errors.New("decode ClickHouse rows")
		}
		rows = append(rows, row)
	}
}

func chTime(at time.Time) string { return at.UTC().Format("2006-01-02 15:04:05.999999999") }

// addEquals appends a typed equality filter for each non-empty value.
func addEquals(where []string, parameters map[string]string, filters ...[3]string) []string {
	for _, filter := range filters {
		column, name, value := filter[0], filter[1], filter[2]
		if value != "" {
			where = append(where, column+" = {"+name+":String}")
			parameters[name] = value
		}
	}
	return where
}

// inStrings binds values as one parameter each, for an IN list the skip indexes can use.
func inStrings(column, prefix string, values []string, parameters map[string]string) string {
	names := make([]string, len(values))
	for index, value := range values {
		name := prefix + strconv.Itoa(index)
		parameters[name] = value
		names[index] = "{" + name + ":String}"
	}
	return column + " IN (" + strings.Join(names, ", ") + ")"
}
