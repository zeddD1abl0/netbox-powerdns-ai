package cli

import (
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/metrics"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/service"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/version"
)

// TestFeedCheck checks that each of the feed's checks reaches the election,
// with the leader's row if it could read it, and without if it couldn't,
// and that the election gets the latest check, not one that it didn't take.
func TestFeedCheck(t *testing.T) {
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "nbpdns.db"), store.Options{Create: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Migrate(t.Context(), "test"); err != nil {
		t.Fatal(err)
	}
	s := testSession(t)
	logs := &lockedBuffer{}
	s.log = slog.New(slog.NewJSONHandler(logs, nil))
	svc := service.New(service.Options{Log: s.log, Metrics: metrics.New(version.Get())})
	f := &feed{s: s, st: st, w: &watcher{s: s, st: st, svc: svc}, polls: make(chan poll, 1), wake: make(chan struct{}, 1)}

	f.check(t.Context())
	f.check(t.Context())
	if p := <-f.polls; !p.read || p.leader != nil {
		t.Errorf("the check of a new database = %+v, want one read, with no leader", p)
	}
	select {
	case p := <-f.polls:
		t.Errorf("a second check waits for the election: %+v", p)
	default:
	}

	// A check that can't read the versions reaches the election all the
	// same.
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	f.check(t.Context())
	if p := <-f.polls; p.read {
		t.Errorf("the check of a closed database = %+v, want one that didn't read", p)
	}
	if !strings.Contains(logs.String(), "couldn't check the database for changes") {
		t.Errorf("no warning of the failed check:\n%s", logs)
	}
}

// TestFeedPoke checks that pokes that come before the feed checks are
// one check.
func TestFeedPoke(t *testing.T) {
	f := &feed{wake: make(chan struct{}, 1)}
	f.poke()
	f.poke()
	<-f.wake
	select {
	case <-f.wake:
		t.Error("two pokes woke the feed twice")
	default:
	}
}
