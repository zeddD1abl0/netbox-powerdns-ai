// Command releasenotes prints the CHANGELOG's section for a release's tag,
// for `make release` to publish as the release's notes (ADR-0030). It fails
// if the tag isn't vMAJOR.MINOR.PATCH, or if the CHANGELOG has no section
// for it, or an empty one.
//
//	releasenotes -tag v0.1.0 CHANGELOG.md
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// tagRE matches a release's tag, and captures its version.
var tagRE = regexp.MustCompile(`^v([0-9]+\.[0-9]+\.[0-9]+)$`)

func main() {
	tag := flag.String("tag", "", "the release's tag, such as v0.1.0")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: releasenotes -tag vMAJOR.MINOR.PATCH CHANGELOG.md")
		os.Exit(2)
	}
	changelog, err := os.ReadFile(flag.Arg(0))
	if err == nil {
		var out string
		if out, err = notes(string(changelog), *tag); err == nil {
			_, err = fmt.Print(out)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "releasenotes:", err)
		os.Exit(1)
	}
}

// notes returns the section of changelog for tag, without its heading: the
// lines after `## [VERSION]`, up to the next release's heading.
func notes(changelog, tag string) (string, error) {
	m := tagRE.FindStringSubmatch(tag)
	if m == nil {
		return "", fmt.Errorf("%q isn't a release's tag, such as v0.1.0", tag)
	}
	heading := "## [" + m[1] + "]"
	var body []string
	in := false
	for line := range strings.Lines(changelog) {
		switch {
		case strings.HasPrefix(line, heading):
			in = true
		case in && strings.HasPrefix(line, "## ["):
			in = false
		case in:
			body = append(body, line)
		}
		if !in && len(body) > 0 {
			break
		}
	}
	out := strings.TrimSpace(strings.Join(body, ""))
	if out == "" {
		return "", errors.New("the CHANGELOG has no section " + heading + ", or it's empty; move the Unreleased changes into it")
	}
	return out + "\n", nil
}
