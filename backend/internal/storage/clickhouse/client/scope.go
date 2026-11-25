package client

import (
	"fmt"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

// FinalSettings is safe because a retried log lands in the partition of its
// first copy: the partition key derives from occurred_at, a deduplication key.
const FinalSettings = " SETTINGS do_not_merge_across_partitions_select_final = 1"

// Scope returns the WHERE terms and parameters that confine a read to one project and environment.
func Scope(projectID, environmentID string) ([]string, map[string]string, error) {
	if err := ValidateScope(projectID, environmentID); err != nil {
		return nil, nil, err
	}
	where := []string{"project_id = {project:UUID}", "environment_id = {environment:UUID}"}
	parameters := map[string]string{"project": projectID, "environment": environmentID}
	return where, parameters, nil
}

// ValidateScope refuses a scope that is not a project and environment UUID, so identifiers never
// reach a statement unchecked.
func ValidateScope(projectID, environmentID string) error {
	if !telemetry.ValidUUID(projectID) || !telemetry.ValidUUID(environmentID) {
		return fmt.Errorf("invalid ClickHouse scope: %w", telemetry.ErrInvalidEvent)
	}
	return nil
}
