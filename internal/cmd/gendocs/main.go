// Command gendocs writes the reference pages that are generated from the
// code: the configuration keys, the command line, the metrics, and the
// supported versions. `make generate` runs it, and `make generate-check` fails when a
// committed page is stale.
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
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/metrics"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/netbox"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/powerdns"
)

// pages maps each generated page's file name to its writer.
var pages = map[string]func(io.Writer) error{
	"configuration.md": config.WriteReference,
	"command-line.md": func(w io.Writer) error {
		return cli.WriteReference(w, cli.New(io.Discard, io.Discard))
	},
	"metrics.md": metrics.WriteReference,
	"supported-versions.md": func(w io.Writer) error {
		if err := netbox.WriteReference(w); err != nil {
			return err
		}
		return powerdns.WriteReference(w)
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
