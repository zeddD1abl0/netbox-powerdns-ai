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

// A Lock is held on a database by the one `nbpdns serve` that may use it
// (ADR-0038, REQ-033). On SQLite, it's an exclusive flock on the file beside
// the database, path.lock, which the kernel releases if the process dies,
// however it dies; the lock file names its holder, for the error that
// another serve gets. On PostgreSQL, it's a session advisory lock, held on a
// connection of its own, which PostgreSQL releases when the session ends;
// the session's application_name names its holder (ADR-0046).
type Lock struct {
	f    *os.File
	conn *sql.Conn
}

// serveLockKey is the key of the session advisory lock that one nbpdns serve
// holds on a PostgreSQL database: "nbpdns" in ASCII, then 2. Advisory locks
// belong to their database, so two databases on one server never share it.
const serveLockKey int64 = 0x6e62_7064_6e73_0002

// LockedError is the error for a database whose lock another process holds.
type LockedError struct {
	// Path is the lock file, or empty on PostgreSQL.
	Path string
	// Holder names the holder, as it named itself: its host and PID, and
	// when it took the lock.
	Holder string
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
	host, _ := os.Hostname()
	holder := fmt.Sprintf("host %s, PID %d, since %s", host, os.Getpid(), time.Now().UTC().Format(time.RFC3339))
	// The holder's name only helps the error another process gets; the lock
	// holds without it.
	if f.Truncate(0) == nil {
		_, _ = f.WriteAt([]byte(holder+"\n"), 0)
	}
	return &Lock{f: f}, nil
}

// Lock takes the lock on the store's database, or fails with a *LockedError
// if another process holds it. It doesn't wait. On PostgreSQL, the lock is
// held on a connection of its own until Close. On SQLite, it's Acquire's,
// which can be taken before the database is opened.
func (s *Store) Lock(ctx context.Context) (*Lock, error) {
	if s.dialect != postgresDialect {
		return Acquire(s.path)
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("locking the database: %w", err)
	}
	var locked bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", serveLockKey).Scan(&locked); err != nil {
		discard(conn)
		return nil, fmt.Errorf("locking the database: %w", err)
	}
	if !locked {
		holder := lockHolder(ctx, conn)
		discard(conn)
		return nil, &LockedError{Holder: holder}
	}
	// As on SQLite, the holder's name only helps the error that another
	// process gets. PostgreSQL keeps 63 bytes of it.
	host, _ := os.Hostname()
	name := fmt.Sprintf("nbpdns: host %s, PID %d", host, os.Getpid())
	_, _ = conn.ExecContext(ctx, "SELECT set_config('application_name', $1, false)", name)
	return &Lock{conn: conn}, nil
}

// lockHolder names the session that holds the serve lock, from its
// application_name and when it connected, or returns nothing if it can't.
func lockHolder(ctx context.Context, conn *sql.Conn) string {
	var name sql.NullString
	var since sql.NullTime
	// A bigint key is split into classid, its high 32 bits, and objid, its
	// low ones, with objsubid 1.
	err := conn.QueryRowContext(ctx, `SELECT a.application_name, a.backend_start
FROM pg_locks l JOIN pg_stat_activity a ON a.pid = l.pid
WHERE l.locktype = 'advisory' AND l.granted AND l.objsubid = 1
  AND l.database = (SELECT oid FROM pg_database WHERE datname = current_database())
  AND l.classid::bigint = $1 AND l.objid::bigint = $2`,
		serveLockKey>>32, serveLockKey&0xffff_ffff).Scan(&name, &since)
	if err != nil || name.String == "" {
		return ""
	}
	holder := strings.TrimPrefix(printable(name.String), "nbpdns: ")
	if since.Valid {
		holder += ", since " + since.Time.UTC().Format(time.RFC3339)
	}
	return holder
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
