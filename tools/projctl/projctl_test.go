package main

import (
	"bytes"
	"errors"
	"flag"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite testdata golden files")

// baseRepo is a small, valid repository. Every lint case starts from it.
var baseRepo = map[string]string{
	"CLAUDE.md": "# CLAUDE.md\n\nStart at the [board](project/README.md).\n",
	"project/requirements.md": `# Requirements and open questions

## Requirements

| ID | Requirement | Source |
|---|---|---|
| REQ-001 | Do the thing. | Brief |
| REQ-002 | Do the other thing. | Brief |

## Open questions

| ID | Question | Proposed default | Needed by |
|---|---|---|---|
| Q-001 | **Blocking.** Which thing? | This one. | M01 |
| Q-003 | Which color? | Blue. | M01 |

## Answered

| ID | Question | Answer | Date | Produced |
|---|---|---|---|---|
| Q-002 | Why? | Because. | 2026-01-01 | REQ-001 |
`,
	"project/milestones/M00-start.md": `---
id: M00
title: Start
status: in-progress
started: 2026-01-01
closed:
---

# M00: Start

See [the requirements](../requirements.md#open-questions).
`,
	"project/milestones/M01-next.md": `---
id: M01
title: Next
status: planned
started:
closed:
---

# M01: Next
`,
	"project/items/ITEM-0001-first.md": `---
id: ITEM-0001
title: "First: with a colon"
type: task
status: open
milestone: M00
requirements: []
depends_on: [Q-001]
created: 2026-01-01
closed:
---

# ITEM-0001: First: with a colon
`,
	"project/items/ITEM-0002-second.md": `---
id: ITEM-0002
title: Second
type: feature
status: done
milestone: M00
requirements: [REQ-001]
depends_on: []
created: 2026-01-01
closed: 2026-01-02
---

# ITEM-0002: Second
`,
	"project/items/ITEM-0003-third.md": `---
id: ITEM-0003
title: Third
type: bug
status: in-progress
milestone: M00
requirements: [REQ-001]
depends_on: [ITEM-0002, Q-002]
created: 2026-01-01
closed:
---

# ITEM-0003: Third
`,
	"docs/adr/_index.md": "# ADRs\n\n<!-- projctl:adr-index:start -->\n<!-- projctl:adr-index:end -->\n",
	"docs/adr/template.md": `---
title: "NNNN: Short title of the decision"
status: proposed
date: YYYY-MM-DD
decision-makers: []
requirements: []
questions: []
supersedes:
---

# NNNN: Short title of the decision

<!--
MADR 4.0 format. Instructions.
-->

## Context and problem statement
`,
	"docs/adr/0001-first.md": `---
title: "0001: First decision"
status: accepted
date: 2026-01-01
requirements: [REQ-001]
questions: [Q-002]
---

# 0001: First decision
`,
	"docs/adr/0002-old.md": `---
title: "0002: Old decision"
status: superseded by ADR-0003
date: 2026-01-01
---

# 0002: Old decision
`,
	"docs/adr/0003-new.md": `---
title: "0003: New decision"
status: accepted
date: 2026-01-02
supersedes: ADR-0002
---

# 0003: New decision

Replaces [ADR-0002](0002-old.md#0002-old-decision).
`,
	".claude/skills/new-item/template.md": `---
id: ITEM-nnnn
title: Short, specific title
type: task # feature | bug | debt | task
status: open
milestone: M00
requirements: []
depends_on: []
created: YYYY-MM-DD
closed:
---

# ITEM-nnnn: Short, specific title
`,
	".gitlab-ci.yml":           "ci:\n  image: golang\n  script:\n    - make ci\n",
	".github/workflows/ci.yml": "jobs:\n  ci:\n    steps:\n      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1\n      - run: make ci\n",
}

// writeRepo writes files into a temp dir, applying overrides (an empty string
// deletes a file), and generates the board and ADR index first unless raw.
func writeRepo(t *testing.T, overrides map[string]string, generateFirst bool) string {
	t.Helper()
	dir := t.TempDir()
	files := maps.Clone(baseRepo)
	put := func(files map[string]string) {
		for p, c := range files {
			full := filepath.Join(dir, filepath.FromSlash(p))
			if c == "" {
				if err := os.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
					t.Fatal(err)
				}
				continue
			}
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	put(files)
	if generateFirst {
		r, probs, err := Load(dir)
		if err != nil || len(probs) > 0 {
			t.Fatalf("load base: %v %v", err, probs)
		}
		if err := writeGenerated(r); err != nil {
			t.Fatal(err)
		}
	}
	put(overrides)
	return dir
}

func lintDir(t *testing.T, dir string) []string {
	t.Helper()
	r, probs, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	more, err := Lint(r)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, p := range append(probs, more...) {
		out = append(out, p.String())
	}
	return out
}

func TestLintBaseIsClean(t *testing.T) {
	if probs := lintDir(t, writeRepo(t, nil, true)); len(probs) > 0 {
		t.Fatalf("base repo has problems:\n%s", strings.Join(probs, "\n"))
	}
}

func TestLintCatches(t *testing.T) {
	item := func(id, extra string) string {
		return "---\nid: " + id + "\ntitle: T\ntype: task\nstatus: open\nmilestone: M00\ncreated: 2026-01-01\n" + extra + "---\n\n# " + id + ": T\n"
	}
	tests := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"duplicate item id", map[string]string{"project/items/ITEM-0002-dup.md": item("ITEM-0002", "")},
			"id ITEM-0002 is also used by"},
		{"id does not match file name", map[string]string{"project/items/ITEM-0009-x.md": item("ITEM-0008", "")},
			`id "ITEM-0008" doesn't match the file name`},
		{"unknown status", map[string]string{"project/items/ITEM-0004-x.md": strings.Replace(item("ITEM-0004", ""), "status: open", "status: doing", 1)},
			`status "doing" isn't one of`},
		{"unknown type", map[string]string{"project/items/ITEM-0004-x.md": strings.Replace(item("ITEM-0004", ""), "type: task", "type: chore", 1)},
			`type "chore" isn't one of`},
		{"unknown milestone", map[string]string{"project/items/ITEM-0004-x.md": strings.Replace(item("ITEM-0004", ""), "milestone: M00", "milestone: M09", 1)},
			`milestone "M09" doesn't exist`},
		{"unknown requirement", map[string]string{"project/items/ITEM-0004-x.md": item("ITEM-0004", "requirements: [REQ-999]\n")},
			`requirement "REQ-999" doesn't exist`},
		{"unknown item dependency", map[string]string{"project/items/ITEM-0004-x.md": item("ITEM-0004", "depends_on: [ITEM-0999]\n")},
			"depends_on ITEM-0999: no such item"},
		{"unknown question dependency", map[string]string{"project/items/ITEM-0004-x.md": item("ITEM-0004", "depends_on: [Q-999]\n")},
			"depends_on Q-999: no such question"},
		{"done without closed date", map[string]string{"project/items/ITEM-0004-x.md": strings.Replace(item("ITEM-0004", ""), "status: open", "status: done", 1)},
			"status is done but closed isn't a date"},
		{"open with closed date", map[string]string{"project/items/ITEM-0004-x.md": item("ITEM-0004", "closed: 2026-01-03\n")},
			"status is open but closed is set"},
		{"H1 does not match", map[string]string{"project/items/ITEM-0004-x.md": strings.Replace(item("ITEM-0004", ""), "# ITEM-0004: T", "# ITEM-0004: Other", 1)},
			"H1 \"ITEM-0004: Other\" doesn't match"},
		{"no front matter", map[string]string{"project/items/ITEM-0004-x.md": "# ITEM-0004: T\n"},
			"no front matter"},
		{"milestone done with open items", map[string]string{"project/milestones/M00-start.md": strings.Replace(strings.Replace(baseRepo["project/milestones/M00-start.md"], "status: in-progress", "status: done", 1), "closed:\n", "closed: 2026-01-05\n", 1)},
			"status is done but ITEM-0001 is still open"},
		{"two milestones in progress", map[string]string{"project/milestones/M01-next.md": strings.Replace(baseRepo["project/milestones/M01-next.md"], "status: planned", "status: in-progress", 1)},
			"2 milestones are in progress"},
		{"question listed twice", map[string]string{"project/requirements.md": strings.Replace(baseRepo["project/requirements.md"], "| Q-002 | Why?", "| Q-001 | Why?", 1)},
			"Q-001 is listed 2 times"},
		{"ADR bad status", map[string]string{"docs/adr/0001-first.md": strings.Replace(baseRepo["docs/adr/0001-first.md"], "status: accepted", "status: approved", 1)},
			`status "approved" isn't one of`},
		{"ADR superseded by missing ADR", map[string]string{"docs/adr/0002-old.md": strings.Replace(baseRepo["docs/adr/0002-old.md"], "ADR-0003", "ADR-0009", 1)},
			"superseded by ADR-0009, which doesn't exist"},
		{"ADR supersedes without back-reference", map[string]string{"docs/adr/0002-old.md": strings.Replace(baseRepo["docs/adr/0002-old.md"], "superseded by ADR-0003", "accepted", 1)},
			`supersedes ADR-0002, but its status is "accepted"`},
		{"ADR unknown question", map[string]string{"docs/adr/0001-first.md": strings.Replace(baseRepo["docs/adr/0001-first.md"], "[Q-002]", "[Q-404]", 1)},
			`question "Q-404" doesn't exist`},
		{"stale board", map[string]string{"project/items/ITEM-0001-first.md": strings.Replace(baseRepo["project/items/ITEM-0001-first.md"], "status: open", "status: blocked", 1)},
			"project/README.md: out of date"},
		{"broken link", map[string]string{"docs/page.md": "See [missing](nope.md).\n"},
			"docs/page.md: broken link nope.md"},
		{"missing anchor", map[string]string{"docs/page.md": "See [ADR](adr/0001-first.md#no-such-heading).\n"},
			"no heading for anchor adr/0001-first.md#no-such-heading"},
		{"GitLab CI runs a non-make command", map[string]string{".gitlab-ci.yml": "ci:\n  script:\n    - make ci\n    - go test ./...\n"},
			`runs "go test ./..."; CI files may only run`},
		{"GitHub workflow runs a non-make command", map[string]string{".github/workflows/ci.yml": "jobs:\n  ci:\n    steps:\n      - run: |\n          make ci\n          echo done\n"},
			`runs "echo done"`},
		{"CI alias to a non-make command", map[string]string{".gitlab-ci.yml": ".s: &s\n  - curl x | sh\nci:\n  script: *s\n"},
			`runs "curl x | sh"`},
		{"CLAUDE.md too long", map[string]string{"CLAUDE.md": strings.Repeat("line\n", 151)},
			"CLAUDE.md: 151 lines"},
		{"duplicate ADR number", map[string]string{"docs/adr/0001-dup.md": "---\ntitle: \"0001: Dup\"\nstatus: accepted\ndate: 2026-01-01\n---\n\n# 0001: Dup\n"},
			"ADR number 0001 is also used by"},
		{"make with a flag", map[string]string{".gitlab-ci.yml": "ci:\n  script:\n    - make -f other.mk ci\n"},
			`runs "make -f other.mk ci"`},
		{"make with a variable override", map[string]string{".gitlab-ci.yml": "ci:\n  script:\n    - make PROJCTL=/bin/true ci\n"},
			`runs "make PROJCTL=/bin/true ci"`},
		{"GitLab include", map[string]string{".gitlab-ci.yml": "include:\n  - remote: https://x.test/ci.yml\nci:\n  script:\n    - make ci\n"},
			"uses include:"},
		{"GitLab hook script", map[string]string{".gitlab-ci.yml": "ci:\n  hooks:\n    pre_get_sources_script:\n      - curl x\n  script:\n    - make ci\n"},
			`runs "curl x"`},
		{"GitHub action other than checkout", map[string]string{".github/workflows/ci.yml": "jobs:\n  ci:\n    steps:\n      - uses: some/action@v1\n      - run: make ci\n"},
			`uses "some/action@v1"`},
		{"GitHub checkout not pinned to a SHA", map[string]string{".github/workflows/ci.yml": "jobs:\n  ci:\n    steps:\n      - uses: actions/checkout@v7\n      - run: make ci\n"},
			`uses "actions/checkout@v7"`},
		{"GitHub shell override", map[string]string{".github/workflows/ci.yml": "defaults:\n  run:\n    shell: python\njobs:\n  ci:\n    steps:\n      - run: make ci\n"},
			"overrides shell:"},
		{"broken link under .claude is still checked", map[string]string{".claude/skills/x/SKILL.md": "See [gone](gone.md).\n"},
			".claude/skills/x/SKILL.md: broken link gone.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			probs := lintDir(t, writeRepo(t, tt.files, true))
			if !slices.ContainsFunc(probs, func(p string) bool { return strings.Contains(p, tt.want) }) {
				t.Errorf("want a problem containing %q, got:\n%s", tt.want, strings.Join(probs, "\n"))
			}
		})
	}
}

