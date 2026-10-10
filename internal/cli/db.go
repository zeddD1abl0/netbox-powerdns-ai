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
		Long: "Show the database at database.url: its file, its identity, and each migration\n" +
			"that this nbpdns knows, with when it was applied, or that it's pending.",
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
			r, err := dbReport(ctx, st)
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
		Long: "Create the database at database.url if it doesn't exist, and apply the\n" +
			"migrations it lacks, each in its own transaction. It refuses while nbpdns\n" +
			"serve runs on the database, since serve applies them itself when it starts,\n" +
			"and on a database that a newer nbpdns has migrated.",
		Args: usageArgs(cobra.NoArgs),
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return a.run(cmd, func(ctx context.Context, s *session) error {
			lock, err := store.Acquire(s.cfg.Database.SQLitePath())
			if err != nil {
				return err
			}
			defer lock.Close()
			st, err := s.openStore(ctx, true)
			if err != nil {
				return err
			}
			defer st.Close()
			applied, err := s.migrate(ctx, st)
			for _, m := range applied {
				fmt.Fprintf(a.stdout, "Applied migration %d, %s.\n", m.Version, m.Name)
			}
			if err != nil {
				return err
			}
			r, err := dbReport(ctx, st)
			if err != nil {
				return err
			}
			fmt.Fprintf(a.stdout, "The database, %s, is up to date, at version %d.\n", r.Path, r.Version)
			return nil
		})
	}
	return cmd
}

// openStore opens the database at database.url. With create, it makes the
// database if it doesn't exist.
func (s *session) openStore(ctx context.Context, create bool) (*store.Store, error) {
	return store.Open(ctx, s.cfg.Database.SQLitePath(), store.Options{Create: create})
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
	Path string `json:"path"`
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

// dbReport reads the database's status.
func dbReport(ctx context.Context, st *store.Store) (dbStatus, error) {
	ss, err := st.Status(ctx)
	if err != nil {
		return dbStatus{}, err
	}
	r := dbStatus{Path: st.Path(), Version: ss.Version, Latest: ss.Latest, Pending: ss.Pending(),
		Migrations: make([]dbMigration, len(ss.Migrations))}
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
	fmt.Fprintf(w, "Database: %s\n", r.Path)
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
