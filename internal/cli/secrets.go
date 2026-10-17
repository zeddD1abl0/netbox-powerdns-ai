package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/audit"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/secrets"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store"
)

func newSecretsCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secrets",
		Short: "Manage the master key that seals the secrets the database stores",
		Long: "Manage database.master_key, which seals the secrets that the database stores,\n" +
			"such as a netbox.token that nbpdns settings set stored (ADR-0041). Each secret\n" +
			"is sealed with its own data key, which the master key wraps.",
		Args: usageArgs(cobra.NoArgs),
		RunE: showHelp,
	}
	cmd.AddCommand(newSecretsRotateCmd(a))
	return cmd
}

func newSecretsRotateCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rotate-master-key",
		Short: "Re-wrap every stored secret with a new master key",
		Long: "Re-wrap every secret that the database stores with the new master key in the\n" +
			"file that --new-key-file names, opening them with database.master_key, all in\n" +
			"one transaction, which is an audit event. The secrets themselves don't change.\n" +
			"It refuses while nbpdns serve runs on the database: stop serve, rotate, then\n" +
			"start serve with database.master_key set to the new key. Keep the old key until\n" +
			"serve has started with the new one.",
		Args: usageArgs(cobra.NoArgs),
	}
	newKey := cmd.Flags().String("new-key-file", "", "The file that holds the new master key, in base64, such as openssl rand -base64 32 prints. Required.")
	_ = cmd.Flags().SetAnnotation("new-key-file", config.MarkdownUsage,
		[]string{"The file that holds the new master key, in base64, such as `openssl rand -base64 32` prints. Required."})
	reason := addReasonFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if *newKey == "" {
			return usageError{errors.New("--new-key-file is required")}
		}
		text, err := config.ReadSecretFile(*newKey)
		if err != nil {
			return fmt.Errorf("--new-key-file: %w", err)
		}
		next, err := secrets.ParseMasterKey(text)
		if err != nil {
			return fmt.Errorf("the new master key, in %s: %w", *newKey, err)
		}
		return a.run(cmd, func(ctx context.Context, s *session) error {
			old, err := s.masterKey()
			if err != nil {
				return err
			}
			switch {
			case !old.IsSet():
				return errors.New("database.master_key isn't set, so the stored secrets can't be opened to re-wrap them")
			case old.ID() == next.ID():
				return errors.New("the new master key is database.master_key")
			}
			// serve opens the secrets with the old key, so it mustn't run.
			lock, err := store.Acquire(s.cfg.Database.SQLitePath())
			if err != nil {
				return err
			}
			defer lock.Close()
			st, err := s.currentStore(ctx)
			if err != nil {
				return err
			}
			defer st.Close()
			n, err := st.RotateMasterKey(ctx, old.ID(), next.ID(), func(place, sealed string) (string, error) {
				return secrets.Rewrap(old, next, place, sealed)
			}, audit.OSUser(), *reason)
			if err != nil {
				return err
			}
			what := "secrets"
			if n == 1 {
				what = "secret"
			}
			fmt.Fprintf(a.stdout, "Re-wrapped %d %s with %s.\n", n, what, next)
			fmt.Fprintln(a.stdout, "Set database.master_key to the new key before nbpdns serve starts again.")
			return nil
		})
	}
	return cmd
}
