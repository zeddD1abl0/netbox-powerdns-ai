package main

import (
	"strings"
	"testing"
)

const changelog = `# Changelog

## [Unreleased]

### Added

- Something new.

## [0.2.0] - 2026-11-01

### Fixed

- A bug.

## [0.1.0] - 2026-10-08

### Added

- The first release.
`

func TestNotes(t *testing.T) {
	tests := []struct {
		tag, want, wantErr string
	}{
		{"v0.2.0", "### Fixed\n\n- A bug.\n", ""},
		{"v0.1.0", "### Added\n\n- The first release.\n", ""},
		{"v0.3.0", "", "no section ## [0.3.0]"},
		{"0.1.0", "", "isn't a release's tag"},
		{"v0.1.0-rc.1", "", "isn't a release's tag"},
		{"v1.2", "", "isn't a release's tag"},
	}
	for _, tt := range tests {
		got, err := notes(changelog, tt.tag)
		switch {
		case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
			t.Errorf("notes(%q): %v, want an error with %q", tt.tag, err, tt.wantErr)
		case tt.wantErr == "" && (err != nil || got != tt.want):
			t.Errorf("notes(%q) = %q, %v; want %q", tt.tag, got, err, tt.want)
		}
	}
	// An empty section is as good as none.
	if _, err := notes("## [0.1.0] - 2026-10-08\n\n## [0.0.1]\n\n- x\n", "v0.1.0"); err == nil {
		t.Error("an empty section gave notes")
	}
}
