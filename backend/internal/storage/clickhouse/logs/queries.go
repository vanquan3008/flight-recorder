package logs

import _ "embed"

//go:embed sql/get_log.sql
var getLogSQL string

//go:embed sql/list_log_keys.sql
var listLogKeysSQL string

//go:embed sql/list_logs.sql
var listLogsSQL string

//go:embed sql/log_quality.sql
var logQualitySQL string
