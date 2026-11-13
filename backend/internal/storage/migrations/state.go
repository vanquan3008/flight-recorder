// Package migrations provides pure status helpers. Each database adapter owns
// its migration engine, version sequence and journal independently.
package migrations

import (
	"fmt"

	"github.com/vanquan3008/tracepilot/db"
)

type Status struct {
	Version uint
	Target  uint
	Dirty   bool
	Pending []uint
}

// NewStatus derives the target and pending versions from the ordered embedded history.
func NewStatus(history []db.Migration, version uint, dirty bool) Status {
	state := Status{Version: version, Dirty: dirty}
	for _, migration := range history {
		state.Target = migration.Version
		if migration.Version > version {
			state.Pending = append(state.Pending, migration.Version)
		}
	}
	return state
}

func (state Status) String() string {
	return fmt.Sprintf("version: %d, target: %d, dirty: %t, pending: %v", state.Version, state.Target, state.Dirty, state.Pending)
}
