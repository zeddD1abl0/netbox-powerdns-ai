package hooktest

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// unformatted mixes standard, third-party and internal imports in one group,
// with gofmt problems besides.
const unformatted = `package demo
import (
"github.com/zeddD1abl0/netbox-powerdns-ai/internal/version"
"fmt"
"go.yaml.in/yaml/v3"
)
var (
_ = fmt.Sprint
_  = yaml.Marshal
_ = version.String
)
`

// formatted is unformatted after the hook: gofmt'd, with the internal import
// in a group of its own, as golangci-lint's goimports settings want.
const formatted = `package demo

import (
	"fmt"

	"go.yaml.in/yaml/v3"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/version"
)

var (
	_ = fmt.Sprint
	_ = yaml.Marshal
	_ = version.String
)
`

// TestEditHook runs the PostToolUse hook that .claude/settings.json
// registers for Write and Edit, and checks that its result passes the
// repository's golangci-lint formatters.
func TestEditHook(t *testing.T) {
	root := root(t)
	command := hookCommand(t, "PostToolUse", "Write|Edit")
	jq := pinnedTool(t, "HOOKTEST_JQ", "jq")
	lint := pinnedTool(t, "HOOKTEST_GOLANGCI_LINT", "golangci-lint")
	env := append(os.Environ(),
		"PATH="+binDir(t, map[string]string{"jq": jq})+string(os.PathListSeparator)+os.Getenv("PATH"),
		"CLAUDE_PROJECT_DIR="+root)
	// A space in the path checks the hook's quoting.
	dir := filepath.Join(t.TempDir(), "a dir")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	lintFmt := func(file string) ([]byte, error) {
		return exec.CommandContext(t.Context(), lint, "fmt", "--diff", "--config", filepath.Join(root, ".golangci.yml"), file).CombinedOutput()
	}

	for _, c := range []struct {
		name  string
		input func(file string) any
	}{
		{"path in tool_input", func(f string) any {
			return map[string]any{"tool_input": map[string]string{"file_path": f}}
		}},
		{"path in tool_response", func(f string) any {
			return map[string]any{"tool_input": map[string]string{}, "tool_response": map[string]string{"filePath": f}}
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			file := filepath.Join(dir, "demo.go")
			write(t, file, unformatted)
			if out, err := lintFmt(file); err == nil {
				t.Fatalf("golangci-lint fmt --diff passed the unformatted file, so it can't judge the hook:\n%s", out)
			}

			runHook(t, command, c.input(file), env)
			if got := read(t, file); got != formatted {
				t.Fatalf("after the hook:\n%s\nwant:\n%s", got, formatted)
			}
			if out, err := lintFmt(file); err != nil || len(out) > 0 {
				t.Errorf("golangci-lint fmt --diff disagrees with the hook: %v\n%s", err, out)
			}
		})
	}

	t.Run("other files untouched", func(t *testing.T) {
		file := filepath.Join(dir, "notes.md")
		const notes = "#  Not   Go\nimport (\n\"fmt\"\n)\n"
		write(t, file, notes)
		runHook(t, command, map[string]any{"tool_input": map[string]string{"file_path": file}}, env)
		if got := read(t, file); got != notes {
			t.Errorf("the hook changed %s:\n%s", filepath.Base(file), got)
		}
	})
}

func write(t *testing.T, file, content string) {
	t.Helper()
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, file string) string {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
