package cli

import (
	"strconv"

	"github.com/spf13/cobra"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/version"
)

func newVersionCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version, commit, and Go version that nbpdns was built with",
		Args:  usageArgs(cobra.NoArgs),
	}
	output := addOutputFlag(cmd)
	cmd.RunE = func(*cobra.Command, []string) error {
		info := version.Get()
		if *output == outputJSON {
			return writeJSON(a.stdout, info)
		}
		return writeTable(a.stdout, []string{"FIELD", "VALUE"}, [][]string{
			{"version", info.Version},
			{"commit", info.Commit},
			{"commit_time", info.CommitTime},
			{"modified", strconv.FormatBool(info.Modified)},
			{"go_version", info.GoVersion},
			{"platform", info.Platform},
		})
	}
	return cmd
}
