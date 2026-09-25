package main

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

var (
	itemTypes         = []string{"feature", "bug", "debt", "task"}
	itemStatuses      = []string{"open", "in-progress", "blocked", "done", "wontfix"}
	milestoneStatuses = []string{"planned", "in-progress", "done"}
	adrStatuses       = []string{"proposed", "accepted", "rejected", "deprecated"}

	itemIDRE      = regexp.MustCompile(`^ITEM-\d{4}$`)
	milestoneIDRE = regexp.MustCompile(`^M\d{2}$`)
	reqIDRE       = regexp.MustCompile(`^REQ-\d{3}$`)
	questionIDRE  = regexp.MustCompile(`^Q-\d{3}$`)
	dateRE        = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	supersededRE  = regexp.MustCompile(`^superseded by ADR-(\d{4})$`)
	makeOnlyRE    = regexp.MustCompile(`^make( [A-Za-z0-9_.=/-]+)+$`)

	// skipDirs are never searched for Markdown links.
	skipDirs = []string{".git", "node_modules", "_vendor", "public", "resources", "testdata"}
)

// Lint runs every check and returns the problems, sorted.
func Lint(r *Repo) ([]Problem, error) {
	var probs []Problem
	add := func(p, format string, args ...any) { probs = append(probs, Problem{p, fmt.Sprintf(format, args...)}) }

	lintRequirements(r, add)
	lintMilestones(r, add)
	lintItems(r, add)
	lintADRs(r, add)

	gen, err := generate(r)
	if err != nil {
		add(adrIndexMD, "%v", err)
	}
	for _, g := range gen {
		have, err := os.ReadFile(filepath.Join(r.Root, g.Path))
		if err != nil || string(have) != g.Content {
			add(g.Path, "out of date; run `make project`")
		}
	}

	lp, err := lintLinks(r.Root)
	if err != nil {
		return nil, err
	}
	probs = append(probs, lp...)

	cp, err := lintCI(r.Root)
	if err != nil {
		return nil, err
	}
	probs = append(probs, cp...)

	if src, err := os.ReadFile(filepath.Join(r.Root, claudeMD)); err == nil {
		if n := strings.Count(string(src), "\n"); n > claudeMDMaxLine {
			add(claudeMD, "%d lines; keep it to %d or fewer", n, claudeMDMaxLine)
		}
	}

	slices.SortFunc(probs, func(a, b Problem) int { return strings.Compare(a.String(), b.String()) })
	return slices.Compact(probs), nil
}

func lintRequirements(r *Repo, add func(string, string, ...any)) {
	seen := map[string]int{}
	for _, id := range r.Reqs.Reqs {
		seen[id]++
	}
	for _, q := range r.Reqs.Open {
		seen[q.ID]++
	}
	for _, q := range r.Reqs.Answered {
		seen[q.ID]++
	}
	for _, id := range slices.Sorted(maps.Keys(seen)) {
		if seen[id] > 1 {
			add(requirementsMD, "%s is listed %d times", id, seen[id])
		}
	}
}

func lintMilestones(r *Repo, add func(string, string, ...any)) {
	ids := map[string]string{}
	inProgress := 0
	for _, m := range r.Milestones {
		p, fm := m.Path, m.FM
		if want := fileID(p, milestoneFileRE); want != fm.ID {
			add(p, "id %q doesn't match the file name (want %q)", fm.ID, want)
		}
		if !milestoneIDRE.MatchString(fm.ID) {
			add(p, "id %q isn't of the form Mnn", fm.ID)
		}
		if prev, dup := ids[fm.ID]; dup {
			add(p, "id %s is also used by %s", fm.ID, prev)
		}
		ids[fm.ID] = p
		if !slices.Contains(milestoneStatuses, fm.Status) {
			add(p, "status %q isn't one of %s", fm.Status, strings.Join(milestoneStatuses, ", "))
		}
		if fm.Status == "in-progress" {
			inProgress++
		}
		checkH1(add, p, m.H1, fm.ID+": "+fm.Title)
		if fm.Status == "done" {
			if !dateRE.MatchString(fm.Closed) {
				add(p, "status is done but closed isn't a date")
			}
			for _, it := range r.Items {
				if it.FM.Milestone == fm.ID && !it.Closed() {
					add(p, "status is done but %s is still %s", it.FM.ID, it.FM.Status)
				}
			}
		}
	}
	if inProgress > 1 {
		add(milestonesDir, "%d milestones are in progress; at most one may be", inProgress)
	}
}

