package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"iter"
	"strconv"
	"strings"
	"time"
)

// QueryRows runs a JSONEachRow query and decodes every row.
func QueryRows[T any](ctx context.Context, client *Client, sql string, parameters map[string]string) ([]T, error) {
	data, err := client.Execute(ctx, sql, parameters, nil, false)
	if err != nil {
		return nil, err
	}
	rows := []T{}
	for row, err := range DecodeJSONRows[T](bytes.NewReader(data)) {
		if err != nil {
			return nil, errors.New("decode ClickHouse rows")
		}
		rows = append(rows, *row)
	}
	return rows, nil
}

// DecodeJSONRows streams JSONEachRow rows and reports a truncated response as an error.
func DecodeJSONRows[T any](source io.Reader) iter.Seq2[*T, error] {
	return func(yield func(*T, error) bool) {
		decoder := json.NewDecoder(source)
		for {
			var row T
			err := decoder.Decode(&row)
			if errors.Is(err, io.EOF) {
				return
			}
			if !yield(&row, err) || err != nil {
				return
			}
		}
	}
}

// Time formats a time as a ClickHouse DateTime64(9) parameter.
func Time(at time.Time) string { return at.UTC().Format("2006-01-02 15:04:05.999999999") }

// AddTime appends a typed time comparison unless at is zero.
func AddTime(where []string, parameters map[string]string, column, name, comparison string, at time.Time) []string {
	if at.IsZero() {
		return where
	}
	parameters[name] = Time(at)
	return append(where, column+" "+comparison+" {"+name+":DateTime64(9, 'UTC')}")
}

// AddEquals appends a typed equality filter for each non-empty value.
func AddEquals(where []string, parameters map[string]string, filters ...[3]string) []string {
	for _, filter := range filters {
		column, name, value := filter[0], filter[1], filter[2]
		if value != "" {
			where = append(where, column+" = {"+name+":String}")
			parameters[name] = value
		}
	}
	return where
}

// InStrings binds values as one parameter each, for an IN list the skip indexes can use.
func InStrings(column, prefix string, values []string, parameters map[string]string) string {
	names := make([]string, len(values))
	for index, value := range values {
		name := prefix + strconv.Itoa(index)
		parameters[name] = value
		names[index] = "{" + name + ":String}"
	}
	return column + " IN (" + strings.Join(names, ", ") + ")"
}

// MaxTime is the later of two times.
func MaxTime(first, second time.Time) time.Time {
	if second.After(first) {
		return second
	}
	return first
}
