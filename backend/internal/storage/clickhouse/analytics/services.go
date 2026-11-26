package analytics

import (
	"context"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/query"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/client"
)

// parentLookback is how far before a window a parent span may have started and
// still be matched to a child inside it.
const parentLookback = time.Hour

// dependencyWindow bounds the service dependency list, which takes no window.
const dependencyWindow = 24 * time.Hour

func parentStart(from time.Time) time.Time {
	if from.IsZero() {
		return from
	}
	return from.Add(-parentLookback)
}

type serviceRow struct {
	Name           string  `json:"name"`
	TraceCount     int64   `json:"trace_count"`
	RequestCount   int64   `json:"request_count"`
	ErrorCount     int64   `json:"error_count"`
	ErrorRate      float64 `json:"error_rate"`
	AverageLatency int64   `json:"average_latency"`
}

func (store *Analytics) ListServices(ctx context.Context, projectID, environmentID string, from, to time.Time) ([]query.ServiceSummary, error) {
	where, parameters, err := client.Scope(projectID, environmentID)
	if err != nil {
		return nil, err
	}
	where = client.AddTime(where, parameters, "started_at", "from", ">=", from)
	where = client.AddTime(where, parameters, "started_at", "to", "<=", to)
	rows, err := client.QueryRows[serviceRow](ctx, store.client, `SELECT
    service_name AS name,
    uniqExact(trace_id) AS trace_count,
    countIf(span_kind = 2) AS request_count,
    countIf(status_code = 'ERROR') AS error_count,
    if(count() = 0, 0, error_count / count()) AS error_rate,
    toInt64(avg(duration_ns)) AS average_latency
FROM tracepilot_spans FINAL
WHERE `+strings.Join(where, " AND ")+`
GROUP BY service_name
ORDER BY service_name`+client.FinalSettings+`
FORMAT JSONEachRow`, parameters)
	if err != nil {
		return nil, err
	}
	services := make([]query.ServiceSummary, 0, len(rows))
	for _, row := range rows {
		services = append(services, query.ServiceSummary(row))
	}
	return services, nil
}

// ListServiceDependencies counts calls from a service to other services over the last day.
func (store *Analytics) ListServiceDependencies(ctx context.Context, projectID, environmentID, serviceName string, since time.Time) ([]query.ServiceDependency, error) {
	_, parameters, err := client.Scope(projectID, environmentID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	from := now.Add(-dependencyWindow)
	parameters["service"], parameters["to"] = serviceName, client.Time(now)
	parameters["from"] = client.Time(client.MaxTime(from, since))
	parameters["parent_from"] = client.Time(client.MaxTime(parentStart(from), since))
	rows, err := client.QueryRows[query.ServiceDependency](ctx, store.client, `SELECT
    child.service_name AS service_name,
    count() AS call_count
FROM (
    SELECT trace_id, parent_span_id, service_name
    FROM tracepilot_spans FINAL
    WHERE project_id = {project:UUID}
      AND environment_id = {environment:UUID}
      AND started_at >= {from:DateTime64(9, 'UTC')}
      AND started_at <= {to:DateTime64(9, 'UTC')}
      AND parent_span_id != ''
      AND service_name != {service:String}
) AS child
INNER JOIN (
    SELECT trace_id, span_id
    FROM tracepilot_spans FINAL
    WHERE project_id = {project:UUID}
      AND environment_id = {environment:UUID}
      AND started_at >= {parent_from:DateTime64(9, 'UTC')}
      AND started_at <= {to:DateTime64(9, 'UTC')}
      AND service_name = {service:String}
) AS parent
    ON child.trace_id = parent.trace_id
   AND child.parent_span_id = parent.span_id
GROUP BY service_name
ORDER BY call_count DESC, service_name`+client.FinalSettings+`
FORMAT JSONEachRow`, parameters)
	return rows, err
}