func lintItems(r *Repo, add func(string, string, ...any)) {
	ids := map[string]string{}
	for _, it := range r.Items {
		p, fm := it.Path, it.FM
		if want := fileID(p, itemFileRE); want != fm.ID {
			add(p, "id %q doesn't match the file name (want %q)", fm.ID, want)
		}
		if !itemIDRE.MatchString(fm.ID) {
			add(p, "id %q isn't of the form ITEM-nnnn", fm.ID)
		}
		if prev, dup := ids[fm.ID]; dup {
			add(p, "id %s is also used by %s", fm.ID, prev)
		}
		ids[fm.ID] = p
		if fm.Title == "" {
			add(p, "title is empty")
		}
		if !slices.Contains(itemTypes, fm.Type) {
			add(p, "type %q isn't one of %s", fm.Type, strings.Join(itemTypes, ", "))
		}
		if !slices.Contains(itemStatuses, fm.Status) {
			add(p, "status %q isn't one of %s", fm.Status, strings.Join(itemStatuses, ", "))
		}
		if r.milestone(fm.Milestone) == nil {
			add(p, "milestone %q doesn't exist", fm.Milestone)
		}
		if !dateRE.MatchString(fm.Created) {
			add(p, "created %q isn't a date", fm.Created)
		}
		switch {
		case it.Closed() && !dateRE.MatchString(fm.Closed):
			add(p, "status is %s but closed isn't a date", fm.Status)
		case !it.Closed() && fm.Closed != "":
			add(p, "status is %s but closed is set", fm.Status)
		}
		for _, req := range fm.Requirements {
			if !reqIDRE.MatchString(req) || !slices.Contains(r.Reqs.Reqs, req) {
				add(p, "requirement %q doesn't exist", req)
			}
		}
		for _, d := range fm.DependsOn {
			switch {
			case itemIDRE.MatchString(d):
				if r.item(d) == nil {
					add(p, "depends_on %s: no such item", d)
				}
			case questionIDRE.MatchString(d):
				if !r.Reqs.HasQuestion(d) {
					add(p, "depends_on %s: no such question", d)
				}
			default:
				add(p, "depends_on %q isn't an ITEM or Q id", d)
			}
		}
		checkH1(add, p, it.H1, fm.ID+": "+fm.Title)
	}
}

func lintADRs(r *Repo, add func(string, string, ...any)) {
	for _, a := range r.ADRs {
		p, fm := a.Path, a.FM
		if a.Number == "" {
			add(p, "file name isn't of the form NNNN-short-title.md")
			continue
		}
		if !strings.HasPrefix(fm.Title, a.Number+": ") {
			add(p, "title %q doesn't start with %q", fm.Title, a.Number+": ")
		}
		checkH1(add, p, a.H1, fm.Title)
		if !dateRE.MatchString(fm.Date) {
			add(p, "date %q isn't a date", fm.Date)
		}
		if m := supersededRE.FindStringSubmatch(fm.Status); m != nil {
			if by := r.adr(m[1]); by == nil {
				add(p, "superseded by ADR-%s, which doesn't exist", m[1])
			} else if by.FM.Supersedes != "ADR-"+a.Number {
				add(p, "superseded by ADR-%s, but that ADR's supersedes is %q", m[1], by.FM.Supersedes)
			}
		} else if !slices.Contains(adrStatuses, fm.Status) {
			add(p, "status %q isn't one of %s, or superseded by ADR-NNNN", fm.Status, strings.Join(adrStatuses, ", "))
		}
		if fm.Supersedes != "" {
			old := r.adr(strings.TrimPrefix(fm.Supersedes, "ADR-"))
			if old == nil {
				add(p, "supersedes %s, which doesn't exist", fm.Supersedes)
			} else if old.FM.Status != "superseded by ADR-"+a.Number {
				add(p, "supersedes %s, but its status is %q", fm.Supersedes, old.FM.Status)
			}
		}
		for _, req := range fm.Requirements {
			if !slices.Contains(r.Reqs.Reqs, req) {
				add(p, "requirement %q doesn't exist", req)
			}
		}
		for _, q := range fm.Questions {
			if !r.Reqs.HasQuestion(q) {
				add(p, "question %q doesn't exist", q)
			}
		}
	}
}

