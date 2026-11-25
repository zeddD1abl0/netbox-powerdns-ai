//go:build integration

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/lab"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/service"
)

// replicaEnv, set in its environment, has the test binary run as nbpdns,
// with its arguments, rather than run the tests. The takeover tests start
// their replicas of serve so, as child processes, which can be killed, or
// cut off from the database with their sockets left open, as serve in a
// goroutine can't.
const replicaEnv = "CLI_TEST_REPLICA"

// TestMain runs the package's tests, or, with replicaEnv set, nbpdns, as its
// main does.
func TestMain(m *testing.M) {
	if os.Getenv(replicaEnv) == "" {
		os.Exit(m.Run())
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := Main(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// TestTakeovers checks how the replicas of serve choose their leader, and
// share their state, as ADR-0047 and ADR-0048 have it. Three replicas run as
// child processes, on the lab's NetBox and PowerDNS, and on a database of
// their own, each through a proxy of its own. Exactly one of them refreshes.
// A setting changed with the command line, and a webhook sent to a
// follower, reach every replica. A leader stopped with SIGTERM, or killed
// with SIGKILL, is replaced within 5 seconds. A leader cut off from the
// database steps down before another replica takes over, within 40 seconds.
//
// With database.listen on, the setting, the webhook and the stopped leader
// are checked again, with the longest poll, 30 seconds, so that only the
// database's notifications can bring their changes within seconds.
func TestTakeovers(t *testing.T) {
	tests := []struct {
		name   string
		listen bool
		// poll is database.poll_interval.
		poll string
		// all is whether to check every case, or only those that
		// database.listen changes: the setting, the webhook and the stopped
		// leader.
		all bool
	}{
		{name: "polling", poll: "1s", all: true},
		{name: "listening", listen: true, poll: "30s"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			nb, p := lab.NetBoxes[0], lab.PowerDNSes[0]
			f := lab.NewDriftFixture(t, nb, p)
			_, keyPath := keyFile(t)
			env := with(configFile(t, "powerdns:\n  groups:\n"+labGroup(f, p.Group, p.URL())),
				"NBPDNS_NETBOX_URL", nb.URL(), "NBPDNS_NETBOX_TOKEN", f.ReaderToken,
				"NBPDNS_DATABASE_URL", lab.NewPostgresDatabase(t), "NBPDNS_DATABASE_MASTER_KEY_FILE", keyPath,
				"NBPDNS_DATABASE_POLL_INTERVAL", tc.poll, "NBPDNS_DATABASE_LISTEN", strconv.FormatBool(tc.listen),
				"NBPDNS_NETBOX_WEBHOOK_SECRET", webhookSecret, "NBPDNS_DRIFT_WEBHOOK_DELAY", "200ms")

			// Of three replicas that start at once, exactly one leads and
			// refreshes, and the others serve its refresh.
			rs := startReplicas(t, env, "A", "B", "C")
			leader, followers := elected(t, rs, 2*time.Minute)

			// A setting changed with the command line reaches every replica.
			if code, _, stderr := run(t, env, "settings", "set", "drift.interval", "2m", "--reason", "a test"); code != exitOK {
				t.Fatalf("settings set: exit %d:\n%s", code, stderr)
			}
			for _, r := range rs {
				r.statusWhen(t, func(st service.Status) bool { return st.Schedule.IntervalSeconds == 120 })
			}

			// A record changes in NetBox, and its webhook reaches a follower:
			// the leader refreshes its zone, which every replica then serves.
			if code, body := followers[0].postEvent(inSyncChanged(t, nb, f, nil), webhookSecret); code != http.StatusAccepted {
				t.Fatalf("the event to the follower %s: %d %s", followers[0].name, code, body)
			}
			const zoneRefreshes = `nbpdns_drift_zone_refreshes_total{outcome="complete"}`
			eventually(t, 15*time.Second, "the leader's refresh of the follower's webhook", func() bool {
				return metric(t, leader.served, zoneRefreshes) == 1
			})
			for _, r := range rs {
				eventually(t, 15*time.Second, r.name+"'s serving the zone's change", func() bool {
					state, changes := zoneState(t, r.served, p.Group, f.InSync)
					return state == "drift" && changes == 1
				})
			}
			for _, r := range followers {
				if n := metric(t, r.served, zoneRefreshes); n != 0 {
					t.Errorf("the follower %s ran %v zone refreshes, want none", r.name, n)
				}
			}

			// The leader stops, and releases its lock, which a follower takes.
			stopped := time.Now()
			leader.cancel()
			takeover(t, leader, followers, stopped, 5*time.Second)
			if code := leader.stop(); code != exitOK {
				t.Errorf("the stopped leader %s exited %d:\n%s", leader.name, code, leader.stderr)
			}
			if !tc.all {
				return
			}

			// The new leader is killed: PostgreSQL ends its session, and frees
			// its lock, as its connection closes.
			leader, followers = elected(t, append(followers, startReplicas(t, env, "D")...), 15*time.Second)
			killed := time.Now()
			leader.kill()
			takeover(t, leader, followers, killed, 5*time.Second)

			// The next is cut off. It steps down once a heartbeat misses its
			// deadline, within 10 seconds, and PostgreSQL ends its session,
			// and frees its lock, once the session has been idle for 30.
			leader, followers = elected(t, append(followers, startReplicas(t, env, "E")...), 15*time.Second)
			cut := time.Now()
			leader.proxy.cut()
			took := takeover(t, leader, followers, cut, 40*time.Second)
			down := leader.logged("the leader missed its heartbeat, so this replica steps down")
			t.Logf("the cut-off leader %s stepped down %s after the cut", leader.name, down.Time.Sub(cut).Round(time.Millisecond))
			if !down.Time.Before(took) {
				t.Errorf("the cut-off leader %s stepped down at %s, not before another took over, at %s", leader.name, down.Time, took)
			}
			if v := metric(t, leader.served, "nbpdns_leader"); v != 0 {
				t.Errorf("the cut-off leader %s: nbpdns_leader %v, want 0", leader.name, v)
			}
		})
	}
}

// elected waits, for up to within, until exactly one of rs leads, and has
// refreshed, and the others follow it, and serve its last refresh. None of
// rs has led before, so it checks that none of the followers has refreshed.
// It returns the leader, and the followers.
func elected(t *testing.T, rs []*replica, within time.Duration) (*replica, []*replica) {
	t.Helper()
	deadline := time.Now().Add(within)
	for {
		sts := make([]service.Status, len(rs))
		var leader *replica
		var ls service.Status
		leaders := 0
		for i, r := range rs {
			sts[i] = r.status()
			r.id = sts[i].Replica.ID
			if sts[i].Replica.Role == service.RoleLeader {
				leader, ls, leaders = r, sts[i], leaders+1
			}
		}
		done := leaders == 1 && ls.Schedule.LastRefresh != nil && refreshes(ls) > 0
		var followers []*replica
		for i, r := range rs {
			if r == leader {
				continue
			}
			st := sts[i]
			done = done && st.Replica.Role == service.RoleFollower && st.Leader != nil && st.Leader.ID == ls.Replica.ID &&
				st.Schedule.LastRefresh != nil && st.Schedule.LastRefresh.Finished.Equal(ls.Schedule.LastRefresh.Finished)
			followers = append(followers, r)
		}
		if done {
			for i, r := range rs {
				if r != leader && refreshes(sts[i]) != 0 {
					t.Errorf("the follower %s refreshed: %+v", r.name, sts[i].Schedule.Refreshes)
				}
			}
			return leader, followers
		}
		if time.Now().After(deadline) {
			var b strings.Builder
			for i, r := range rs {
				fmt.Fprintf(&b, "%s: replica %+v, leader %+v, last refresh %+v, refreshes %+v\n", r.name, sts[i].Replica,
					sts[i].Leader, sts[i].Schedule.LastRefresh, sts[i].Schedule.Refreshes)
			}
			t.Fatalf("the replicas didn't elect one leader, which refreshed, within %s:\n%s%s", within, &b, logs(rs))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// refreshes returns how many refreshes the replica whose status is st ran.
func refreshes(st service.Status) int {
	r := st.Schedule.Refreshes
	return r.Complete + r.Incomplete + r.Failed
}

// takeover waits until exactly one of rs, the followers of from, leads in
// its place, for up to within of since, when from stopped, died or was cut
// off. It returns when the new leader took over, as its log has it, and
// checks that its takeover names from as the leader before.
func takeover(t *testing.T, from *replica, rs []*replica, since time.Time, within time.Duration) time.Time {
	t.Helper()
	for {
		var leading []*replica
		for _, r := range rs {
			if r.status().Replica.Role == service.RoleLeader {
				leading = append(leading, r)
			}
		}
		took := time.Since(since)
		switch {
		case len(leading) > 1:
			t.Fatalf("%s and %s lead at once:\n%s", leading[0].name, leading[1].name, logs(rs))
		case len(leading) == 1:
			r := leading[0]
			t.Logf("%s took over from %s within %s", r.name, from.name, took.Round(time.Millisecond))
			if took > within {
				t.Errorf("%s took over from %s after %s, want within %s", r.name, from.name, took, within)
			}
			rec := r.logged("took over as the leader")
			if rec.PreviousInstanceID != from.id {
				t.Errorf("%s took over from the instance %q, want %s's, %q", r.name, rec.PreviousInstanceID, from.name, from.id)
			}
			return rec.Time
		case took > within:
			t.Fatalf("no replica took over from %s within %s:\n%s", from.name, within, logs(append(rs, from)))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// logs returns the logs of rs, each under its name.
func logs(rs []*replica) string {
	var b strings.Builder
	for _, r := range rs {
		fmt.Fprintf(&b, "--- %s's log:\n%s", r.name, r.stderr)
	}
	return b.String()
}

// A replica is `nbpdns serve`, run as a child process, the test binary
// running itself as nbpdns, on its database through a proxy of its own.
type replica struct {
	*served
	// name names it in the test's messages, and id is its instance ID, once
	// elected has read its status.
	name, id string
	cmd      *exec.Cmd
	proxy    *proxy
}

// startReplicas starts a replica of serve with env for each of names, all at
// once, each on env's database through a proxy of its own, and returns them
// once each listens. Each stops as the test ends.
func startReplicas(t *testing.T, env map[string]string, names ...string) []*replica {
	t.Helper()
	var rs []*replica
	for _, name := range names {
		rs = append(rs, startReplica(t, name, env))
	}
	for _, r := range rs {
		r.listening()
	}
	return rs
}

// startReplica starts the replica name, without waiting for it to listen.
func startReplica(t *testing.T, name string, env map[string]string) *replica {
	t.Helper()
	db, err := url.Parse(env["NBPDNS_DATABASE_URL"])
	if err != nil {
		t.Fatal(err)
	}
	p := newProxy(t, db.Host)
	db.Host = p.addr()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// The replica outlives the test's context, which ends before end stops
	// it.
	cmd := exec.CommandContext(context.WithoutCancel(t.Context()), exe, "serve")
	// The replica's environment is the test's, but for nbpdns's own
	// variables, which are env's.
	cmd.Env = []string{replicaEnv + "=1"}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, config.EnvPrefix) {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	for k, v := range with(env, "NBPDNS_DATABASE_URL", db.String(), "NBPDNS_SERVER_LISTEN", "127.0.0.1:0", "NBPDNS_LOG_FORMAT", "json") {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	s := &served{t: t, stderr: &lockedBuffer{}, done: make(chan int, 1)}
	cmd.Stdout, cmd.Stderr = s.stderr, s.stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// Once only, as a context's cancel does: once serve has stopped, a second
	// SIGTERM, before the process exits, would end it with the signal.
	s.cancel = sync.OnceFunc(func() { _ = cmd.Process.Signal(syscall.SIGTERM) })
	go func() {
		_ = cmd.Wait()
		s.done <- cmd.ProcessState.ExitCode()
	}()
	r := &replica{served: s, name: name, cmd: cmd, proxy: p}
	t.Cleanup(r.end)
	return r
}

// status returns the replica's /status.
func (r *replica) status() service.Status {
	r.t.Helper()
	return r.statusWhen(r.t, func(service.Status) bool { return true })
}

// kill kills the replica with SIGKILL, and waits for it to end.
func (r *replica) kill() {
	r.t.Helper()
	if err := r.cmd.Process.Kill(); err != nil {
		r.t.Fatal(err)
	}
	code := <-r.done
	r.done <- code
}

// end stops the replica as the test ends, if it still runs: with SIGTERM,
// once its proxy has closed if it's cut off, so that it fails at once to
// reach the database, or with SIGKILL if it hasn't stopped within 30
// seconds. It fails the test if the race detector found a data race in it.
func (r *replica) end() {
	select {
	case code := <-r.done:
		r.done <- code
	default:
		if r.proxy.isCut() {
			r.proxy.close()
		}
		r.cancel()
		select {
		case code := <-r.done:
			r.done <- code
		case <-time.After(30 * time.Second):
			r.t.Errorf("%s didn't stop within 30 seconds of SIGTERM, so it's killed:\n%s", r.name, r.stderr)
			_ = r.cmd.Process.Kill()
			<-r.done
		}
	}
	if strings.Contains(r.stderr.String(), "WARNING: DATA RACE") {
		r.t.Errorf("the race detector found a data race in %s:\n%s", r.name, r.stderr)
	}
}

// A record is one of a replica's log records, with the attributes that the
// takeover tests read.
type record struct {
	Time               time.Time `json:"time"`
	Msg                string    `json:"msg"`
	PreviousInstanceID string    `json:"previous_instance_id"`
}

// logged returns the last record that the replica logged whose message
// starts with msg. It waits up to a second for one, which may still be on
// its way from the replica.
func (r *replica) logged(msg string) record {
	r.t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		var found *record
		for l := range strings.Lines(r.stderr.String()) {
			var rec record
			if json.Unmarshal([]byte(l), &rec) == nil && strings.HasPrefix(rec.Msg, msg) {
				found = &rec
			}
		}
		if found != nil {
			return *found
		}
		if time.Now().After(deadline) {
			r.t.Fatalf("%s didn't log %q:\n%s", r.name, msg, r.stderr)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A proxy forwards a replica's connections to its database until it's cut
// off. Then it forwards nothing more, either way, and answers no new
// connection, but keeps every connection open, as a hung host's kernel
// does, so that PostgreSQL ends the replica's sessions on its own timeouts
// alone (ADR-0047).
type proxy struct {
	ln     net.Listener
	target string
	// ctx ends as the proxy closes.
	ctx                context.Context
	cancel             context.CancelFunc
	cutOff             chan struct{}
	cutOnce, closeOnce sync.Once
	// mu guards conns, the connections that the proxy holds, both ways.
	mu    sync.Mutex
	conns []net.Conn
	wg    sync.WaitGroup
}

// newProxy starts a proxy to target, a host and port, on a free port of
// 127.0.0.1. It closes as the test ends.
func newProxy(t *testing.T, target string) *proxy {
	t.Helper()
	ctx, cancel := context.WithCancel(context.WithoutCancel(t.Context()))
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	p := &proxy{ln: ln, target: target, ctx: ctx, cancel: cancel, cutOff: make(chan struct{})}
	p.wg.Go(p.accept)
	t.Cleanup(p.close)
	return p
}

// addr returns the proxy's address.
func (p *proxy) addr() string { return p.ln.Addr().String() }

// cut cuts the proxy off.
func (p *proxy) cut() { p.cutOnce.Do(func() { close(p.cutOff) }) }

// isCut reports whether the proxy is cut off.
func (p *proxy) isCut() bool {
	select {
	case <-p.cutOff:
		return true
	default:
		return false
	}
}

// close closes the proxy, and every connection that it holds.
func (p *proxy) close() {
	p.closeOnce.Do(func() {
		p.cancel()
		_ = p.ln.Close()
		p.mu.Lock()
		for _, c := range p.conns {
			_ = c.Close()
		}
		p.conns = nil
		p.mu.Unlock()
		p.wg.Wait()
	})
}

// keep holds c, for close to close, and reports whether it does: if the
// proxy has closed, it closes c instead.
func (p *proxy) keep(c net.Conn) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ctx.Err() != nil {
		_ = c.Close()
		return false
	}
	p.conns = append(p.conns, c)
	return true
}

// accept accepts connections until the proxy closes, and forwards each,
// unless the proxy is cut off: then it holds them, unanswered.
func (p *proxy) accept() {
	for {
		c, err := p.ln.Accept()
		if err != nil {
			return
		}
		if p.keep(c) && !p.isCut() {
			p.wg.Go(func() { p.forward(c) })
		}
	}
}

// forward connects c to the target, and copies between them, both ways.
func (p *proxy) forward(c net.Conn) {
	var d net.Dialer
	up, err := d.DialContext(p.ctx, "tcp", p.target)
	if err != nil || !p.keep(up) {
		_ = c.Close()
		return
	}
	p.wg.Go(func() { p.pipe(up, c) })
	p.pipe(c, up)
}

// pipe copies from src to dst until either fails, then closes both, so that
// a close at one end reaches the other. Once the proxy is cut off, it
// forwards nothing more, and holds both open until the proxy closes.
func (p *proxy) pipe(dst, src net.Conn) {
	buf := make([]byte, 32<<10)
	for {
		n, err := src.Read(buf)
		if p.isCut() {
			<-p.ctx.Done()
			return
		}
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				err = werr
			}
		}
		if err != nil {
			_ = src.Close()
			_ = dst.Close()
			return
		}
	}
}
