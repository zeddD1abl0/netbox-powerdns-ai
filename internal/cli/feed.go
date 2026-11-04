package cli

import (
	"context"
	"errors"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store"
)

// checkTimeout bounds each of the feed's reads of the versions, so that a
// database that hangs doesn't hold up the election's next try.
const checkTimeout = 10 * time.Second

// listenCheck is how long the listener waits for a notification before it
// checks that its connection still works, and listenTimeout how long it
// lets that check, or listening, take.
const (
	listenCheck   = 30 * time.Second
	listenTimeout = 10 * time.Second
)

// A feed is the replicas' change feed (ADR-0048). It checks the versions
// that every replica follows as serve starts, then every
// database.poll_interval, and at once whenever the listener, with
// database.listen, hears of a change. It hands each check to the election,
// whose followers try for the leader's lock at each, and then to the
// watcher, which reloads the settings and the groups that moved.
type feed struct {
	s        *session
	st       *store.Store
	interval time.Duration
	w        *watcher
	// polls are the checks for the election, or nil on SQLite, where serve
	// always leads.
	polls chan poll
	// wake has the feed check at once.
	wake chan struct{}
}

// A poll is one of the feed's checks, as the election takes it.
type poll struct {
	// read is whether the check read the versions, and leader is the
	// leader's row that it read, or nil if no replica has led the database.
	read   bool
	leader *store.Leader
}

// newFeed returns the change feed of st, for w and, on PostgreSQL, the
// election, which it returns the polls of.
func newFeed(s *session, st *store.Store, w *watcher, elected bool) *feed {
	f := &feed{s: s, st: st, interval: s.cfg.Database.PollInterval, w: w, wake: make(chan struct{}, 1)}
	if elected {
		f.polls = make(chan poll, 1)
	}
	return f
}

// run checks the database for changes, at once, then every interval, or
// when woken, until ctx is done.
func (f *feed) run(ctx context.Context) {
	t := time.NewTicker(f.interval)
	defer t.Stop()
	for {
		f.check(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-f.wake:
		}
	}
}

// check reads the versions, and hands them to the election, even if it
// couldn't read them, since a follower tries for the lock all the same, and
// then to the watcher.
func (f *feed) check(ctx context.Context) {
	cctx, cancel := context.WithTimeout(ctx, checkTimeout)
	v, err := f.st.Versions(cctx)
	cancel()
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		f.s.log.WarnContext(ctx, "couldn't check the database for changes; serve checks again at its next poll", "err", err)
	}
	if f.polls != nil {
		f.send(poll{read: err == nil, leader: v.Leader})
	}
	if err == nil {
		f.w.apply(ctx, v)
	}
}

// send hands p to the election, in place of a check that it hasn't taken
// yet, such as one from while it led, so that it never waits for the
// election, nor the election for a stale check.
func (f *feed) send(p poll) {
	for {
		select {
		case f.polls <- p:
			return
		default:
		}
		select {
		case <-f.polls:
		default:
		}
	}
}

// poke has the feed check at once, unless it's about to already.
func (f *feed) poke() {
	select {
	case f.wake <- struct{}{}:
	default:
	}
}

// listen listens for the database's changes until ctx is done, and has the
// feed check at once at each (ADR-0048). Once it listens, it has the feed
// check too, for any change that it missed before. If it can't listen, or
// its connection is lost, it tries again after an interval: the feed's
// checks go on meanwhile.
func (f *feed) listen(ctx context.Context) {
	lost := false
	for {
		lctx, cancel := context.WithTimeout(ctx, listenTimeout)
		l, err := f.st.Listen(lctx)
		cancel()
		switch {
		case ctx.Err() != nil:
			if l != nil {
				l.Close()
			}
			return
		case err != nil:
			f.s.log.WarnContext(ctx, "couldn't listen for the database's changes, so serve finds them at its polls alone; "+
				"it tries again at its next poll", "err", err)
		default:
			if lost {
				f.s.log.InfoContext(ctx, "listening for the database's changes again")
			} else {
				f.s.log.InfoContext(ctx, "listening for the database's changes")
			}
			f.poke()
			err = f.hear(ctx, l)
			l.Close()
			if ctx.Err() != nil {
				return
			}
			lost = true
			f.s.log.WarnContext(ctx, "lost the connection that listens for the database's changes, so serve finds them at its polls alone; "+
				"it listens again at its next poll", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(f.interval):
		}
	}
}

// hear has the feed check at each notification on l, until ctx is done, or
// l's connection is lost, which it returns why. A connection can be lost
// without a word, so it checks that it still works whenever it has heard
// nothing for listenCheck.
func (f *feed) hear(ctx context.Context, l *store.Listener) error {
	for {
		wctx, cancel := context.WithTimeout(ctx, listenCheck)
		err := l.Wait(wctx)
		cancel()
		switch {
		case ctx.Err() != nil:
			return ctx.Err()
		case err == nil:
			f.poke()
		case errors.Is(err, context.DeadlineExceeded):
			cctx, cancel := context.WithTimeout(ctx, listenTimeout)
			err := l.Check(cctx)
			cancel()
			if err != nil {
				return err
			}
		default:
			return err
		}
	}
}
