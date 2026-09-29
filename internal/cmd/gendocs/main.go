// Command gendocs writes the reference pages that are generated from the
// code: the configuration keys and the command line. `make generate` runs
// it, and `make generate-check` fails when a committed page is stale.
//
//	gendocs [-out dir]
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/cli"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
)

// pages maps each generated page's file name to its writer.
var pages = map[string]func(io.Writer) error{
	"configuration.md": config.WriteReference,
	"command-line.md": func(w io.Writer) error {
		return cli.WriteReference(w, cli.New(io.Discard, io.Discard))
	},
}

func main() {
	out := flag.String("out", "docs/reference", "the directory to write the pages to")
	flag.Parse()
	if err := write(*out); err != nil {
		fmt.Fprintln(os.Stderr, "gendocs:", err)
		os.Exit(1)
	}
}

func write(dir string) error {
	for name, gen := range pages {
		var b bytes.Buffer
		if err := gen(&b); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		//nolint:gosec // The pages are ordinary repository files, readable by anyone.
		if err := os.WriteFile(filepath.Join(dir, name), b.Bytes(), 0o644); err != nil {
			return err
		}
	}
	return nil
}
