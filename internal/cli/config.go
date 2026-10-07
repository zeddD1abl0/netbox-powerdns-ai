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
			"variable, config file, or default it came from. Secrets show as [redacted].\n\n" +
			"If any setting is invalid, nbpdns lists every problem and exits with status 1.",
		Args:        usageArgs(cobra.NoArgs),
		Annotations: map[string]string{annotationNoExport: "true"},
	}
	output := addOutputFlag(show)
	show.RunE = func(cmd *cobra.Command, _ []string) error {
		return a.run(cmd, func(_ context.Context, s *session) error {
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