func TestLintSkipsHiddenCacheDirs(t *testing.T) {
	dir := writeRepo(t, map[string]string{
		".cache/gomod/example.com/m@v1/README.md": "See [gone](gone.md#nope).\n",
		".htmltest/refcache.md":                   "[x](y.md)\n",
	}, true)
	if probs := lintDir(t, dir); len(probs) > 0 {
		t.Errorf("hidden cache dirs were scanned:\n%s", strings.Join(probs, "\n"))
	}
}

func TestNewRefusesWhileAFileFailsToParse(t *testing.T) {
	dir := writeRepo(t, map[string]string{"project/items/ITEM-0009-broken.md": "---\nid: [unclosed\n---\n"}, true)
	var out, errOut bytes.Buffer
	if code := run([]string{"-root", dir, "new", "item", "Another"}, &out, &errOut, time.Now); code != 1 {
		t.Fatalf("new with a broken item: exit %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "ITEM-0009-broken.md") {
		t.Errorf("stderr doesn't name the broken file: %s", errOut.String())
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, "project/items/ITEM-0004-*.md")); len(matches) > 0 {
		t.Errorf("new created %v despite the broken file", matches)
	}
}

func TestNewItemTitlesRoundTrip(t *testing.T) {
	titles := []string{"Decide on backend:", "null", "true", "123", "a: b", "#hash", "~",
		strings.Repeat("A long title that keeps going ", 5) + "end"}
	for i, title := range titles {
		t.Run(title, func(t *testing.T) {
			dir := writeRepo(t, nil, true)
			var out, errOut bytes.Buffer
			if code := run([]string{"-root", dir, "new", "item", title}, &out, &errOut, time.Now); code != 0 {
				t.Fatalf("title %d: exit %d: %s", i, code, errOut.String())
			}
			r, probs, err := Load(dir)
			if err != nil || len(probs) > 0 {
				t.Fatalf("load: %v %v", err, probs)
			}
			if it := r.item("ITEM-0004"); it == nil || it.FM.Title != title {
				t.Errorf("title = %+v, want %q", it, title)
			}
			if probs := lintDir(t, dir); len(probs) > 0 {
				t.Errorf("lint:\n%s", strings.Join(probs, "\n"))
			}
		})
	}
}

