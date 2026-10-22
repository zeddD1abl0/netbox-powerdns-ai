package cli

import (
	"context"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/service"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store"
)

// The election's timing (ADR-0047). They're variables so that tests can
// shorten them.
var (
	// leaderPoll is how often a follower tries for the leader's lock, and
	// reads the leader's row.
	leaderPoll = 5 * time.Second
	// heartbeatInterval is how often the leader confirms that it leads, and
	// heartbeatTimeout how long it waits for that before it steps down: it
	// steps down within 10 seconds of its last heartbeat, before PostgreSQL
	// can end a lost leader's session, about 25 seconds, and free its lock.
	heartbeatInterval = 5 * time.Second
	heartbeatTimeout  = 5 * time.Second
)

// electionTimeout bounds each of a follower's steps: joining, trying for the
// lock, which may write the takeover, and reading the leader's row.
const electionTimeout = 10 * time.Second

// An election is serve's part in choosing the leader of the replicas on a
// PostgreSQL database (ADR-0047). As a follower, it tries for the leader's
// lock at each poll. As the leader, it runs lead, the refreshes and the
// prune, and confirms that it leads at each heartbeat, until it misses one:
// then it steps down, and follows.
type election struct {
	s   *session
	st  *store.Store
	svc *service.Service
	me  store.Replica
	// ss is the replica's session, from openServeStore at first, or nil
	// once it has ended, until the replica joins again.
	ss *store.Session
	// lead runs the leader's work until its context is canceled.
	lead func(ctx context.Context)
	// leader is the leader as this replica last knew it, or nil, and seen
	// the ID of the last leader it saw, for nbpdns_leader_changes_total.
	leader *store.Leader
	seen   string
}

// run takes part in the election until ctx is canceled. Then it ends the
// replica's session, which releases the leader's lock, if it holds it, once
// the leader's work has stopped.
func (e *election) run(ctx context.Context) {
	defer e.leave()
	for {
		if e.ss == nil {
			e.join(ctx)
		}
		if e.ss != nil {
			if t := e.try(ctx); t != nil {
				e.leadUntil(ctx, t)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(leaderPoll):
		}
	}
}

// join opens a session for the replica again, after its last one ended.
func (e *election) join(ctx context.Context) {
	jctx, cancel := context.WithTimeout(ctx, electionTimeout)
	defer cancel()
	ss, err := e.st.Join(jctx, e.me)
	if err != nil {
		if ctx.Err() == nil {
			e.s.log.WarnContext(ctx, "couldn't join the database's replicas, so this replica follows; it tries again at its next poll", "err", err)
		}
		e.svc.Follow(e.leader)
		return
	}
	e.ss = ss
}

// leave ends the replica's session, if it has one.
func (e *election) leave() {
	if e.ss != nil {
		e.ss.Close()
		e.ss = nil
	}
}

// try tries for the leader's lock, and returns the takeover if it took it.
// Otherwise, it reads the leader's row, and follows. If the session fails,
// it ends it, for join to replace.
func (e *election) try(ctx context.Context) *store.Takeover {
	tctx, cancel := context.WithTimeout(ctx, electionTimeout)
	defer cancel()
	// A new leader refreshes at once.
	t, err := e.ss.TryLead(tctx, time.Now())
	switch {
	case err != nil:
		if ctx.Err() == nil {
			e.s.log.WarnContext(ctx, "couldn't try for the leader's lock, so this replica follows, and joins the database's replicas again", "err", err)
		}
		e.leave()
	case t != nil:
		return t
	default:
		l, err := e.st.Leader(tctx)
		if err != nil {
			if ctx.Err() == nil {
				e.s.log.WarnContext(ctx, "couldn't read which replica leads", "err", err)
			}
			break
		}
		if l != nil && e.seen != "" && l.ID != e.seen {
			e.s.metrics.LeaderChanged()
		}
		if l != nil {
			e.seen = l.ID
			e.s.metrics.Leader(false, l.Since)
		}
		e.leader = l
	}
	e.svc.Follow(e.leader)
	return nil
}

// leadUntil runs the leader's work, from takeover t, and confirms at each
// heartbeat that this replica still leads, until ctx is canceled or a
// heartbeat fails. Either way, it stops the work, and ends the session,
// which releases the leader's lock. A failed heartbeat ends the session
// first, so that the replica steps down within heartbeatInterval +
// heartbeatTimeout of its last heartbeat, whatever the work is doing.
func (e *election) leadUntil(ctx context.Context, t *store.Takeover) {
	e.s.metrics.AuditEvents(t.Events)
	if p := t.Previous; p != nil && p.ID != e.me.ID {
		e.s.metrics.LeaderChanged()
		e.s.log.InfoContext(ctx, "took over as the leader, so this replica refreshes and prunes",
			"previous_replica", p.Name, "previous_instance_id", p.ID, "previous_last_seen", p.LastSeen)
	} else {
		e.s.log.InfoContext(ctx, "took over as the leader, so this replica refreshes and prunes")
	}
	e.seen, e.leader = e.me.ID, &t.Leader
	e.s.metrics.Leader(true, t.Leader.Since)
	e.svc.Lead(t.Leader)

	wctx, stop := context.WithCancel(ctx)
	defer stop()
	done := make(chan struct{})
	go func() {
		defer close(done)
		e.lead(wctx)
	}()
	tick := time.NewTicker(heartbeatInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
		case <-tick.C:
		}
		var seen time.Time
		err := ctx.Err()
		if err == nil {
			hctx, cancel := context.WithTimeout(ctx, heartbeatTimeout)
			seen, err = e.ss.Heartbeat(hctx, e.svc.NextRefresh())
			cancel()
		}
		switch {
		case ctx.Err() != nil:
			// serve stops: the refresh in flight isn't recorded, so the
			// work stops at once.
			stop()
			<-done
			e.leave()
			return
		case err != nil:
			e.s.log.ErrorContext(ctx, "the leader missed its heartbeat, so this replica steps down: "+
				"it stops refreshing and pruning, releases the leader's lock, and follows", "err", err)
			stop()
			e.leave()
			<-done
			e.leader = nil
			e.s.metrics.Leader(false, time.Time{})
			e.svc.Follow(nil)
			return
		}
		e.leader.LastSeen, e.leader.NextRefresh = seen, e.svc.NextRefresh()
		e.svc.Lead(*e.leader)
	}
}
