---
title: '0049: End a lost replica''s write transaction after 20 seconds idle'
status: accepted # proposed | accepted | rejected | deprecated | superseded by ADR-NNNN
date: 2026-11-25
decision-makers: [jordan]
requirements: [REQ-033] # REQ-nnn this decision serves
questions: [] # Q-nnn this decision answers
supersedes: # ADR-NNNN this replaces, if any
---

# 0049: End a lost replica's write transaction after 20 seconds idle

## Context and problem statement

On PostgreSQL, every write transaction takes the store's write lock, a
transaction-scoped advisory lock, so that writes run one at a time
(ADR-0046). It runs on a connection of the store's pool. ADR-0047 bounds
how long a lost leader keeps the leader's lock, by the settings of the
replica's own session, but the pool's connections have none of them, and
the server's defaults end nothing: no idle-in-transaction timeout, and
keepalives after the OS's 7200 seconds.

So a replica lost inside a write keeps the write lock. A hung process, or
a half-open connection, keeps it until it resumes, since its kernel still
answers the keepalives. A dead host, or a lost network, keeps it for
hours. Meanwhile the replica that takes the leader's lock can't write its
takeover, and no replica, nor the command line, can write anything: each
write waits 5 seconds for the lock, then fails. ITEM-0116's cut-off test
didn't meet it, since it cuts the leader off between writes; ITEM-0126's
test does.

## Decision drivers

- REQ-033 and ADR-0047: a lost leader is replaced within about 30 seconds,
  so the write lock must be free by then.
- A healthy replica's write must not be ended: its statements follow each
  other within milliseconds.
- What the user's `database.url`, role or server sets stays theirs.
- A lost replica that resumes must not commit a write that the others have
  moved past.

## Considered options

1. `idle_in_transaction_session_timeout`, set locally in each write
   transaction as it takes the write lock.
2. `idle_in_transaction_session_timeout` for every pool connection, in its
   startup parameters.
3. PostgreSQL 17's `transaction_timeout`, set locally in each write
   transaction.
4. The replica session's keepalives and `tcp_user_timeout` on every pool
   connection too.
5. Nothing: document that a write lost mid-way can hold up every write.

## Decision outcome

Chosen option: **option 1**, with 20 seconds, because it ends exactly the
transactions that hold the write lock, whatever the loss, and touches
nothing else. `lockWrites` sets it with `set_config(…, true)`, in the
statement that already sets `lock_timeout`, so it costs no round trip.
When it fires, PostgreSQL ends the transaction and its connection, and
frees the lock; a replica that resumes finds its connection gone, so its
commit fails.

20 seconds is far longer than a healthy replica waits between the
statements of a write, and no later than a lost leader's session ends: a dead host's
keepalives end it 25 seconds after its last query, and the leader queries
at least every 5 seconds, so at least 20 seconds after the loss. The
replica that takes over then waits up to 5 seconds for the write lock
anyway.

### Consequences

- Good: a lost replica's write frees the write lock within 20 seconds;
  the new leader writes its takeover, and every other writer writes again.
- Good: a lost leader that resumes can't commit its write, whatever the
  fence checked when the write started.
- Good: reads, and the pool's connections between writes, keep
  PostgreSQL's defaults, and the user's settings stand.
- Bad: a write whose replica pauses for 20 seconds between statements,
  such as one starved of CPU, fails, and is counted as a write failure. A
  refresh's write is tried again at the next refresh.
- Bad: for those 20 seconds, the other writes wait and fail: a follower
  answers a webhook with `503`, and the command line fails. The new
  leader's first refresh is a full one, which covers the webhooks.
- Bad: it doesn't end a statement that's running as the replica is lost,
  such as one whose result the server can't finish sending. Writes'
  results are small, so this is left alone.

### Confirmation

`TestLeaderLostInAWrite`, in `internal/store`'s integration tests, cuts a
leader off inside a leader-only write, through `lab.Proxy`, which keeps its
sockets open. Another replica writes again within 25 seconds, 20 in
practice; it takes over within 40, 30 in practice; and the lost leader's
write, once the proxy is restored, isn't committed. Without this decision
the test fails: no write within 25 seconds.

## Pros and cons of the options

### Option 1: per write transaction

- Good: only the transactions that hold the write lock; one setting, no
  extra round trip.
- Bad: a read transaction left open still holds its snapshot, and holds
  back vacuum, until the connection ends; it holds no advisory lock.

### Option 2: on every pool connection

- Good: bounds read transactions too.
- Bad: overrides what the URL, the role or the server sets, or has to
  give way to it, so the bound would depend on the deployment.

### Option 3: `transaction_timeout`

- Good: bounds a running statement too.
- Bad: ends a slow write that's working, such as a large prune, so it would
  need a limit longer than the takeover.

### Option 4: keepalives on the pool's connections

- Good: ends a dead host's, or a lost network's, connections, idle or not.
- Bad: a hung process's, or a half-open connection's, kernel answers the
  keepalives, so it doesn't help there.

### Option 5: nothing

- Bad: a write lost mid-way holds up the takeover, and every write, for as
  long as hours.

## More information

- ITEM-0126, which found it in ITEM-0116's work and fixes it.
- ADR-0046, the write lock; ADR-0047, the leader's lock and its timings.
- PostgreSQL's `idle_in_transaction_session_timeout`: "Terminate any
  session that has been idle (that is, waiting for a client query) within
  an open transaction for longer than the specified amount of time."