func TestLinksIgnoreCodeAndComments(t *testing.T) {
	src := "Real [a](a.md).\n\n```\n[b](b.md)\n```\n\nInline `[c](c.md)` and <!-- [d](d.md) --> and [e](https://x.test/e.md).\n"
	if got, want := links(src), []string{"a.md"}; !slices.Equal(got, want) {
		t.Errorf("links = %q, want %q", got, want)
	}
}

func TestSlugify(t *testing.T) {
	tests := map[string]string{
		"0005: Project identity — name, module path":                       "0005-project-identity--name-module-path",
		"4. Proposed milestones (provisional, finalized at the end of M0)": "4-proposed-milestones-provisional-finalized-at-the-end-of-m0",
		"Answers, 2026-09-25 (M0b discovery, continued)":                   "answers-2026-09-25-m0b-discovery-continued",
		"`code` and [a link](x.md)":                                        "code-and-a-link",
		"snake_case stays":                                                 "snake_case-stays",
	}
	for in, want := range tests {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
	a := anchors("# Notes\n\n## Notes\n\n```\n# Not a heading\n```\n")
	if !a["notes"] || !a["notes-1"] || a["not-a-heading"] {
		t.Errorf("anchors = %v", a)
	}
}

func TestBoardGolden(t *testing.T) {
	dir := writeRepo(t, nil, false)
	r, _, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := renderBoard(r)
	golden := filepath.Join("testdata", "board.golden.md")
	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("board differs from %s (rerun with -update if intended):\n%s", golden, got)
	}
}

