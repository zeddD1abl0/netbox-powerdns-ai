package cli

import (
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
		Args: usageArgs(cobra.NoArgs),
	}
	output := addOutputFlag(show)
	show.RunE = func(*cobra.Command, []string) error {
		_, settings, err := a.loader.Load()
		if err != nil {
			return err
		}
		if *output == outputJSON {
			return writeJSON(a.stdout, settings)
		}
		rows := make([][]string, len(settings))
		for i, s := range settings {
			rows[i] = []string{s.Key, s.Value, s.Source.String()}
		}
		return writeTable(a.stdout, []string{"KEY", "VALUE", "SOURCE"}, rows)
	}
	cmd.AddCommand(show)
	return cmd
}
