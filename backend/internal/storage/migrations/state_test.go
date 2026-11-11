package migrations

import (
	"reflect"
	"testing"

	"github.com/vanquan3008/tracepilot/db"
)

func TestStatusDerivesVersionsFromHistory(t *testing.T) {
	history := []db.Migration{{Version: 1}, {Version: 3}, {Version: 7}}
	for _, test := range []struct {
		name    string
		version uint
		dirty   bool
		pending []uint
	}{
		{"fresh", 0, false, []uint{1, 3, 7}},
		{"partially applied", 1, false, []uint{3, 7}},
		{"dirty", 3, true, []uint{7}},
		{"current", 7, false, nil},
		{"newer database", 8, false, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := NewStatus(history, test.version, test.dirty)
			if state.Target != 7 || state.Version != test.version || state.Dirty != test.dirty || !reflect.DeepEqual(state.Pending, test.pending) {
				t.Fatalf("unexpected status: %+v", state)
			}
		})
	}
}