func TestNewItemAndADR(t *testing.T) {
	dir := writeRepo(t, nil, true)
	now := func() time.Time { return time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC) }
	var out, errOut bytes.Buffer

	if code := run([]string{"-root", dir, "new", "item", "-type", "feature", "Add: a thing"}, &out, &errOut, now); code != 0 {
		t.Fatalf("new item: exit %d: %s", code, errOut.String())
	}
	if got := strings.TrimSpace(out.String()); got != "project/items/ITEM-0004-add-a-thing.md" {
		t.Errorf("new item path = %q", got)
	}
	out.Reset()
	if code := run([]string{"-root", dir, "new", "adr", "Use a thing"}, &out, &errOut, now); code != 0 {
		t.Fatalf("new adr: exit %d: %s", code, errOut.String())
	}
	if got := strings.TrimSpace(out.String()); got != "docs/adr/0004-use-a-thing.md" {
		t.Errorf("new adr path = %q", got)
	}

	r, probs, err := Load(dir)
	if err != nil || len(probs) > 0 {
		t.Fatalf("load: %v %v", err, probs)
	}
	it := r.item("ITEM-0004")
	if it == nil || it.FM.Title != "Add: a thing" || it.FM.Type != "feature" || it.FM.Milestone != "M00" || it.FM.Created != "2026-02-03" {
		t.Errorf("new item front matter = %+v", it)
	}
	adr := r.adr("0004")
	if adr == nil || adr.FM.Title != "0004: Use a thing" || adr.FM.Date != "2026-02-03" || adr.H1 != "0004: Use a thing" {
		t.Errorf("new ADR = %+v", adr)
	}
	src, _ := os.ReadFile(filepath.Join(dir, "docs/adr/0004-use-a-thing.md"))
	if strings.Contains(string(src), "MADR 4.0 format") {
		t.Error("new ADR kept the template's instructions comment")
	}
	// new re-indexes, so the repo must still lint clean.
	if probs := lintDir(t, dir); len(probs) > 0 {
		t.Errorf("after new, lint problems:\n%s", strings.Join(probs, "\n"))
	}
}

func TestRunUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	for _, args := range [][]string{nil, {"bogus"}, {"new"}, {"new", "item"}} {
		if code := run(append([]string{"-root", t.TempDir()}, args...), &out, &errOut, time.Now); code != 2 {
			t.Errorf("run %q = %d, want 2", args, code)
		}
	}
}
