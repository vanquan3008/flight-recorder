package detection

import _ "embed"

//go:embed sql/metric_evidence.sql
var metricEvidenceSQL string

//go:embed sql/window_signals.sql
var windowSignalsSQL string

//go:embed sql/window_signal_columns.sql
var windowSignalColumns string
