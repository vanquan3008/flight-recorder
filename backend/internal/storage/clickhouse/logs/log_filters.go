package logs

import (
	"strconv"
	"strings"

	"github.com/vanquan3008/tracepilot/internal/query"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/client"
)

func (store *Logs) logFilter(projectID string, filter query.LogFilter) ([]string, map[string]string, error) {
	where, parameters, err := client.Scope(projectID, filter.EnvironmentID)
	if err != nil {
		return nil, nil, err
	}
	where = client.AddEquals(where, parameters, [3]string{"service_name", "service", filter.Service},
		[3]string{"trace_id", "trace", filter.TraceID}, [3]string{"span_id", "span", filter.SpanID})
	if filter.Severity != "" {
		where = append(where, logSeverityPredicate(filter.Severity, parameters))
	}
	if filter.Search != "" {
		where = append(where, "lowerUTF8(body) LIKE {search:String}")
		parameters["search"] = logSearchPattern(filter.Search)
	}
	where = client.AddTime(where, parameters, "occurred_at", "from", ">=", filter.From)
	where = client.AddTime(where, parameters, "occurred_at", "to", "<=", filter.To)
	if !filter.Before.IsZero() {
		where = append(where, "(occurred_at, toString(event_id)) < ({before:DateTime64(9, 'UTC')}, {cursor:String})")
		parameters["before"], parameters["cursor"] = client.Time(filter.Before), filter.CursorID
	}
	return where, parameters, nil
}

// logSearchPattern escapes LIKE wildcards and lower-cases the term so the
// body_search_lower index applies.
func logSearchPattern(term string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.ToLower(term)) + "%"
}

// logSeverityPredicate matches a standard level by severity number, or by text
// when the number is unspecified.
func logSeverityPredicate(value string, parameters map[string]string) string {
	level := strings.ToUpper(strings.TrimSpace(value))
	switch level {
	case "INFORMATION":
		level = "INFO"
	case "WARNING":
		level = "WARN"
	}
	for index, group := range []string{"TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL"} {
		if level != group && !(strings.HasPrefix(level, group) && len(level) == len(group)+1 && level[len(group)] >= '1' && level[len(group)] <= '4') {
			continue
		}
		parameters["severity_min"], parameters["severity_max"] = strconv.Itoa(index*4+1), strconv.Itoa(index*4+4)
		pattern := group + "[1-4]?"
		if group == "INFO" {
			pattern += "|INFORMATION|"
		} else if group == "WARN" {
			pattern += "|WARNING"
		}
		parameters["severity_pattern"] = "^(" + pattern + ")$"
		return "(severity_number BETWEEN {severity_min:Int32} AND {severity_max:Int32} OR " +
			"(NOT (severity_number BETWEEN 1 AND 24) AND match(upperUTF8(trimBoth(severity_text)), {severity_pattern:String})))"
	}
	parameters["severity"] = level
	return "upperUTF8(trimBoth(severity_text)) = {severity:String}"
}
