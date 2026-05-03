package machine_test

import (
	"testing"

	. "github.com/adoublef/hey/internal/machine"
)

func TestDB(t *testing.T) {
	// t.Parallel()

	t.Run("OK", func(t *testing.T) {
		ctx := t.Context()
		var (
			pool = testDB(t)

			d = &DB{
				RWC: pool,
			}
		)

		migrations(t, pool)

		meta := Meta{
			State: StateCreating,
		}
		id, err := d.Add(ctx, meta)
		ok(t, err)

		mach, err := d.Machine(ctx, id)
		ok(t, err)

		equal(t, mach.Meta.State, StateCreating)
	})
}