func checkH1(add func(string, string, ...any), p, have, want string) {
	if have != want {
		add(p, "H1 %q doesn't match the front matter (want %q)", have, want)
	}
}

func fileID(p string, re *regexp.Regexp) string {
	if m := re.FindStringSubmatch(path.Base(p)); m != nil {
		return m[1]
	}
	return ""
}

// lintLinks checks every relative link in every Markdown file: the target
// must exist and, for a Markdown target, so must the #anchor.
func lintLinks(root string) ([]Problem, error) {
	var probs []Problem
	cache := map[string]map[string]bool{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != root && slices.Contains(skipDirs, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".md") {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		for _, target := range links(string(src)) {
			file, anchor, _ := strings.Cut(target, "#")
			dest := p
			if file != "" {
				dest = filepath.Join(filepath.Dir(p), filepath.FromSlash(file))
			}
			info, err := os.Stat(dest)
			if err != nil {
				probs = append(probs, Problem{rel, "broken link " + target})
				continue
			}
			if anchor == "" || info.IsDir() || !strings.HasSuffix(dest, ".md") {
				continue
			}
			a, ok := cache[dest]
			if !ok {
				s, err := os.ReadFile(dest)
				if err != nil {
					return err
				}
				a = anchors(string(s))
				cache[dest] = a
			}
			if !a[strings.ToLower(anchor)] {
				probs = append(probs, Problem{rel, "no heading for anchor " + target})
			}
		}
		return nil
	})
	return probs, err
}

// lintCI checks that forge CI files only run make targets (ADR-0013).
func lintCI(root string) ([]Problem, error) {
	var probs []Problem
	check := func(rel string, keys []string) error {
		src, err := os.ReadFile(filepath.Join(root, rel))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		var doc yaml.Node
		if err := yaml.Unmarshal(src, &doc); err != nil {
			probs = append(probs, Problem{rel, "invalid YAML: " + err.Error()})
			return nil
		}
		for _, cmd := range scriptLines(&doc, keys) {
			if !makeOnlyRE.MatchString(cmd) {
				probs = append(probs, Problem{rel, fmt.Sprintf("runs %q; CI files may only run make targets", cmd)})
			}
		}
		return nil
	}
	if err := check(".gitlab-ci.yml", []string{"script", "before_script", "after_script"}); err != nil {
		return nil, err
	}
	workflows, _ := filepath.Glob(filepath.Join(root, ".github", "workflows", "*.y*ml"))
	for _, wf := range workflows {
		rel, _ := filepath.Rel(root, wf)
		if err := check(filepath.ToSlash(rel), []string{"run"}); err != nil {
			return nil, err
		}
	}
	return probs, nil
}

// scriptLines collects every non-empty command line under the given keys,
// anywhere in the document, following YAML aliases.
func scriptLines(n *yaml.Node, keys []string) []string {
	var out []string
	var scalars func(*yaml.Node)
	scalars = func(v *yaml.Node) {
		switch v.Kind {
		case yaml.AliasNode:
			scalars(v.Alias)
		case yaml.ScalarNode:
			for line := range strings.Lines(v.Value) {
				if line = strings.TrimSpace(line); line != "" {
					out = append(out, line)
				}
			}
		case yaml.SequenceNode:
			for _, c := range v.Content {
				scalars(c)
			}
		}
	}
	var walk func(*yaml.Node)
	walk = func(v *yaml.Node) {
		switch v.Kind {
		case yaml.DocumentNode, yaml.SequenceNode:
			for _, c := range v.Content {
				walk(c)
			}
		case yaml.MappingNode:
			for i := 0; i+1 < len(v.Content); i += 2 {
				if slices.Contains(keys, v.Content[i].Value) {
					scalars(v.Content[i+1])
				} else {
					walk(v.Content[i+1])
				}
			}
		}
	}
	walk(n)
	return out
}
