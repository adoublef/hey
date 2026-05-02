package machine_test

import (
	"context"
	"io/fs"
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

			m = &migrator{pool: pool, fsys: []fs.FS{FS}}
		)

		err := m.up(context.Background())
		ok(t, err)
		t.Cleanup(func() { m.down(context.Background()) })

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
