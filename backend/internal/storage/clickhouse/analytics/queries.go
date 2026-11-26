package analytics

import _ "embed"

//go:embed sql/list_metric_points.sql
var listMetricPointsSQL string

//go:embed sql/list_service_dependencies.sql
var listServiceDependenciesSQL string

//go:embed sql/list_services.sql
var listServicesSQL string

//go:embed sql/list_topology.sql
var listTopologySQL string

//go:embed sql/signal_quality.sql
var signalQualitySQL string

//go:embed sql/performance_spans.sql
var performanceSpansSQL string

//go:embed sql/performance_rollups.sql
var performanceRollupsSQL string

//go:embed sql/request_performance.sql
var requestPerformanceSQL string

//go:embed sql/clean_minutes.sql
var cleanMinutesSQL string
