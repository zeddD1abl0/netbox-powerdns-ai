package cli

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/version"
)

func newDBCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Show and migrate the database that nbpdns serve keeps its state in",
		Long: "Show and migrate the database at database.url, where nbpdns serve keeps its\n" +
			"state. nbpdns serve creates it, and applies its migrations, when it starts.\n" +
			"Migrations only move forward: take a copy of the database before an upgrade.",
		Args: usageArgs(cobra.NoArgs),
		RunE: showHelp,
	}
	cmd.AddCommand(newDBStatusCmd(a), newDBMigrateCmd(a))
	return cmd
}

func newDBStatusCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the database, and the migrations it has and lacks",
		Long: "Show the database at database.url: its file, or its URL, with any password\n" +
			"masked, its identity, and each migration that this nbpdns knows, with when it\n" +
			"was applied, or that it's pending.",
		Args: usageArgs(cobra.NoArgs),
	}
	output := addOutputFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return a.run(cmd, func(ctx context.Context, s *session) error {
			st, err := s.openStore(ctx, false)
			if err != nil {
				return err
			}
			defer st.Close()
			r, err := dbReport(ctx, st, s.cfg.Database.ShownURL())
			if err != nil {
				return err
			}
			if *output == outputJSON {
				return writeJSON(a.stdout, r)
			}
			return writeDBReport(a.stdout, r)
		})
	}
	return cmd
}

func newDBMigrateCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Create the database, or apply the migrations it lacks",
		Long: "Apply the migrations that the database at database.url lacks, each in its own\n" +
			"transaction, and create a SQLite database if it doesn't exist. A PostgreSQL\n" +
			"database must exist. It refuses while nbpdns serve runs on the database, since\n" +
			"serve applies them itself when it starts, and on a database that a newer\n" +
			"nbpdns has migrated.",
		Args: usageArgs(cobra.NoArgs),
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return a.run(cmd, func(ctx context.Context, s *session) error {
			st, lock, err := s.openLocked(ctx, s.createStore)
			if err != nil {
				return err
			}
			defer st.Close()
			defer lock.Close()
			applied, err := s.migrate(ctx, st)
			for _, m := range applied {
				fmt.Fprintf(a.stdout, "Applied migration %d, %s.\n", m.Version, m.Name)
			}
			if err != nil {
				return err
			}
			r, err := dbReport(ctx, st, s.cfg.Database.ShownURL())
			if err != nil {
				return err
			}
			fmt.Fprintf(a.stdout, "The database, %s, is up to date, at version %d.\n", r.where(), r.Version)
			return nil
		})
	}
	return cmd
}

// openStore opens the database at database.url, SQLite or PostgreSQL
// (ADR-0046). With create, it makes a SQLite database if it doesn't exist.
// On PostgreSQL, it warns of a release that nbpdns doesn't support, and of a
// connection that crosses the network unencrypted.
func (s *session) openStore(ctx context.Context, create bool) (*store.Store, error) {
	db := s.cfg.Database
	if path := db.SQLitePath(); path != "" {
		return store.Open(ctx, path, store.Options{Create: create})
	}
	st, err := store.OpenPostgres(ctx, db.URL.Reveal())
	if err != nil {
		return nil, err
	}
	if r := st.ServerRelease(); !store.SupportedRelease(r) {
		s.log.WarnContext(ctx, "the database's PostgreSQL release isn't one that nbpdns supports; nbpdns carries on, but isn't tested on it",
			"release", r, "supported", store.SupportedSeries())
	}
	if st.Cleartext() {
		s.log.WarnContext(ctx, "the connection to PostgreSQL isn't encrypted, so its password and data cross the network in the clear; "+
			"use sslmode=verify-full if the server offers TLS", "url", db.ShownURL())
	}
	return st, nil
}

// createStore opens the database at database.url, and makes a SQLite
// database if it doesn't exist.
func (s *session) createStore(ctx context.Context) (*store.Store, error) {
	return s.openStore(ctx, true)
}

// openLocked opens the database at database.url with open, such as
// createStore, under the lock that one nbpdns serve holds (ADR-0038), so
// that no serve runs on it meanwhile. On SQLite, the lock comes first, so
// that a second serve changes nothing. On PostgreSQL, it's taken on the open
// database, on a connection of its own (ADR-0046). Close the lock before the
// store.
func (s *session) openLocked(ctx context.Context, open func(context.Context) (*store.Store, error)) (*store.Store, *store.Lock, error) {
	if path := s.cfg.Database.SQLitePath(); path != "" {
		lock, err := store.Acquire(path)
		if err != nil {
			return nil, nil, err
		}
		st, err := open(ctx)
		if err != nil {
			_ = lock.Close()
			return nil, nil, err
		}
		return st, lock, nil
	}
	st, err := open(ctx)
	if err != nil {
		return nil, nil, err
	}
	lock, err := st.Lock(ctx)
	if err != nil {
		_ = st.Close()
		return nil, nil, err
	}
	return st, lock, nil
}

