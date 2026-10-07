//go:build release

package release

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// root is the repository's root, where GoReleaser's paths start.
const root = "../.."

// An artifact is one entry of GoReleaser's dist/artifacts.json.
type artifact struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Goos   string `json:"goos"`
	Goarch string `json:"goarch"`
	Type   string `json:"type"`
}

// metadata is GoReleaser's dist/metadata.json.
type metadata struct {
	Version string `json:"version"`
	Tag     string `json:"tag"`
	Commit  string `json:"commit"`
}

// snapshot reports whether the build is a snapshot, rather than a tag's.
func (m metadata) snapshot() bool { return strings.Contains(m.Version, "SNAPSHOT") }

func readJSON(t *testing.T, name string, v any) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "dist", name))
	if err != nil {
		t.Fatalf("%v (run make release-check)", err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

func build(t *testing.T) (metadata, []artifact) {
	t.Helper()
	var m metadata
	var as []artifact
	readJSON(t, "metadata.json", &m)
	readJSON(t, "artifacts.json", &as)
	return m, as
}

func ofType(as []artifact, typ string) []artifact {
	var out []artifact
	for _, a := range as {
		if a.Type == typ {
			out = append(out, a)
		}
	}
	return out
}

// commitTime returns the commit's time, which every file in an archive
// carries.
func commitTime(t *testing.T, commit string) time.Time {
	t.Helper()
	out, err := exec.CommandContext(t.Context(), "git", "-C", root, "show", "-s", "--format=%cI", commit).Output()
	if err != nil {
		t.Fatal(err)
	}
	ct, err := time.Parse(time.RFC3339, strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatal(err)
	}
	return ct
}

func TestArchives(t *testing.T) {
	m, as := build(t)
	archives := ofType(as, "Archive")
	var arches []string
	for _, a := range archives {
		arches = append(arches, a.Goos+"/"+a.Goarch)
	}
	slices.Sort(arches)
	if !slices.Equal(arches, []string{"linux/amd64", "linux/arm64"}) {
		t.Fatalf("archives for %v, want linux/amd64 and linux/arm64", arches)
	}
	ct := commitTime(t, m.Commit)
	for _, a := range archives {
		t.Run(a.Goarch, func(t *testing.T) {
			if want := "nbpdns_" + m.Version + "_linux_" + a.Goarch + ".tar.gz"; a.Name != want {
				t.Errorf("archive %s, want %s", a.Name, want)
			}
			files := readArchive(t, filepath.Join(root, a.Path))
			var names []string
			for name, f := range files {
				names = append(names, name)
				// The same commit builds the same bytes, on any machine.
				if !f.hdr.ModTime.Equal(ct) || f.hdr.Uid != 0 || f.hdr.Gid != 0 || f.hdr.Uname != "root" {
					t.Errorf("%s: time %s, owner %s (%d:%d); want %s, root (0:0)", name, f.hdr.ModTime, f.hdr.Uname, f.hdr.Uid, f.hdr.Gid, ct)
				}
			}
			slices.Sort(names)
			if !slices.Equal(names, []string{"CHANGELOG.md", "LICENSE", "README.md", "nbpdns"}) {
				t.Fatalf("archive holds %v", names)
			}
			bin := files["nbpdns"]
			if bin.hdr.Mode != 0o755 || files["LICENSE"].hdr.Mode != 0o644 {
				t.Errorf("modes %o and %o, want 755 and 644", bin.hdr.Mode, files["LICENSE"].hdr.Mode)
			}
			checkStatic(t, bin.data, a.Goarch)
			if a.Goarch == runtime.GOARCH {
				checkVersion(t, bin.data, m)
			}
		})
	}
}

// A file is one entry of an archive.
type file struct {
	hdr  *tar.Header
	data []byte
}

func readArchive(t *testing.T, path string) map[string]file {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	out := map[string]file{}
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		out[hdr.Name] = file{hdr: hdr, data: data}
	}
}

// checkStatic checks that bin is a static ELF executable for goarch: one
// with no interpreter and no dynamic section, so that it needs no libc.
func checkStatic(t *testing.T, bin []byte, goarch string) {
	t.Helper()
	ef, err := elf.NewFile(strings.NewReader(string(bin)))
	if err != nil {
		t.Fatalf("not an ELF file: %v", err)
	}
	want := map[string]elf.Machine{"amd64": elf.EM_X86_64, "arm64": elf.EM_AARCH64}[goarch]
	if ef.Machine != want {
		t.Errorf("machine %s, want %s", ef.Machine, want)
	}
	for _, p := range ef.Progs {
		if p.Type == elf.PT_INTERP || p.Type == elf.PT_DYNAMIC {
			t.Errorf("the binary has a %s program header, so it isn't static", p.Type)
		}
	}
}

// checkVersion runs bin, and checks the version it reports: the tag's, for
// a tag's build, and a pseudo-version of the commit, for a snapshot.
func checkVersion(t *testing.T, bin []byte, m metadata) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nbpdns")
	if err := os.WriteFile(path, bin, 0o755); err != nil { //nolint:gosec // An executable, to run.
		t.Fatal(err)
	}
	out, err := exec.CommandContext(t.Context(), path, "version", "-o", "json").Output()
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Version  string `json:"version"`
		Commit   string `json:"commit"`
		Modified bool   `json:"modified"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatal(err)
	}
	if v.Commit != m.Commit {
		t.Errorf("commit %s, want %s", v.Commit, m.Commit)
	}
	switch {
	case m.snapshot() && !strings.HasPrefix(v.Version, "v0.0.0-") && !strings.HasPrefix(v.Version, m.Tag+"-"):
		t.Errorf("a snapshot reports %s, want a pseudo-version", v.Version)
	case !m.snapshot() && (v.Version != m.Tag || v.Modified):
		t.Errorf("a build of %s reports %s, modified %v", m.Tag, v.Version, v.Modified)
	}
}

func TestChecksums(t *testing.T) {
	_, as := build(t)
	sums := ofType(as, "Checksum")
	if len(sums) != 1 || sums[0].Name != "checksums.txt" {
		t.Fatalf("checksum files %v", sums)
	}
	f, err := os.Open(filepath.Join(root, sums[0].Path))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	listed := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		sum, name, ok := strings.Cut(sc.Text(), "  ")
		if !ok {
			t.Fatalf("checksums.txt line %q", sc.Text())
		}
		listed[name] = sum
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	for _, a := range ofType(as, "Archive") {
		b, err := os.ReadFile(filepath.Join(root, a.Path))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(b)
		if got := hex.EncodeToString(sum[:]); listed[a.Name] != got {
			t.Errorf("%s: checksums.txt says %q, the file is %s", a.Name, listed[a.Name], got)
		}
		delete(listed, a.Name)
	}
	if len(listed) != 0 {
		t.Errorf("checksums.txt lists files that aren't archives: %v", listed)
	}
}
