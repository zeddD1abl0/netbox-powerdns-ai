// Package cli defines nbpdns's command line: the Cobra command tree, its
// output formats and its exit codes.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
)

// Exit codes. Scripts can rely on them.
const (
	exitOK    = 0 // the command succeeded
	exitError = 1 // the command failed; stderr says why
	exitUsage = 2 // the command line was wrong: an unknown command or flag, or a bad argument
	exitDrift = 3 // nbpdns drift compared everything, and found drift
)

// Main runs nbpdns with args, not including the program name, and returns its
// exit code.
func Main(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	root := New(stdout, stderr) //nolint:contextcheck // ctx reaches the commands through ExecuteContext.
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return exitOK
	}
	fmt.Fprintf(stderr, "nbpdns: %v\n", err)
	code := exitCode(err)
	if code == exitUsage {
		fmt.Fprintln(stderr, "Run 'nbpdns --help' for usage.")
	}
	return code
}

// exitCode returns the exit code for a command's error.
func exitCode(err error) int {
	var (
		ue usageError
		de driftError
	)
	switch {
	case err == nil:
		return exitOK
	case errors.As(err, &ue):
		return exitUsage
	case errors.As(err, &de):
		return exitDrift
	}
	return exitError
}

// app holds what every command shares.
type app struct {
	stdout, stderr io.Writer
	loader         *config.Loader
}

// New returns nbpdns's root command, writing to stdout and stderr.
func New(stdout, stderr io.Writer) *cobra.Command {
	a := &app{stdout: stdout, stderr: stderr, loader: config.NewLoader()}
	root := &cobra.Command{
		Use:   "nbpdns",
		Short: "Read DNS data from NetBox, and keep PowerDNS in step with it",
		Long: "nbpdns reads DNS data from NetBox's DNS plugin, the source of truth, and\n" +
			"keeps PowerDNS Authoritative servers in step with it.\n\n" +
			"Settings come from flags, NBPDNS_ environment variables and a YAML config\n" +
			"file, in that order of precedence. Logs go to standard error, and command\n" +
			"output to standard output.",
		Args:          usageArgs(cobra.NoArgs),
		RunE:          showHelp,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError{err} })
	a.loader.AddFlags(root.PersistentFlags())
	root.AddCommand(newVersionCmd(a), newConfigCmd(a), newNetBoxCmd(a), newPowerDNSCmd(a), newDriftCmd(a), newServeCmd(a))
	// Cobra adds these when the command runs. Add them now, so the generated
	// reference sees the whole tree.
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()
	rewordCompletion(root)
	return root
}

// rewordCompletion replaces the help of Cobra's completion commands with
// wording that follows the documentation style. Each shell's own long help,
// which explains how to load its script, is Cobra's.
func rewordCompletion(root *cobra.Command) {
	c, _, err := root.Find([]string{"completion"})
	if err != nil || c == root {
		return
	}
	c.Args, c.RunE = usageArgs(cobra.NoArgs), showHelp
	c.Short = "Print a script that completes nbpdns's commands and flags in a shell"
	c.Long = "Print a script that completes nbpdns's commands and flags in bash, zsh,\n" +
		"fish, or PowerShell. Each shell's subcommand explains how to load its script."
	shells := map[string]string{"bash": "bash", "zsh": "zsh", "fish": "fish", "powershell": "PowerShell"}
	for _, s := range c.Commands() {
		if name, ok := shells[s.Name()]; ok {
			s.Short = "Print the completion script for " + name
		}
		if f := s.Flags().Lookup("no-descriptions"); f != nil {
			f.Usage = "Leave out the descriptions of commands and flags."
		}
	}
}

// usageError marks an error in how nbpdns was invoked.
type usageError struct{ err error }

func (e usageError) Error() string { return e.err.Error() }
func (e usageError) Unwrap() error { return e.err }

// showHelp is the RunE of a command that only groups subcommands. Cobra
// validates a command's arguments only if it can run, so this makes a stray
// argument a usage error instead of a silent help page.
func showHelp(cmd *cobra.Command, _ []string) error { return cmd.Help() }

// usageArgs makes an argument validator's errors usage errors.
func usageArgs(validate cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := validate(cmd, args); err != nil {
			return usageError{err}
		}
		return nil
	}
}
