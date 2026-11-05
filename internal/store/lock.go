package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode"
)

// maxHolder bounds what's read of a lock file's holder.
const maxHolder = 256

// A Lock is held on a database by the one nbpdns process that may change it
// while no serve runs (ADR-0038, REQ-033). On SQLite, it's an exclusive flock
// on the file beside the database, path.lock, which the kernel releases if
// the process dies, however it dies; the lock file names its holder, for the
// error that another process gets. serve holds it there too, as the one
// instance on the database. On PostgreSQL, it's the serve lock, a session
// advisory lock, taken exclusively, on a connection of its own, which
// PostgreSQL releases when the session ends; the session's application_name
// names its holder (ADR-0046). There, each replica of serve holds the serve
// lock shared, through Join (ADR-0047), so that db migrate, say, refuses
// while any replica runs.
type Lock struct {
	f    *os.File
	conn *sql.Conn
}

// serveLockKey is the key of the serve lock on a PostgreSQL database: "nbpdns"
// in ASCII, then 2. Advisory locks belong to their database, so two
// databases on one server never share it.
const serveLockKey int64 = 0x6e62_7064_6e73_0002

// maxHolders is how many of a lock's holders a LockedError names.
const maxHolders = 3

// LockedError is the error for a database whose lock another process holds.
type LockedError struct {
	// Path is the lock file, or empty on PostgreSQL.
	Path string
	// Holder names the holder, as it named itself: its host and PID, and
	// when it took the lock. On PostgreSQL, it names up to three, of
	// Holders.
	Holder  string
	Holders int
}

func (e *LockedError) Error() string {
	holder := e.Holder
	if holder == "" {
		holder = "a process that didn't say which"
	}
	lock := "the database's lock"
	if e.Path != "" {
		lock += ", " + e.Path
	}
	if e.Holders > 1 {
		return fmt.Sprintf("%d other nbpdns processes hold %s: %s; stop them first, "+
			"or give this one another database.url", e.Holders, lock, holder)
	}
	return fmt.Sprintf("another nbpdns holds %s: %s; stop it first, "+
		"or give this one another database.url", lock, holder)
}

// Acquire takes the lock on the database at path, or fails with a
// *LockedError if another process holds it. It doesn't wait.
func Acquire(path string) (*Lock, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("the database's path: %w", err)
	}
	if err := checkDir(path); err != nil {
		return nil, err
	}
	lp := path + ".lock"
	f, err := os.OpenFile(lp, os.O_RDWR|os.O_CREATE, 0o600)
	switch {
	case errors.Is(err, fs.ErrPermission):
		return nil, fmt.Errorf("opening the database's lock: %w; make %s writable by user %d", err, filepath.Dir(path), os.Getuid())
	case err != nil:
		return nil, fmt.Errorf("opening the database's lock: %w", err)
	}
	if err := flock(f, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		defer f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			b, _ := io.ReadAll(io.LimitReader(f, maxHolder))
			return nil, &LockedError{Path: lp, Holder: printable(string(b))}
		}
		return nil, fmt.Errorf("locking the database, %s: %w", lp, err)
	}
	host, _ := hostname()
	holder := fmt.Sprintf("host %s, PID %d, since %s", host, os.Getpid(), time.Now().UTC().Format(time.RFC3339))
	// The holder's name only helps the error another process gets; the lock
	// holds without it.
	if f.Truncate(0) == nil {
		_, _ = f.WriteAt([]byte(holder+"\n"), 0)
	}
	return &Lock{f: f}, nil
}

// Lock takes the lock on the store's database, or fails with a *LockedError
// if another process holds it, such as a replica of serve on PostgreSQL. It
// doesn't wait. On PostgreSQL, the lock is held on a connection of its own
// until Close. On SQLite, it's Acquire's, which can be taken before the
// database is opened.
func (s *Store) Lock(ctx context.Context) (*Lock, error) {
	if s.dialect != postgresDialect {
		return Acquire(s.path)
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("locking the database: %w", err)
	}
	// As on SQLite, the holder's name only helps the error that another
	// process gets.
	if _, err := conn.ExecContext(ctx, "SELECT set_config('application_name', $1, false)", holderName()); err != nil {
		discard(conn)
		return nil, fmt.Errorf("locking the database: %w", err)
	}
	var locked bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", serveLockKey).Scan(&locked); err != nil {
		discard(conn)
		return nil, fmt.Errorf("locking the database: %w", err)
	}
	if !locked {
		err := lockedError(ctx, conn)
		discard(conn)
		return nil, err
	}
	return &Lock{conn: conn}, nil
}

