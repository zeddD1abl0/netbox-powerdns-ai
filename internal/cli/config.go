package cli

import (
	"context"

	"github.com/spf13/cobra"
)

func newConfigCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect nbpdns's configuration",
		Args:  usageArgs(cobra.NoArgs),
		RunE:  showHelp,
	}
	show := &cobra.Command{
		Use:   "show",
		Short: "Show each configuration key's value and where it came from",
		Long: "Show each configuration key's effective value, and the flag, environment\n" +
			"variable, config file, database, or default it came from. Secrets show as\n" +
			"[redacted]. The database, at database.url, counts if it exists: it stores the\n" +
			"runtime settings that nbpdns settings set, which nbpdns serve applies.\n\n" +
			"If any setting is invalid, nbpdns lists every problem and exits with status 1.",
		Args:        usageArgs(cobra.NoArgs),
		Annotations: map[string]string{annotationNoExport: "true"},
	}
	output := addOutputFlag(show)
	show.RunE = func(cmd *cobra.Command, _ []string) error {
		return a.run(cmd, func(ctx context.Context, s *session) error {
			s.applyStoredIfAny(ctx)
			if *output == outputJSON {
				return writeJSON(a.stdout, s.settings)
			}
			rows := make([][]string, len(s.settings))
			for i, st := range s.settings {
				rows[i] = []string{st.Key, st.Value, st.Source.String()}
			}
			return writeTable(a.stdout, []string{"KEY", "VALUE", "SOURCE"}, rows)
		})
	}
	cmd.AddCommand(show)
	return cmd
}
