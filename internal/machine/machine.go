package machine

import (
	"context"
	"database/sql/driver"
	"embed"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.adoublef.dev/xiota"
)

//go:embed all:*.sql
var FS embed.FS

type DB struct {
	RWC *pgxpool.Pool
}

type Machine struct {
	ID    uuid.UUID
	Meta  Meta
	ModAt time.Time
}

func (d *DB) Machine(ctx context.Context, id uuid.UUID) (Machine, error) {
	var found Machine
	err := d.RWC.QueryRow(ctx, "select m.id, m.metadata, m.modified_at from machine.machine m where m.id = $1", id).Scan(&found.ID, &found.Meta, &found.ModAt)
	// wrap error
	return found, err
}

type Meta struct {
	State State `json:"state"`
}

func (m *Meta) Scan(src any) error {
	if src == nil {
		*m = Meta{}
		return nil
	}

	var data []byte
	switch v := src.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		return errors.New("unsupported type for Meta Scan")
	}
	return json.Unmarshal(data, m)
}

func (m Meta) Value() (driver.Value, error) {
	return json.Marshal(m)
}

func (d *DB) Add(ctx context.Context, meta Meta) (uuid.UUID, error) {
	id := uuid.Must(uuid.NewV7())
	_, err := d.RWC.Exec(ctx, "insert into machine.machine (id, metadata, modified_at) values ($1, $2, $3)", id, meta, time.Now().UTC())
	if err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

type State uint8

const (
	StateFailed State = iota
	StateCreated
	StateStarted
	StateStopped
	StateCreating
	StateStarting
	StateStopping
)

func (s State) String() string {
	return xiota.Format(s, states, StateFailed, StateStopping, 0)
}

func (s *State) UnmarshalText(p []byte) (err error) {
	*s, err = ParseState(string(p))
	return
}

func (s State) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

func ParseState(s string) (State, error) {
	return xiota.Parse[State](states, s, 0)
}

var states = []string{
	"failed",
	"created",
	"started",
	"stopped",
	"creating",
	"starting",
	"stopping",
}