// maxApplicationName is how much of application_name PostgreSQL keeps:
// NAMEDATALEN - 1 bytes.
const maxApplicationName = 63

// hostname returns the host's name, for the names of the process's
// sessions. A test may replace it.
var hostname = os.Hostname

// holderName is the application_name of a session that holds the serve
// lock, or the leader's lock, or listens, which names the process.
func holderName() string {
	host, _ := hostname()
	return applicationName(host, os.Getpid())
}

// applicationName returns the application_name that names the process with
// PID pid on host. PostgreSQL keeps 63 bytes of it, so a host name too long
// to fit beside the PID is cut short in its middle, at "...", which says
// so: a pod's or a runner's name tells it apart at its end, and a domain
// name at its start. PostgreSQL takes only printable ASCII in it, and
// escapes the rest, which would lengthen it, so any other character is "?".
func applicationName(host string, pid int) string {
	host = strings.Map(func(r rune) rune {
		if r < ' ' || r > '~' {
			return '?'
		}
		return r
	}, host)
	prefix, suffix := "nbpdns: host ", fmt.Sprintf(", PID %d", pid)
	if room := maxApplicationName - len(prefix) - len(suffix); len(host) > room {
		const cut = "..."
		keep := room - len(cut)
		head := (keep + 1) / 2
		host = host[:head] + cut + host[len(host)-(keep-head):]
	}
	return prefix + host + suffix
}

// lockedError returns the error for the serve lock that conn couldn't take,
// which names its holders, from their sessions' application_name and when
// they connected, if it can.
func lockedError(ctx context.Context, conn *sql.Conn) *LockedError {
	// A bigint key is split into classid, its high 32 bits, and objid, its
	// low ones, with objsubid 1.
	rows, err := conn.QueryContext(ctx, `SELECT a.application_name, a.backend_start
FROM pg_locks l JOIN pg_stat_activity a ON a.pid = l.pid
WHERE l.locktype = 'advisory' AND l.granted AND l.objsubid = 1
  AND l.database = (SELECT oid FROM pg_database WHERE datname = current_database())
  AND l.classid::bigint = $1 AND l.objid::bigint = $2
ORDER BY a.backend_start`,
		serveLockKey>>32, serveLockKey&0xffff_ffff)
	if err != nil {
		return &LockedError{}
	}
	defer rows.Close()
	e := &LockedError{}
	var named []string
	for rows.Next() {
		var name sql.NullString
		var since sql.NullTime
		if rows.Scan(&name, &since) != nil {
			return &LockedError{}
		}
		e.Holders++
		if len(named) == maxHolders || name.String == "" {
			continue
		}
		holder := strings.TrimPrefix(printable(name.String), "nbpdns: ")
		if since.Valid {
			holder += ", since " + since.Time.UTC().Format(time.RFC3339)
		}
		named = append(named, holder)
	}
	if rows.Err() != nil {
		return &LockedError{}
	}
	e.Holder = strings.Join(named, "; ")
	if n := e.Holders - len(named); n > 0 && len(named) > 0 {
		e.Holder += fmt.Sprintf("; and %d more", n)
	}
	return e
}

// Close releases the lock. On SQLite, the lock file stays, for the next
// holder: removing it would let two processes lock different files of one
// name. On PostgreSQL, the lock's session ends.
func (l *Lock) Close() error {
	if l.conn != nil {
		discard(l.conn)
		return nil
	}
	return l.f.Close()
}

// flock applies how to f's file descriptor.
func flock(f *os.File, how int) error {
	conn, err := f.SyscallConn()
	if err != nil {
		return err
	}
	var ferr error
	if err := conn.Control(func(fd uintptr) {
		ferr = syscall.Flock(int(fd), how)
	}); err != nil {
		return err
	}
	return ferr
}

// printable returns s on one line, without control characters, since it
// comes from a file that anyone who can write it chose.
func printable(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return ' '
	}, s)
	return strings.Join(strings.Fields(s), " ")
}
