// Command projctl scaffolds, indexes and lints the project's tracking files:
// project/ (board, requirements, milestones, items) and docs/adr/. The model
// it enforces is described in ADR-0002.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

const usage = `usage: projctl [-root dir] <command>

commands:
  new item [-milestone Mnn] [-type task] <title>   create the next work item
  new adr <title>                                  create the next ADR
  index                                            regenerate the board and ADR index
  lint                                             check every tracking file
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, time.Now))
}

func run(args []string, stdout, stderr io.Writer, now func() time.Time) int {
	fs := flag.NewFlagSet("projctl", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usage) }
	root := fs.String("root", ".", "repository root")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return 2
	}

	r, loadProbs, err := Load(*root)
	if err != nil {
		fmt.Fprintln(stderr, "projctl:", err)
		return 1
	}

	switch cmd, rest := fs.Arg(0), fs.Args()[1:]; cmd {
	case "lint":
		probs, err := Lint(r)
		if err != nil {
			fmt.Fprintln(stderr, "projctl:", err)
			return 1
		}
		probs = append(loadProbs, probs...)
		for _, p := range probs {
			fmt.Fprintln(stdout, p)
		}
		if len(probs) > 0 {
			fmt.Fprintf(stderr, "projctl: %d problem(s)\n", len(probs))
			return 1
		}
		return 0

	case "index":
		if len(loadProbs) > 0 {
			for _, p := range loadProbs {
				fmt.Fprintln(stderr, p)
			}
			return 1
		}
		if err := writeGenerated(r); err != nil {
			fmt.Fprintln(stderr, "projctl:", err)
			return 1
		}
		return 0

	case "new":
		return runNew(r, rest, stdout, stderr, now().Format(time.DateOnly))

	default:
		fs.Usage()
		return 2
	}
}

func runNew(r *Repo, args []string, stdout, stderr io.Writer, today string) int {
	if len(args) == 0 || (args[0] != "item" && args[0] != "adr") {
		fmt.Fprint(stderr, usage)
		return 2
	}
	kind := args[0]
	fs := flag.NewFlagSet("projctl new "+kind, flag.ContinueOnError)
	fs.SetOutput(stderr)
	milestone := fs.String("milestone", "", "milestone (default: the one in progress)")
	typ := fs.String("type", "task", "feature, bug, debt or task")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if fs.NArg() != 1 || fs.Arg(0) == "" {
		fmt.Fprintf(stderr, "projctl new %s: give the title as one quoted argument\n", kind)
		return 2
	}
	title := fs.Arg(0)

	var path string
	var err error
	if kind == "item" {
		path, err = NewItem(r, title, *milestone, *typ, today)
	} else {
		path, err = NewADR(r, title, today)
	}
	if err != nil {
		fmt.Fprintln(stderr, "projctl:", err)
		return 1
	}
	// Keep the board and ADR index current.
	r2, _, err := Load(r.Root)
	if err == nil {
		err = writeGenerated(r2)
	}
	if err != nil {
		fmt.Fprintln(stderr, "projctl:", err)
		return 1
	}
	fmt.Fprintln(stdout, path)
	return 0
}
