package hooktest

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// repoRoot is the repository root, relative to this package's directory,
// where go test runs.
const repoRoot = "../.."

// root returns the repository root as an absolute path.
func root(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// pinnedTool returns the path of a tool the tests need: the pinned binary
// that `make test` names in the environment variable env, else name on PATH.
// It skips the test when there's neither.
func pinnedTool(t *testing.T, env, name string) string {
	t.Helper()
	if p := os.Getenv(env); p != "" {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("%s: %v", env, err)
		}
		return p
	}
	p, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("no %s: %s isn't set and %s isn't on PATH; `make test` provides the pinned one", name, env, name)
	}
	return p
}

// hostTool returns the path of a standard command on the test's PATH.
func hostTool(t *testing.T, name string) string {
	t.Helper()
	p, err := exec.LookPath(name)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return p
}

// binDir returns a new directory holding exactly the given commands, as
// links named after each key. As PATH, it controls what a hook can run.
func binDir(t *testing.T, cmds map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, target := range cmds {
		if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// hookCommand returns the command of the single command hook registered for
// event and matcher in .claude/settings.json.
func hookCommand(t *testing.T, event, matcher string) string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(root(t), ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(src, &settings); err != nil {
		t.Fatalf(".claude/settings.json: %v", err)
	}
	var cmds []string
	for _, m := range settings.Hooks[event] {
		if m.Matcher != matcher {
			continue
		}
		for _, h := range m.Hooks {
			if h.Type == "command" {
				cmds = append(cmds, h.Command)
			}
		}
	}
	if len(cmds) != 1 {
		t.Fatalf(".claude/settings.json: want 1 %s command hook for %q, found %d", event, matcher, len(cmds))
	}
	return cmds[0]
}

// runHook runs a hook command through sh, as Claude Code does, with input as
// its JSON on stdin. It returns stdout and fails the test if the hook exits
// non-zero.
func runHook(t *testing.T, command string, input any, env []string) string {
	t.Helper()
	in, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), "/bin/sh", "-c", command)
	cmd.Env = env
	cmd.Stdin = bytes.NewReader(in)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("hook failed: %v\nstderr: %s", err, stderr.String())
	}
	return stdout.String()
}
