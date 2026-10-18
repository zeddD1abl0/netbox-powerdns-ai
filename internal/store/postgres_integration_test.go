//go:build integration

package store

import (
	"testing"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/lab"
)

// The integration tests run each of the store's tests on the lab's
// PostgreSQL too, in a database of its own (ADR-0046).
func init() { backends = append(backends, postgresBackend) }

var postgresBackend = backend{
	name:   "postgres",
	create: func(t *testing.T) string { return lab.NewPostgresDatabase(t) },
	connect: func(t *testing.T, where string) *Store {
		t.Helper()
		s, err := OpenPostgres(t.Context(), where)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = s.Close() })
		return s
	},
}
