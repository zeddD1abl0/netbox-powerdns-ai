//go:build release

package release

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
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

// dataDir is where the image's database is, by default: database.url is
// sqlite:///var/lib/nbpdns/nbpdns.db (ADR-0038).
const dataDir = "/var/lib/nbpdns"

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

// do sends a request to the API, with body as JSON, or as a tar archive if
// it's an io.Reader, and returns the status and the body.
func (d *docker) do(method, path string, body any) (int, []byte) {
	d.t.Helper()
	var r io.Reader = http.NoBody
	contentType := "application/json"
	switch b := body.(type) {
	case nil:
	case io.Reader:
		r, contentType = b, "application/x-tar"
	default:
		j, err := json.Marshal(b)
		if err != nil {
			d.t.Fatal(err)
		}
		r = bytes.NewReader(j)
	}
	req, err := http.NewRequestWithContext(d.t.Context(), method, d.base+path, r)
	if err != nil {
		d.t.Fatal(err)
	}
	req.Header.Set("Content-Type", contentType)
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

// A container is how a test runs the image: its entrypoint and command, if
// they're set, its environment, and its mounts, as the API takes them. It
// always runs with a read-only root file system, and no network.
type container struct {
	entrypoint, cmd []string
	env             []string
	mounts          []map[string]any
}

// create creates a container from image, and returns its ID. The caller
// removes it.
func (d *docker) create(image string, c container) string {
	d.t.Helper()
	spec := map[string]any{
		"Image":      image,
		"Cmd":        c.cmd,
		"Env":        c.env,
		"HostConfig": map[string]any{"ReadonlyRootfs": true, "NetworkMode": "none", "Mounts": c.mounts},
	}
	if c.entrypoint != nil {
		spec["Entrypoint"] = c.entrypoint
	}
	code, b := d.do(http.MethodPost, "/containers/create", spec)
	if code != http.StatusCreated {
		d.t.Fatalf("creating a container: %d %s", code, b)
	}
	var created struct{ ID string }
	if err := json.Unmarshal(b, &created); err != nil {
		d.t.Fatal(err)
	}
	return created.ID
}

// remove removes the container id, and stops it first if it runs.
func (d *docker) remove(id string) {
	d.do(http.MethodDelete, "/containers/"+id+"?force=1", nil)
}

// wait waits until the container id stops, and returns its exit code.
func (d *docker) wait(id string) int {
	d.t.Helper()
	code, b := d.do(http.MethodPost, "/containers/"+id+"/wait", nil)
	var waited struct{ StatusCode int }
	if err := json.Unmarshal(b, &waited); err != nil || code != http.StatusOK {
		d.t.Fatalf("waiting for the container: %d %s", code, b)
	}
	return waited.StatusCode
}

// logs returns what the container id has written so far, to its standard
// output and to its standard error.
func (d *docker) logs(id string) (string, string) {
	d.t.Helper()
	_, b := d.do(http.MethodGet, "/containers/"+id+"/logs?stdout=1&stderr=1", nil)
	return demux(d.t, b)
}

// A run is a container that ran, and what it wrote.
type run struct {
	startErr       string
	code           int
	stdout, stderr string
}

// run creates a container from image, runs it to the end, and removes it.
func (d *docker) run(image string, c container) run {
	d.t.Helper()
	id := d.create(image, c)
	defer d.remove(id)
	if code, b := d.do(http.MethodPost, "/containers/"+id+"/start", nil); code != http.StatusNoContent {
		return run{startErr: string(b)}
	}
	code := d.wait(id)
	out, errOut := d.logs(id)
	return run{code: code, stdout: out, stderr: errOut}
}

// waitLog waits until the container id, which runs, writes want, and fails
// if it stops first, or doesn't within 30 seconds.
func (d *docker) waitLog(id, want string) {
	d.t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		out, errOut := d.logs(id)
		if strings.Contains(out+errOut, want) {
			return
		}
		code, b := d.do(http.MethodGet, "/containers/"+id+"/json", nil)
		var inspect struct{ State struct{ Running bool } }
		if err := json.Unmarshal(b, &inspect); err != nil || code != http.StatusOK {
			d.t.Fatalf("inspecting the container: %d %s", code, b)
		}
		if !inspect.State.Running || time.Now().After(deadline) {
			d.t.Fatalf("the container wrote no %s, running %v:\n%s%s", want, inspect.State.Running, out, errOut)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// volume creates a named volume, and returns its name and its directory on
// Docker's host. The caller removes it.
func (d *docker) volume() (string, string) {
	d.t.Helper()
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		d.t.Fatal(err)
	}
	code, body := d.do(http.MethodPost, "/volumes/create", map[string]any{"Name": "nbpdns-release-check-" + hex.EncodeToString(b)})
	var v struct{ Name, Mountpoint string }
	if err := json.Unmarshal(body, &v); err != nil || code != http.StatusCreated || v.Mountpoint == "" {
		d.t.Fatalf("creating a volume: %d %s", code, body)
	}
	return v.Name, v.Mountpoint
}

// removeVolume removes the volume name.
func (d *docker) removeVolume(name string) {
	d.do(http.MethodDelete, "/volumes/"+name+"?force=1", nil)
}

// mkdirs makes the directories dirs in the volume vol, each owned by
// 65532:65532 with mode 0700, as `install -d -o 65532 -g 65532 -m 700`
// would on Docker's host. They're a tar archive's, extracted by the API
// through a container from image that never starts, so making them needs
// no other image, and no root on this host. The API sets the owner of the
// directories that it makes, but never of the directory it extracts into,
// so it can't change the owner of the volume itself.
func (d *docker) mkdirs(image, vol string, dirs ...string) {
	d.t.Helper()
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	for _, dir := range dirs {
		if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: dir + "/", Mode: 0o700, Uid: 65532, Gid: 65532,
			ModTime: time.Now()}); err != nil {
			d.t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		d.t.Fatal(err)
	}
	id := d.create(image, container{mounts: []map[string]any{{"Type": "volume", "Source": vol, "Target": dataDir}}})
	defer d.remove(id)
	if code, body := d.do(http.MethodPut, "/containers/"+id+"/archive?path="+url.QueryEscape(dataDir), &b); code != http.StatusOK {
		d.t.Fatalf("making %v in the volume: %d %s", dirs, code, body)
	}
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

	// Each subtest has its own client, so that a failure is its own.
	t.Run("version", func(t *testing.T) {
		r := newDocker(t).run(image, container{cmd: []string{"version", "-o", "json"}})
		var v struct{ Commit string }
		if err := json.Unmarshal([]byte(r.stdout), &v); err != nil || r.code != 0 || v.Commit != m.Commit {
			t.Errorf("version: exit %d, commit %q, %v:\n%s%s", r.code, v.Commit, err, r.stdout, r.stderr)
		}
	})
	t.Run("serve without a volume", func(t *testing.T) {
		r := newDocker(t).run(image, container{cmd: []string{"serve"}})
		want := "the database's directory, " + dataDir + ", doesn't exist; create it, or mount a volume there, writable by user 65532"
		if r.code != 1 || !strings.Contains(r.stderr, want) {
			t.Errorf("serve: exit %d, want 1 with %q:\n%s%s", r.code, want, r.stdout, r.stderr)
		}
	})
	// The database in a volume, as "Run nbpdns in a container" mounts one:
	// a named volume, or a bind mount, of a directory owned by 65532. Both
	// directories are in one named volume: the first is mounted as its
	// subpath, and the second by its path on Docker's host, which in CI is
	// the Docker-in-Docker service's, where no directory of the job is.
	t.Run("serve on a volume", func(t *testing.T) {
		d := newDocker(t)
		vol, host := d.volume()
		defer d.removeVolume(vol)
		d.mkdirs(image, vol, "named", "bind")
		for _, tt := range []struct {
			name  string
			mount map[string]any
		}{
			{"a named volume", map[string]any{"Type": "volume", "Source": vol, "Target": dataDir, "VolumeOptions": map[string]any{"Subpath": "named"}}},
			{"a bind mount", map[string]any{"Type": "bind", "Source": path.Join(host, "bind"), "Target": dataDir}},
		} {
			t.Run(tt.name, func(t *testing.T) {
				d := newDocker(t)
				// NetBox can't be reached, and there are no groups to read.
				serve := container{cmd: []string{"serve"}, mounts: []map[string]any{tt.mount},
					env: []string{"NBPDNS_NETBOX_URL=http://127.0.0.1:1", "NBPDNS_NETBOX_TOKEN=nbt_abc.def", "NBPDNS_LOG_FORMAT=json"}}
				id := d.create(image, serve)
				defer d.remove(id)
				if code, b := d.do(http.MethodPost, "/containers/"+id+"/start", nil); code != http.StatusNoContent {
					t.Fatalf("starting serve: %d %s", code, b)
				}
				d.waitLog(id, `"msg":"serving"`)
				// The lock holds across containers.
				if r := d.run(image, serve); r.code != 1 || !strings.Contains(r.stderr, "another nbpdns holds the database's lock") {
					t.Errorf("a second serve: exit %d:\n%s%s", r.code, r.stdout, r.stderr)
				}
				if code, b := d.do(http.MethodPost, "/containers/"+id+"/stop?t=30", nil); code != http.StatusNoContent {
					t.Fatalf("stopping serve: %d %s", code, b)
				}
				code := d.wait(id)
				out, errOut := d.logs(id)
				if want := `"path":"` + dataDir + `/nbpdns.db"`; code != 0 || !strings.Contains(errOut, want) {
					t.Errorf("serve: exit %d, want 0, with %s:\n%s%s", code, want, out, errOut)
				}
				// The database, with serve's start and stop, outlives it.
				r := d.run(image, container{cmd: []string{"audit", "list", "-o", "json"}, mounts: []map[string]any{tt.mount}})
				var events []struct{ Action string }
				if err := json.Unmarshal([]byte(r.stdout), &events); err != nil || r.code != 0 || len(events) != 2 ||
					events[0].Action != "service.started" || events[1].Action != "service.stopped" {
					t.Errorf("audit list: exit %d, %v, want serve's start and stop:\n%s%s", r.code, err, r.stdout, r.stderr)
				}
			})
		}
	})
	t.Run("no shell", func(t *testing.T) {
		if r := newDocker(t).run(image, container{entrypoint: []string{"/bin/sh"}, cmd: []string{"-c", "true"}}); r.startErr == "" {
			t.Errorf("a shell ran, exit %d", r.code)
		}
	})
}
