package rollups

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/client"
)

const (
	// rollupLookback bounds the search for minutes that still need a rollup.
	rollupLookback = 8 * 24 * time.Hour
	// rollupSafety marks a refresh as computed slightly before it started, so a
	// span whose mark was taken while its insert was still committing makes its
	// minute dirty again instead of being missed.
	rollupSafety = 5 * time.Second
	// Batch caps the minutes one refresh recomputes. A refresh that
	// returns a full batch may have left more dirty minutes behind.
	Batch = 1000
)

type dirtyMinute struct {
	Project     string    `json:"project"`
	Environment string    `json:"environment"`
	Minute      time.Time `json:"minute"`
}

type serverClock struct {
	Now time.Time `json:"server_now"`
}

type rollupScope struct {
	ProjectID     string
	EnvironmentID string
}

type rollupGroup struct {
	Scope   rollupScope
	Minutes []time.Time
}

// RefreshSpanRollups recomputes closed minutes with new or updated spans.
func (store *Rollups) RefreshSpanRollups(
	ctx context.Context,
) (int, error) {
	now, err := store.serverTime(ctx)
	if err != nil {
		return 0, err
	}

	dirty, err := store.findDirtyMinutes(ctx, now)
	if err != nil {
		return 0, err
	}

	if len(dirty) == 0 {
		return 0, nil
	}

	computedAt := now.Add(-rollupSafety)
	groups := groupDirtyMinutes(dirty)

	processed := 0

	for _, group := range groups {
		if err := ctx.Err(); err != nil {
			return processed, err
		}

		if err := store.rollupMinutes(
			ctx,
			group.Scope,
			group.Minutes,
			computedAt,
		); err != nil {
			return processed, fmt.Errorf(
				"refresh rollup for project %s environment %s: %w",
				group.Scope.ProjectID,
				group.Scope.EnvironmentID,
				err,
			)
		}

		processed += len(group.Minutes)
	}

	return processed, nil
}

func (store *Rollups) serverTime(
	ctx context.Context,
) (time.Time, error) {
	const query = `
		SELECT now64(3, 'UTC') AS server_now
		FORMAT JSONEachRow
	`

	rows, err := client.QueryRows[serverClock](
		ctx,
		store.client,
		query,
		nil,
	)
	if err != nil {
		return time.Time{}, fmt.Errorf(
			"query ClickHouse server time: %w",
			err,
		)
	}

	if len(rows) == 0 || rows[0].Now.IsZero() {
		return time.Time{}, errors.New(
			"ClickHouse returned an invalid server time",
		)
	}

	return rows[0].Now.UTC(), nil
}

func (store *Rollups) findDirtyMinutes(
	ctx context.Context,
	now time.Time,
) ([]dirtyMinute, error) {
	parameters := map[string]string{
		"now":      client.Time(now),
		"lookback": client.Time(now.Add(-rollupLookback)),
		"limit":    strconv.Itoa(Batch),
	}

	rows, err := client.QueryRows[dirtyMinute](
		ctx,
		store.client,
		selectDirtyMinutesSQL,
		parameters,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"query dirty rollup minutes: %w",
			err,
		)
	}

	return rows, nil
}

func groupDirtyMinutes(
	dirty []dirtyMinute,
) []rollupGroup {
	groups := make([]rollupGroup, 0)
	indexes := make(map[rollupScope]int)

	for _, minute := range dirty {
		scope := rollupScope{
			ProjectID:     minute.Project,
			EnvironmentID: minute.Environment,
		}

		index, exists := indexes[scope]
		if !exists {
			index = len(groups)
			indexes[scope] = index

			groups = append(groups, rollupGroup{
				Scope: scope,
			})
		}

		groups[index].Minutes = append(
			groups[index].Minutes,
			minute.Minute,
		)
	}

	return groups
}

func (store *Rollups) rollupMinutes(
	ctx context.Context,
	scope rollupScope,
	minutes []time.Time,
	computedAt time.Time,
) error {
	if len(minutes) == 0 {
		return nil
	}

	if err := client.ValidateScope(
		scope.ProjectID,
		scope.EnvironmentID,
	); err != nil {
		return err
	}

	parameters := rollupParameters(
		scope,
		minutes,
		computedAt,
	)

	if _, err := store.client.Execute(
		ctx,
		insertSpanRollupsSQL+client.FinalSettings,
		parameters,
		nil,
		false,
	); err != nil {
		return fmt.Errorf("insert span rollups: %w", err)
	}

	if _, err := store.client.Execute(
		ctx,
		insertRollupCompletionSQL,
		parameters,
		nil,
		false,
	); err != nil {
		return fmt.Errorf("mark rollups completed: %w", err)
	}

	return nil
}

func rollupParameters(
	scope rollupScope,
	minutes []time.Time,
	computedAt time.Time,
) map[string]string {
	sorted := append([]time.Time(nil), minutes...)

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Before(sorted[j])
	})

	values := make([]string, len(sorted))

	for i, minute := range sorted {
		values[i] = "'" +
			minute.UTC().Format(time.DateTime) +
			"'"
	}

	first := sorted[0]
	last := sorted[len(sorted)-1].Add(time.Minute)

	return map[string]string{
		"project":     scope.ProjectID,
		"environment": scope.EnvironmentID,
		"computed":    client.Time(computedAt),
		"first":       client.Time(first),
		"last":        client.Time(last),
		"minutes":     "[" + strings.Join(values, ",") + "]",
	}
}
