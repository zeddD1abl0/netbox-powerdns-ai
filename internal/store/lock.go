package store

import (
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
// (ADR-0038, REQ-033): an exclusive flock on the file beside it, path.lock.
// The kernel releases it if the process dies, however it dies. The lock file
// names its holder, for the error that another serve gets.
type Lock struct {
	f *os.File
}

// LockedError is the error for a database whose lock another process holds.
type LockedError struct {
	// Path is the lock file.
	Path string
	// Holder names the holder, as it wrote itself into the lock file:
	// its host, PID and start time.
	Holder string
}

func (e *LockedError) Error() string {
	holder := e.Holder
	if holder == "" {
		holder = "a process that didn't say which"
	}
	return fmt.Sprintf("another nbpdns holds the database's lock, %s: %s; stop it first, "+
		"or give this one another database.url", e.Path, holder)
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

// Close releases the lock. The lock file stays, for the next holder:
// removing it would let two processes lock different files of one name.
func (l *Lock) Close() error {
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
