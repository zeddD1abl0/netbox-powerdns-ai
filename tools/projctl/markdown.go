package main

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

type heading struct {
	level int
	text  string
}

var (
	atxHeadingRE  = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)
	inlineLinkRE  = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)
	inlineCodeRE  = regexp.MustCompile("`[^`\n]*`")
	htmlCommentRE = regexp.MustCompile(`(?s)<!--.*?-->`)
	mdLinkRE      = regexp.MustCompile(`\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	schemeRE      = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)
)

// proseLines returns the lines of a Markdown document that are outside fenced
// code blocks.
func proseLines(src string) []string {
	var out []string
	fence := ""
	for line := range strings.Lines(src) {
		line = strings.TrimRight(line, "\n")
		trimmed := strings.TrimLeft(line, " ")
		if fence == "" && (strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")) {
			fence = trimmed[:3]
			continue
		}
		if fence != "" {
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
			continue
		}
		out = append(out, line)
	}
	return out
}

// headings returns the ATX headings of a Markdown document.
func headings(src string) []heading {
	var out []heading
	for _, line := range proseLines(src) {
		if m := atxHeadingRE.FindStringSubmatch(line); m != nil {
			out = append(out, heading{level: len(m[1]), text: m[2]})
		}
	}
	return out
}

// slugify turns heading text into the anchor GitHub generates for it:
// lower case, links reduced to their text, and everything except letters,
// digits, hyphens and underscores dropped, with spaces becoming hyphens.
func slugify(text string) string {
	text = inlineLinkRE.ReplaceAllString(text, "$1")
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('-')
		}
	}
	return b.String()
}

// anchors returns every anchor a document's headings produce, numbering
// duplicates the way GitHub does ("x", "x-1", "x-2").
func anchors(src string) map[string]bool {
	out := map[string]bool{}
	seen := map[string]int{}
	for _, h := range headings(src) {
		s := slugify(h.text)
		if n := seen[s]; n > 0 {
			out[s+"-"+strconv.Itoa(n)] = true
		} else {
			out[s] = true
		}
		seen[s]++
	}
	return out
}

// links returns the relative link targets in a Markdown document, ignoring
// code, HTML comments and links with a URL scheme.
func links(src string) []string {
	text := strings.Join(proseLines(src), "\n")
	text = htmlCommentRE.ReplaceAllString(text, "")
	text = inlineCodeRE.ReplaceAllString(text, "")
	var out []string
	for _, m := range mdLinkRE.FindAllStringSubmatch(text, -1) {
		if !schemeRE.MatchString(m[1]) {
			out = append(out, m[1])
		}
	}
	return out
}
