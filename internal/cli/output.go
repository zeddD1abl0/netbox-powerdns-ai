package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
)

// outputFormat is the --output flag of commands that print data.
type outputFormat string

const (
	outputTable outputFormat = "table"
	outputJSON  outputFormat = "json"
)

func (o *outputFormat) String() string { return string(*o) }
func (o *outputFormat) Type() string   { return "format" }

func (o *outputFormat) Set(s string) error {
	switch f := outputFormat(s); f {
	case outputTable, outputJSON:
		*o = f
		return nil
	}
	return fmt.Errorf("%q isn't table or json", s)
}

// addOutputFlag adds --output to cmd, defaulting to a table.
func addOutputFlag(cmd *cobra.Command) *outputFormat {
	o := outputTable
	cmd.Flags().VarP(&o, "output", "o", "The output format: table or json.")
	_ = cmd.Flags().SetAnnotation("output", config.MarkdownUsage, []string{"The output format: `table` or `json`."})
	return &o
}

// writeJSON writes v as indented JSON.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// writeTable writes rows under a header, in aligned columns.
func writeTable(w io.Writer, header []string, rows [][]string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(header, "\t"))
	for _, r := range rows {
		fmt.Fprintln(tw, strings.Join(r, "\t"))
	}
	return tw.Flush()
}
