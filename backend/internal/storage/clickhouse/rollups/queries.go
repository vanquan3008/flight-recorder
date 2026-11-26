package rollups

import _ "embed"

// selectDirtyMinutesSQL lists closed minutes whose latest span mark is newer
// than their last rollup, oldest first. A minute never rolled up joins no done
// row and compares against the zero time.
//
//go:embed sql/select_dirty_minutes.sql
var selectDirtyMinutesSQL string

// insertSpanRollupsSQL rebuilds whole minutes from deduplicated spans, so
// retries and late spans never double-count. Callers append client.FinalSettings.
//
//go:embed sql/insert_span_rollups.sql
var insertSpanRollupsSQL string

// insertRollupCompletionSQL records when each minute's rollup was computed.
//
//go:embed sql/insert_rollup_completion.sql
var insertRollupCompletionSQL string

//go:embed sql/server_time.sql
var serverTimeSQL string