// migrate applies the database's pending migrations, logging each.
func (s *session) migrate(ctx context.Context, st *store.Store) ([]store.Migration, error) {
	applied, err := st.Migrate(ctx, version.Get().Version)
	for _, m := range applied {
		s.log.InfoContext(ctx, "applied a database migration",
			"version", m.Version, "name", m.Name, "duration_seconds", m.Duration.Seconds())
	}
	return applied, err
}

// dbStatus is what `nbpdns db status` shows.
type dbStatus struct {
	// Backend is sqlite or postgresql, and URL is database.url, with its
	// passwords masked.
	Backend string `json:"backend"`
	URL     string `json:"url"`
	// Path is SQLite's file, or empty on PostgreSQL.
	Path string `json:"path"`
	// On PostgreSQL, TLS is whether the connection uses TLS, and Release is
	// the server's.
	TLS     *bool  `json:"tls"`
	Release string `json:"release,omitempty"`
	// ID, Created and CreatedBy are the database's identity, which the
	// first migration makes.
	ID         string        `json:"id,omitempty"`
	Created    *time.Time    `json:"created,omitempty"`
	CreatedBy  string        `json:"created_by,omitempty"`
	Version    int64         `json:"version"`
	Latest     int64         `json:"latest"`
	Pending    bool          `json:"pending"`
	Migrations []dbMigration `json:"migrations"`
}

type dbMigration struct {
	Version int64      `json:"version"`
	Name    string     `json:"name"`
	Applied *time.Time `json:"applied"`
}

// where returns the database's file, or, on PostgreSQL, its URL.
func (r dbStatus) where() string {
	if r.Path != "" {
		return r.Path
	}
	return r.URL
}

// dbReport reads the status of st, the database at shownURL, which is
// database.url with its passwords masked.
func dbReport(ctx context.Context, st *store.Store, shownURL string) (dbStatus, error) {
	ss, err := st.Status(ctx)
	if err != nil {
		return dbStatus{}, err
	}
	r := dbStatus{Backend: st.Backend(), URL: shownURL, Path: st.Path(), Release: st.ServerRelease(),
		Version: ss.Version, Latest: ss.Latest, Pending: ss.Pending(), Migrations: make([]dbMigration, len(ss.Migrations))}
	if st.Backend() == store.PostgreSQL {
		encrypted := st.TLS()
		r.TLS = &encrypted
	}
	for i, m := range ss.Migrations {
		r.Migrations[i] = dbMigration{Version: m.Version, Name: m.Name}
		if !m.Applied.IsZero() {
			r.Migrations[i].Applied = &m.Applied
		}
	}
	if ss.Version > 0 {
		info, err := st.Info(ctx)
		if err != nil {
			return dbStatus{}, err
		}
		r.ID, r.Created, r.CreatedBy = info.ID, &info.Created, info.CreatedBy
	}
	return r, nil
}

// writeDBReport writes r as text.
func writeDBReport(w io.Writer, r dbStatus) error {
	if r.TLS != nil {
		conn := "unencrypted"
		if *r.TLS {
			conn = "over TLS"
		}
		fmt.Fprintf(w, "Database: PostgreSQL %s, %s, %s\n", r.Release, r.URL, conn)
	} else {
		fmt.Fprintf(w, "Database: %s\n", r.Path)
	}
	if r.ID != "" {
		fmt.Fprintf(w, "ID:       %s, created %s by nbpdns %s\n", r.ID, r.Created.Format(time.RFC3339), r.CreatedBy)
	}
	switch {
	case r.Version > r.Latest:
		fmt.Fprintf(w, "Schema:   version %d, newer than this nbpdns knows, %d\n", r.Version, r.Latest)
	case r.Pending:
		fmt.Fprintf(w, "Schema:   version %d of %d; run nbpdns db migrate, or start nbpdns serve\n", r.Version, r.Latest)
	default:
		fmt.Fprintf(w, "Schema:   version %d, up to date\n", r.Version)
	}
	fmt.Fprintln(w)
	rows := make([][]string, len(r.Migrations))
	for i, m := range r.Migrations {
		applied := "pending"
		if m.Applied != nil {
			applied = m.Applied.Format(time.RFC3339)
		}
		rows[i] = []string{strconv.FormatInt(m.Version, 10), m.Name, applied}
	}
	return writeTable(w, []string{"VERSION", "NAME", "APPLIED"}, rows)
}
