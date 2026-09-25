package main

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Paths of the tracking files, relative to the repository root.
const (
	itemsDir        = "project/items"
	milestonesDir   = "project/milestones"
	requirementsMD  = "project/requirements.md"
	boardMD         = "project/README.md"
	adrDir          = "docs/adr"
	adrIndexMD      = "docs/adr/_index.md"
	adrTemplateMD   = "docs/adr/template.md"
	itemTemplateMD  = ".claude/skills/new-item/template.md"
	claudeMD        = "CLAUDE.md"
	claudeMDMaxLine = 150
)

var (
	itemFileRE      = regexp.MustCompile(`^(ITEM-\d{4})-[a-z0-9-]+\.md$`)
	milestoneFileRE = regexp.MustCompile(`^(M\d{2})-[a-z0-9-]+\.md$`)
	adrFileRE       = regexp.MustCompile(`^(\d{4})-[a-z0-9-]+\.md$`)
)

// Problem is one lint finding, reported as "path: message".
type Problem struct {
	Path string
	Msg  string
}

func (p Problem) String() string { return p.Path + ": " + p.Msg }

// Item is a work item in project/items/.
type Item struct {
	Path string
	H1   string
	FM   struct {
		ID           string   `yaml:"id"`
		Title        string   `yaml:"title"`
		Type         string   `yaml:"type"`
		Status       string   `yaml:"status"`
		Milestone    string   `yaml:"milestone"`
		Requirements []string `yaml:"requirements"`
		DependsOn    []string `yaml:"depends_on"`
		Created      string   `yaml:"created"`
		Closed       string   `yaml:"closed"`
	}
}

// Closed reports whether the item is finished, one way or the other.
func (it *Item) Closed() bool { return it.FM.Status == "done" || it.FM.Status == "wontfix" }

// Milestone is a milestone file in project/milestones/.
type Milestone struct {
	Path string
	H1   string
	FM   struct {
		ID      string `yaml:"id"`
		Title   string `yaml:"title"`
		Status  string `yaml:"status"`
		Started string `yaml:"started"`
		Closed  string `yaml:"closed"`
	}
}

// ADR is an architecture decision record in docs/adr/.
type ADR struct {
	Path   string
	Number string // "0001", from the file name
	H1     string
	FM     struct {
		Title        string   `yaml:"title"`
		Status       string   `yaml:"status"`
		Date         string   `yaml:"date"`
		Requirements []string `yaml:"requirements"`
		Questions    []string `yaml:"questions"`
		Supersedes   string   `yaml:"supersedes"`
	}
}

// ShortTitle is the title without its "NNNN: " prefix.
func (a *ADR) ShortTitle() string { return strings.TrimPrefix(a.FM.Title, a.Number+": ") }

// Repo is everything projctl knows about the repository's tracking files.
type Repo struct {
	Root       string
	Items      []*Item
	Milestones []*Milestone
	ADRs       []*ADR
	Reqs       *Requirements
}

// Load reads the tracking files under root. Files that can't be parsed are
// reported as problems and left out, so lint can report everything at once.
func Load(root string) (*Repo, []Problem, error) {
	r := &Repo{Root: root}
	var probs []Problem

	reqs, rp, err := loadRequirements(root)
	if err != nil {
		return nil, nil, err
	}
	r.Reqs, probs = reqs, append(probs, rp...)

	err = eachFile(root, itemsDir, func(rel string, name string, src []byte) {
		it := &Item{Path: rel}
		if p := decode(rel, src, &it.FM, &it.H1); p != nil {
			probs = append(probs, *p)
			return
		}
		r.Items = append(r.Items, it)
	})
	if err != nil {
		return nil, nil, err
	}

	err = eachFile(root, milestonesDir, func(rel string, name string, src []byte) {
		m := &Milestone{Path: rel}
		if p := decode(rel, src, &m.FM, &m.H1); p != nil {
			probs = append(probs, *p)
			return
		}
		r.Milestones = append(r.Milestones, m)
	})
	if err != nil {
		return nil, nil, err
	}

	err = eachFile(root, adrDir, func(rel string, name string, src []byte) {
		if name == "_index.md" || name == "template.md" {
			return
		}
		a := &ADR{Path: rel}
		if m := adrFileRE.FindStringSubmatch(name); m != nil {
			a.Number = m[1]
		}
		if p := decode(rel, src, &a.FM, &a.H1); p != nil {
			probs = append(probs, *p)
			return
		}
		r.ADRs = append(r.ADRs, a)
	})
	if err != nil {
		return nil, nil, err
	}

	slices.SortFunc(r.Items, func(a, b *Item) int { return cmp.Compare(a.FM.ID, b.FM.ID) })
	slices.SortFunc(r.Milestones, func(a, b *Milestone) int { return cmp.Compare(a.FM.ID, b.FM.ID) })
	slices.SortFunc(r.ADRs, func(a, b *ADR) int { return cmp.Compare(a.Path, b.Path) })
	return r, probs, nil
}

// eachFile calls fn for every .md file directly inside dir.
func eachFile(root, dir string, fn func(rel, name string, src []byte)) error {
	entries, err := os.ReadDir(filepath.Join(root, dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		rel := dir + "/" + e.Name()
		src, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return err
		}
		fn(rel, e.Name(), src)
	}
	return nil
}

