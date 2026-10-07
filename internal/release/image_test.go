//go:build release

package release

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"testing"
	"time"
)

// localImage is where ko loads a snapshot's image, in the Docker daemon.
const localImage = "goreleaser.ko.local"

// A docker client talks to the Docker Engine API at DOCKER_HOST, a tcp://
// address or a unix:// socket, with the standard library only.
type docker struct {
	t    *testing.T
	c    *http.Client
	base string
}

func newDocker(t *testing.T) *docker {
	t.Helper()
	host := os.Getenv("DOCKER_HOST")
	if host == "" {
		host = "unix:///var/run/docker.sock"
	}
	u, err := url.Parse(host)
	if err != nil {
		t.Fatalf("DOCKER_HOST %q: %v", host, err)
	}
	d := &docker{t: t, c: &http.Client{Timeout: time.Minute}}
	switch u.Scheme {
	case "unix":
		sock := u.Path
		d.c.Transport = &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", sock)
		}}
		d.base = "http://docker"
	case "tcp":
		d.base = "http://" + u.Host
	default:
		t.Fatalf("DOCKER_HOST %q: want tcp:// or unix://", host)
	}
	return d
}

// do sends a request to the API, and returns the status and the body.
func (d *docker) do(method, path string, body any) (int, []byte) {
	d.t.Helper()
	var r io.Reader = http.NoBody
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			d.t.Fatal(err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(d.t.Context(), method, d.base+path, r)
	if err != nil {
		d.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.c.Do(req)
	if err != nil {
		d.t.Fatalf("%s %s: %v (is Docker at DOCKER_HOST?)", method, path, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		d.t.Fatal(err)
	}
	return resp.StatusCode, b
}

// A run is a container that ran, and what it wrote.
type run struct {
	startErr       string
	code           int
	stdout, stderr string
}

// run creates a container from image, with entrypoint and cmd if they're
// set, and a read-only root file system, runs it to the end, and removes
// it.
func (d *docker) run(image string, entrypoint, cmd []string) run {
	d.t.Helper()
	spec := map[string]any{
		"Image":      image,
		"Cmd":        cmd,
		"HostConfig": map[string]any{"ReadonlyRootfs": true, "NetworkMode": "none"},
	}
	if entrypoint != nil {
		spec["Entrypoint"] = entrypoint
	}
	code, b := d.do(http.MethodPost, "/containers/create", spec)
	if code != http.StatusCreated {
		d.t.Fatalf("creating a container: %d %s", code, b)
	}
	var created struct{ ID string }
	if err := json.Unmarshal(b, &created); err != nil {
		d.t.Fatal(err)
	}
	defer d.do(http.MethodDelete, "/containers/"+created.ID+"?force=1", nil)
	if code, b := d.do(http.MethodPost, "/containers/"+created.ID+"/start", nil); code != http.StatusNoContent {
		return run{startErr: string(b)}
	}
	code, b = d.do(http.MethodPost, "/containers/"+created.ID+"/wait", nil)
	var waited struct{ StatusCode int }
	if err := json.Unmarshal(b, &waited); err != nil || code != http.StatusOK {
		d.t.Fatalf("waiting for the container: %d %s", code, b)
	}
	_, logs := d.do(http.MethodGet, "/containers/"+created.ID+"/logs?stdout=1&stderr=1", nil)
	out, errOut := demux(d.t, logs)
	return run{code: waited.StatusCode, stdout: out, stderr: errOut}
}

// demux splits a container's logs, which the API frames with an 8-byte
// header saying which stream each part is from and how long it is.
func demux(t *testing.T, b []byte) (string, string) {
	t.Helper()
	var out, errOut strings.Builder
	for len(b) >= 8 {
		n := int(binary.BigEndian.Uint32(b[4:8]))
		if len(b) < 8+n {
			t.Fatalf("a log frame of %d bytes with %d left", n, len(b)-8)
		}
		switch b[0] {
		case 1:
			out.Write(b[8 : 8+n])
		case 2:
			errOut.Write(b[8 : 8+n])
		}
		b = b[8+n:]
	}
	return out.String(), errOut.String()
}

func TestImage(t *testing.T) {
	m, _ := build(t)
	if !m.snapshot() {
		t.Skip("a tag's build pushes its image to the registry, not the daemon")
	}
	d := newDocker(t)
	image := localImage + ":" + m.Version

	code, b := d.do(http.MethodGet, "/images/"+url.PathEscape(image)+"/json", nil)
	if code != http.StatusOK {
		t.Fatalf("inspecting %s: %d %s (run make release-check)", image, code, b)
	}
	var inspect struct {
		Os, Architecture string
		Config           struct {
			User       string
			Entrypoint []string
			Labels     map[string]string
		}
	}
	if err := json.Unmarshal(b, &inspect); err != nil {
		t.Fatal(err)
	}
	cfg := inspect.Config
	if inspect.Os != "linux" || cfg.User != "65532:65532" || len(cfg.Entrypoint) != 1 || path.Base(cfg.Entrypoint[0]) != "nbpdns" {
		t.Errorf("image %s/%s, user %q, entrypoint %v", inspect.Os, inspect.Architecture, cfg.User, cfg.Entrypoint)
	}
	for k, want := range map[string]string{
		"org.opencontainers.image.title":    "nbpdns",
		"org.opencontainers.image.version":  m.Version,
		"org.opencontainers.image.revision": m.Commit,
		"org.opencontainers.image.licenses": "Apache-2.0",
		"org.opencontainers.image.source":   "https://github.com/zeddD1abl0/netbox-powerdns-ai",
	} {
		if cfg.Labels[k] != want {
			t.Errorf("label %s = %q, want %q", k, cfg.Labels[k], want)
		}
	}
	if cfg.Labels["org.opencontainers.image.created"] == "" {
		t.Error("no created label")
	}

	t.Run("version", func(t *testing.T) {
		r := d.run(image, nil, []string{"version", "-o", "json"})
		var v struct{ Commit string }
		if err := json.Unmarshal([]byte(r.stdout), &v); err != nil || r.code != 0 || v.Commit != m.Commit {
			t.Errorf("version: exit %d, commit %q, %v:\n%s%s", r.code, v.Commit, err, r.stdout, r.stderr)
		}
	})
	t.Run("serve without groups", func(t *testing.T) {
		r := d.run(image, nil, []string{"serve"})
		if r.code != 1 || !strings.Contains(r.stderr, "no PowerDNS server groups are declared") {
			t.Errorf("serve: exit %d:\n%s%s", r.code, r.stdout, r.stderr)
		}
	})
	t.Run("no shell", func(t *testing.T) {
		if r := d.run(image, []string{"/bin/sh"}, []string{"-c", "true"}); r.startErr == "" {
			t.Errorf("a shell ran, exit %d", r.code)
		}
	})
}
