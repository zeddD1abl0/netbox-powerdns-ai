package lab

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net"
	"net/url"
	"strconv"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // the database/sql driver "pgx"
)

// The lab's PostgreSQL 18 server, postgres-47: NetBox 4.7's, which also
// holds nbpdns's own role and database, which `make lab-up` creates if
// they're missing (ADR-0046). It has no TLS.
const (
	// PostgresVersion is the PostgreSQL release it runs.
	PostgresVersion = "18"
	// PostgresPort is the port it's published on, on the lab's host.
	PostgresPort = 5418
	// PostgresAdmin is its superuser, with PostgresAdminPassword. Tests use
	// it to create and drop their databases, never to read them.
	PostgresAdmin         = "netbox"
	PostgresAdminPassword = "nbpdns-lab-db" //nolint:gosec // The lab's published credential. gitleaks:allow
	// PostgresUser is nbpdns's role, with PostgresPassword, which owns the
	// database PostgresDatabase and the tests' databases.
	PostgresUser     = "nbpdns"
	PostgresPassword = "nbpdns-lab-nbpdns-db" //nolint:gosec // The lab's published credential. gitleaks:allow
	PostgresDatabase = "nbpdns"
)

// PostgresURL returns the URL of the lab's database, as user with password,
// without TLS.
func PostgresURL(user, password, database string) string {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(user, password),
		Host:     net.JoinHostPort(Host(), strconv.Itoa(PostgresPort)),
		Path:     "/" + database,
		RawQuery: "sslmode=disable",
	}
	return u.String()
}

// NewPostgresDatabase creates an empty database in the lab's PostgreSQL,
// owned by nbpdns's role, drops it when the test ends, and returns its URL,
// as that role. Each test gets its own, so tests can run at once. A test
// binary that dies before its cleanup leaves its database behind, which
// `make lab-down` removes.
func NewPostgresDatabase(t testing.TB) string {
	t.Helper()
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	name := "nbpdns_test_" + hex.EncodeToString(b)
	// The test's context ends before its cleanups run.
	admin := func(stmt string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		db, err := sql.Open("pgx", PostgresURL(PostgresAdmin, PostgresAdminPassword, "postgres"))
		if err != nil {
			return err
		}
		defer db.Close()
		_, err = db.ExecContext(ctx, stmt)
		return err
	}
	// name is ours, and needs no quoting.
	if err := admin("CREATE DATABASE " + name + " OWNER " + PostgresUser); err != nil {
		t.Fatalf("creating a database in the lab's PostgreSQL on %s (is the lab up? make lab-up): %v",
			net.JoinHostPort(Host(), strconv.Itoa(PostgresPort)), err)
	}
	t.Cleanup(func() {
		// FORCE ends the sessions that a failed test left open.
		if err := admin("DROP DATABASE " + name + " WITH (FORCE)"); err != nil {
			t.Errorf("dropping the test's database %s: %v", name, err)
		}
	})
	return PostgresURL(PostgresUser, PostgresPassword, name)
}