// decode parses a file's front matter into fm and finds its first H1.
func decode(rel string, src []byte, fm any, h1 *string) *Problem {
	head, body, ok := splitFrontMatter(src)
	if !ok {
		return &Problem{rel, "no front matter (the file must start with ---)"}
	}
	if err := yaml.Unmarshal(head, fm); err != nil {
		return &Problem{rel, "front matter: " + err.Error()}
	}
	*h1 = firstH1(body)
	return nil
}

// splitFrontMatter separates a leading "---" YAML block from the body.
func splitFrontMatter(src []byte) (head []byte, body string, ok bool) {
	src = bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(src, []byte("---\n")) {
		return nil, string(src), false
	}
	rest := src[4:]
	end := bytes.Index(rest, []byte("\n---\n"))
	if end < 0 {
		if bytes.HasSuffix(rest, []byte("\n---")) {
			return rest[:len(rest)-4], "", true
		}
		return nil, string(src), false
	}
	return rest[:end+1], string(rest[end+5:]), true
}

// firstH1 returns the text of the first "# " heading outside code fences.
func firstH1(body string) string {
	for _, h := range headings(body) {
		if h.level == 1 {
			return h.text
		}
	}
	return ""
}

// Requirements holds the tables in project/requirements.md.
type Requirements struct {
	Reqs     []string
	Open     []Question
	Answered []Question
}

// Question is one Q-nnn row.
type Question struct {
	ID       string
	Blocking bool
	NeededBy string
}

var rowIDRE = regexp.MustCompile(`^\|\s*((?:REQ|Q)-\d{3})\s*\|`)

func loadRequirements(root string) (*Requirements, []Problem, error) {
	src, err := os.ReadFile(filepath.Join(root, requirementsMD))
	if errors.Is(err, os.ErrNotExist) {
		return &Requirements{}, []Problem{{requirementsMD, "missing"}}, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return parseRequirements(string(src))
}

func parseRequirements(src string) (*Requirements, []Problem, error) {
	r := &Requirements{}
	var probs []Problem
	section := ""
	for line := range strings.Lines(src) {
		line = strings.TrimRight(line, "\n")
		if strings.HasPrefix(line, "## ") {
			section = strings.TrimPrefix(line, "## ")
			continue
		}
		m := rowIDRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		id := m[1]
		cells := splitRow(line)
		switch {
		case strings.HasPrefix(id, "REQ-") && section == "Requirements":
			r.Reqs = append(r.Reqs, id)
		case strings.HasPrefix(id, "Q-") && section == "Open questions":
			q := Question{ID: id}
			if len(cells) > 1 {
				q.Blocking = strings.HasPrefix(cells[1], "**Blocking.**")
			}
			if len(cells) > 0 {
				q.NeededBy = cells[len(cells)-1]
			}
			r.Open = append(r.Open, q)
		case strings.HasPrefix(id, "Q-") && section == "Answered":
			r.Answered = append(r.Answered, Question{ID: id})
		default:
			probs = append(probs, Problem{requirementsMD, fmt.Sprintf("%s row is under %q", id, section)})
		}
	}
	return r, probs, nil
}

// splitRow returns the trimmed cells of a Markdown table row.
func splitRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	parts := strings.Split(line, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// IsOpenQuestion reports whether id is listed under Open questions.
func (r *Requirements) IsOpenQuestion(id string) bool {
	return slices.ContainsFunc(r.Open, func(q Question) bool { return q.ID == id })
}

// HasQuestion reports whether id is listed at all.
func (r *Requirements) HasQuestion(id string) bool {
	return r.IsOpenQuestion(id) || slices.ContainsFunc(r.Answered, func(q Question) bool { return q.ID == id })
}

// Lookup helpers.

func (r *Repo) item(id string) *Item {
	for _, it := range r.Items {
		if it.FM.ID == id {
			return it
		}
	}
	return nil
}

func (r *Repo) milestone(id string) *Milestone {
	for _, m := range r.Milestones {
		if m.FM.ID == id {
			return m
		}
	}
	return nil
}

func (r *Repo) adr(number string) *ADR {
	for _, a := range r.ADRs {
		if a.Number == number {
			return a
		}
	}
	return nil
}

// Current returns the milestone in progress, if any.
func (r *Repo) Current() *Milestone {
	for _, m := range r.Milestones {
		if m.FM.Status == "in-progress" {
			return m
		}
	}
	return nil
}

// Unresolved returns the item's dependencies that still hold it up: items not
// yet closed, and questions still open.
func (r *Repo) Unresolved(it *Item) []string {
	var out []string
	for _, d := range it.FM.DependsOn {
		switch {
		case strings.HasPrefix(d, "ITEM-"):
			if dep := r.item(d); dep == nil || !dep.Closed() {
				out = append(out, d)
			}
		case strings.HasPrefix(d, "Q-"):
			if r.Reqs.IsOpenQuestion(d) {
				out = append(out, d)
			}
		default:
			out = append(out, d)
		}
	}
	return out
}
