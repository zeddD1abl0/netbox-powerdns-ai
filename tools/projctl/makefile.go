package main

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

var (
	// A rule line: "target: prerequisites ## help". Assignments (:=) aren't rules.
	ruleRE      = regexp.MustCompile(`^([a-z0-9][a-z0-9-]*)[ \t]*:([^=].*|)$`)
	targetRE    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	ciImageRE   = regexp.MustCompile(`^CI_IMAGE[ \t]*:?=[ \t]*(\S+)`)
	makeTokenRE = regexp.MustCompile(`^make( |$)`)
)

// makefile is what the CI rules need from the root Makefile.
type makefile struct {
	deps    map[string][]string // target -> prerequisites that are targets
	recipe  map[string]bool     // targets with a recipe, i.e. that do work
	ciImage string
}

// loadMakefile reads the root Makefile, or returns nil if there isn't one.
func loadMakefile(root string) (*makefile, error) {
	src, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m := &makefile{deps: map[string][]string{}, recipe: map[string]bool{}}
	last := "" // the rule whose recipe lines may follow
	for line := range strings.Lines(string(src)) {
		line = strings.TrimRight(line, "\n")
		if strings.HasPrefix(line, "\t") {
			if last != "" {
				m.recipe[last] = true
			}
			continue
		}
		last = ""
		if c := ciImageRE.FindStringSubmatch(line); c != nil {
			m.ciImage = c[1]
			continue
		}
		r := ruleRE.FindStringSubmatch(line)
		if r == nil {
			continue
		}
		last = r[1]
		prereqs, _, _ := strings.Cut(r[2], "##")
		var deps []string
		for _, f := range strings.Fields(prereqs) {
			if targetRE.MatchString(f) {
				deps = append(deps, f)
			}
		}
		m.deps[r[1]] = append(m.deps[r[1]], deps...)
	}
	return m, nil
}

// work returns the targets with a recipe among targets and everything they
// depend on, transitively. Aggregates such as `ci` and `check` have no recipe,
// so running their parts separately counts the same as running them.
func (m *makefile) work(targets []string) map[string]bool {
	out := map[string]bool{}
	for t := range m.reach(targets) {
		if m.recipe[t] {
			out[t] = true
		}
	}
	return out
}

// reach returns the targets and every target they depend on, transitively.
func (m *makefile) reach(targets []string) map[string]bool {
	seen := map[string]bool{}
	var visit func(string)
	visit = func(t string) {
		if seen[t] {
			return
		}
		seen[t] = true
		for _, d := range m.deps[t] {
			visit(d)
		}
	}
	for _, t := range targets {
		visit(t)
	}
	return seen
}

// makeTargets returns the targets named by `make …` command lines.
func makeTargets(cmds []string) []string {
	var out []string
	for _, c := range cmds {
		if makeTokenRE.MatchString(c) {
			out = append(out, strings.Fields(c)[1:]...)
		}
	}
	return out
}

// lintCoverage checks that a CI file's jobs, together, run exactly the
// targets `make ci` runs, and only in the Makefile's CI_IMAGE (ADR-0016).
func (m *makefile) lintCoverage(rel string, cmds, images []string) []Problem {
	var probs []Problem
	want := m.work([]string{"ci"})
	got := m.work(makeTargets(cmds))
	var missing, extra []string
	for t := range want {
		if !got[t] {
			missing = append(missing, t)
		}
	}
	for t := range got {
		if !want[t] {
			extra = append(extra, t)
		}
	}
	slices.Sort(missing)
	slices.Sort(extra)
	if len(missing) > 0 {
		probs = append(probs, Problem{rel, "doesn't run " + strings.Join(missing, ", ") + ", which `make ci` runs"})
	}
	if len(extra) > 0 {
		probs = append(probs, Problem{rel, "runs " + strings.Join(extra, ", ") + ", which `make ci` doesn't"})
	}
	if m.ciImage != "" {
		if len(images) == 0 {
			probs = append(probs, Problem{rel, "names no image; use the Makefile's CI_IMAGE"})
		}
		for _, img := range images {
			if img != m.ciImage {
				probs = append(probs, Problem{rel, "image " + img + " isn't the Makefile's CI_IMAGE " + m.ciImage})
			}
		}
	}
	return probs
}

// imageValues collects the images named under key anywhere in the document:
// a plain scalar, or a mapping's name (GitLab) or image (GitHub).
func imageValues(n *yaml.Node, key string) []string {
	var out []string
	var walk func(*yaml.Node)
	walk = func(v *yaml.Node) {
		switch v.Kind {
		case yaml.DocumentNode, yaml.SequenceNode:
			for _, c := range v.Content {
				walk(c)
			}
		case yaml.MappingNode:
			for i := 0; i+1 < len(v.Content); i += 2 {
				k, val := v.Content[i], v.Content[i+1]
				if val.Kind == yaml.AliasNode {
					val = val.Alias
				}
				if k.Value != key {
					walk(val)
					continue
				}
				switch val.Kind {
				case yaml.ScalarNode:
					out = append(out, val.Value)
				case yaml.MappingNode:
					for j := 0; j+1 < len(val.Content); j += 2 {
						if n := val.Content[j].Value; n == "name" || n == "image" {
							out = append(out, val.Content[j+1].Value)
						}
					}
				default:
					// Other shapes name no image.
				}
			}
		default:
			// Scalars and aliases outside an image key name no image.
		}
	}
	walk(n)
	return out
}
