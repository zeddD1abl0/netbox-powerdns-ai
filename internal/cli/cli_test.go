package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
)

// run runs nbpdns with args and the environment env, and returns its exit
// code, stdout and stderr.
func run(t *testing.T, env map[string]string, args ...string) (int, string, string) {
	t.Helper()
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, config.EnvPrefix) {
			t.Setenv(name, "")
			if err := os.Unsetenv(name); err != nil {
				t.Fatal(err)
			}
		}
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	var stdout, stderr bytes.Buffer
	code := Main(t.Context(), args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestExitCodes(t *testing.T) {
	tests := []struct {
		name       string
		env        map[string]string
		args       []string
		wantCode   int
		wantStderr string
	}{
		{"no arguments prints help", nil, nil, exitOK, ""},
		{"help", nil, []string{"--help"}, exitOK, ""},
		{"unknown command", nil, []string{"frobnicate"}, exitUsage, "Run 'nbpdns --help' for usage."},
		{"unknown flag", nil, []string{"version", "--frobnicate"}, exitUsage, "unknown flag: --frobnicate"},
		{"stray argument", nil, []string{"version", "extra"}, exitUsage, "Run 'nbpdns --help'"},
		{"stray argument to a group", nil, []string{"config", "extra"}, exitUsage, "Run 'nbpdns --help'"},
		{"bad output format", nil, []string{"version", "-o", "yaml"}, exitUsage, `"yaml" isn't table or json`},
		{"invalid configuration", map[string]string{"NBPDNS_LOG_LEVEL": "loud", "NBPDNS_TYPO": "x"}, []string{"config", "show"}, exitError, "unknown environment variable NBPDNS_TYPO"},
		{"version ignores the configuration", map[string]string{"NBPDNS_TYPO": "x"}, []string{"version"}, exitOK, ""},
		{"completion", nil, []string{"completion", "bash"}, exitOK, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _, stderr := run(t, tt.env, tt.args...)
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d; stderr:\n%s", code, tt.wantCode, stderr)
			}
			if !strings.Contains(stderr, tt.wantStderr) {
				t.Errorf("stderr doesn't contain %q:\n%s", tt.wantStderr, stderr)
			}
			if tt.wantCode == exitOK && stderr != "" {
				t.Errorf("stderr should be empty on success:\n%s", stderr)
			}
		})
	}
}

func TestVersion(t *testing.T) {
	code, out, _ := run(t, nil, "version")
	if code != exitOK || !strings.HasPrefix(out, "FIELD") || !strings.Contains(out, "go_version") {
		t.Errorf("version: exit %d, output:\n%s", code, out)
	}
	code, out, _ = run(t, nil, "version", "--output", "json")
	var info struct {
		Version   string `json:"version"`
		GoVersion string `json:"go_version"`
		Platform  string `json:"platform"`
	}
	if err := json.Unmarshal([]byte(out), &info); err != nil || code != exitOK {
		t.Fatalf("version -o json: exit %d, %v:\n%s", code, err, out)
	}
	if info.Version == "" || !strings.HasPrefix(info.GoVersion, "go") || !strings.Contains(info.Platform, "/") {
		t.Errorf("version -o json = %+v", info)
	}
}

func TestConfigShow(t *testing.T) {
	env := map[string]string{"NBPDNS_NETBOX_TOKEN": "nbt_hunter2", "NBPDNS_LOG_LEVEL": "debug"}
	code, out, stderr := run(t, env, "config", "show", "--netbox-url", "https://netbox.example.com")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	for _, want := range []string{
		"KEY", "SOURCE",
		"log.level", "debug", "env NBPDNS_LOG_LEVEL",
		"netbox.url", "https://netbox.example.com", "flag --netbox-url",
		"netbox.token", "[redacted]",
		"netbox.timeout", "30s", "default",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output doesn't contain %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "hunter2") {
		t.Errorf("output leaks the token:\n%s", out)
	}

	code, out, stderr = run(t, env, "config", "show", "-o", "json")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var settings []config.Setting
	if err := json.Unmarshal([]byte(out), &settings); err != nil {
		t.Fatalf("output isn't JSON: %v\n%s", err, out)
	}
	for _, s := range settings {
		if s.Key == "netbox.token" && (s.Value != config.Redacted || s.Source.Kind != "env" || s.Source.Name != "NBPDNS_NETBOX_TOKEN") {
			t.Errorf("netbox.token setting = %+v", s)
		}
	}
	if len(settings) != len(config.Keys()) {
		t.Errorf("got %d settings, want one per key (%d)", len(settings), len(config.Keys()))
	}
}

// TestReference checks the generated command-line reference against the
// command tree.
func TestReference(t *testing.T) {
	var b bytes.Buffer
	if err := WriteReference(&b, New(&bytes.Buffer{}, &bytes.Buffer{})); err != nil {
		t.Fatal(err)
	}
	page := b.String()
	for _, want := range []string{
		"### `nbpdns version`",
		"### `nbpdns config show`",
		"### `nbpdns completion bash`",
		"| `--netbox-url` | `url` | none |",
		"| `--netbox-token-file` | `path` | none | Read `netbox.token` from this file. |",
		"| `-o`, `--output` | `format` | `table` |",
		"(#nbpdns-config-show)",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("reference doesn't contain %q", want)
		}
	}
	if strings.Contains(page, "### `nbpdns help`") {
		t.Error("reference lists the help command")
	}
}

// TestLogLinesCarryIDs runs a command at debug level and checks that every
// log line on stderr carries the same trace, span and request IDs.
func TestLogLinesCarryIDs(t *testing.T) {
	for _, format := range []string{"json", "text"} {
		t.Run(format, func(t *testing.T) {
			env := map[string]string{"NBPDNS_LOG_LEVEL": "debug", "NBPDNS_NETBOX_TOKEN": "nbt_hunter2"}
			code, stdout, stderr := run(t, env, "config", "show", "--log-format", format)
			if code != exitOK {
				t.Fatalf("exit %d: %s", code, stderr)
			}
			if strings.Contains(stderr, "hunter2") || strings.Contains(stdout, "hunter2") {
				t.Error("the token leaked")
			}
			lines := strings.Split(strings.TrimSpace(stderr), "\n")
			if len(lines) < 2 {
				t.Fatalf("want the start and finish lines, got:\n%s", stderr)
			}
			ids := map[string]string{}
			for _, line := range lines {
				for _, key := range []string{"trace_id", "span_id", "request_id"} {
					v := field(t, format, line, key)
					if v == "" {
						t.Errorf("line has no %s: %s", key, line)
					}
					if prev, ok := ids[key]; ok && prev != v {
						t.Errorf("%s changed within one run: %s, then %s", key, prev, v)
					}
					ids[key] = v
				}
			}
		})
	}
}

// field returns key's value in one JSON or text log line.
func field(t *testing.T, format, line, key string) string {
	t.Helper()
	if format == "json" {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("not JSON: %s", line)
		}
		s, _ := m[key].(string)
		return s
	}
	for _, kv := range strings.Fields(line) {
		if v, ok := strings.CutPrefix(kv, key+"="); ok {
			return v
		}
	}
	return ""
}
