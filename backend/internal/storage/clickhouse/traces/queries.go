package traces

import _ "embed"

//go:embed sql/trace_bounds.sql
var traceBoundsSQL string

//go:embed sql/trace_matches.sql
var traceMatchesSQL string

//go:embed sql/trace_spans.sql
var traceSpansSQL string

//go:embed sql/trace_summaries.sql
var traceSummariesSQL string

//go:embed sql/traces_by_duration.sql
var tracesByDurationSQL string

//go:embed sql/trace_summary_columns.sql
var traceSummaryColumns string
