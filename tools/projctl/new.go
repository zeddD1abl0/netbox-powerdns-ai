package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

var nonSlugRE = regexp.MustCompile(`[^a-z0-9]+`)

// slugFor turns a title into the kebab-case part of a file name. A title with
// no letters or digits gets "untitled", so the name still matches the ID pattern.
func slugFor(title string) string {
	s := strings.Trim(nonSlugRE.ReplaceAllString(strings.ToLower(title), "-"), "-")
	if len(s) > 50 {
		s = strings.TrimRight(s[:50], "-")
	}
	if s == "" {
		return "untitled"
	}
	return s
}

// yamlString renders s as a YAML scalar, quoted whenever plain YAML would
// misread it (a trailing colon, "null", "true", a number, and so on).
func yamlString(s string) string {
	out, err := yaml.Marshal(s)
	if err != nil {
		return strconv.Quote(s)
	}
	return strings.TrimSuffix(string(out), "\n")
}

// NewItem creates the next work item from the item template.
func NewItem(r *Repo, title, milestone, typ, today string) (string, error) {
	if milestone == "" {
		cur := r.Current()
		if cur == nil {
			return "", fmt.Errorf("no milestone in progress; pass -milestone")
		}
		milestone = cur.FM.ID
	}
	if r.milestone(milestone) == nil {
		return "", fmt.Errorf("milestone %s doesn't exist", milestone)
	}
	next := 1
	for _, it := range r.Items {
		if n, err := strconv.Atoi(strings.TrimPrefix(it.FM.ID, "ITEM-")); err == nil && n >= next {
			next = n + 1
		}
	}
	id := fmt.Sprintf("ITEM-%04d", next)

	tmpl, err := os.ReadFile(filepath.Join(r.Root, itemTemplateMD))
	if err != nil {
		return "", err
	}
	s := string(tmpl)
	for _, rep := range [][2]string{
		{"id: ITEM-nnnn", "id: " + id},
		{"title: Short, specific title", "title: " + yamlString(title)},
		{"type: task", "type: " + typ},
		{"milestone: M00", "milestone: " + milestone},
		{"created: YYYY-MM-DD", "created: " + today},
		{"# ITEM-nnnn: Short, specific title", "# " + id + ": " + title},
	} {
		if !strings.Contains(s, rep[0]) {
			return "", fmt.Errorf("%s: expected %q in the template", itemTemplateMD, rep[0])
		}
		s = strings.Replace(s, rep[0], rep[1], 1)
	}
	rel := itemsDir + "/" + id + "-" + slugFor(title) + ".md"
	return rel, writeNew(r.Root, rel, s)
}

// NewADR creates the next ADR from the ADR template.
func NewADR(r *Repo, title, today string) (string, error) {
	next := 1
	for _, a := range r.ADRs {
		if n, err := strconv.Atoi(a.Number); err == nil && n >= next {
			next = n + 1
		}
	}
	num := fmt.Sprintf("%04d", next)

	tmpl, err := os.ReadFile(filepath.Join(r.Root, adrTemplateMD))
	if err != nil {
		return "", err
	}
	s := string(tmpl)
	// Drop the template's instructions comment.
	if i := strings.Index(s, "<!--\nMADR"); i >= 0 {
		if j := strings.Index(s[i:], "-->\n"); j >= 0 {
			s = s[:i] + strings.TrimLeft(s[i+j+4:], "\n")
		}
	}
	full := num + ": " + title
	for _, rep := range [][2]string{
		{`title: "NNNN: Short title of the decision"`, "title: " + yamlString(full)},
		{"date: YYYY-MM-DD", "date: " + today},
		{"# NNNN: Short title of the decision", "# " + full},
	} {
		if !strings.Contains(s, rep[0]) {
			return "", fmt.Errorf("%s: expected %q in the template", adrTemplateMD, rep[0])
		}
		s = strings.Replace(s, rep[0], rep[1], 1)
	}
	rel := adrDir + "/" + num + "-" + slugFor(title) + ".md"
	return rel, writeNew(r.Root, rel, s)
}

func writeNew(root, rel, content string) error {
	f, err := os.OpenFile(filepath.Join(root, rel), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(content); err != nil {
		return errors.Join(err, f.Close())
	}
	return f.Close()
}
