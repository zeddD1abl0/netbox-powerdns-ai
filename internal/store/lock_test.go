package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLock(t *testing.T) {
	db := filepath.Join(t.TempDir(), "nbpdns.db")
	l, err := Acquire(db)
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(db + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("the lock's permissions = %v, want -rw-------", perm)
	}

	// A flock belongs to an open file, so a second one in this process
	// conflicts, as another process's would.
	_, err = Acquire(db)
	var le *LockedError
	if !errors.As(err, &le) {
		t.Fatalf("a second Acquire() error = %v, want a LockedError", err)
	}
	host, _ := os.Hostname()
	want := fmt.Sprintf("host %s, PID %d, since ", host, os.Getpid())
	if le.Path != db+".lock" || !strings.HasPrefix(le.Holder, want) {
		t.Errorf("LockedError = %+v, want the lock's path, and a holder starting %q", le, want)
	}
	if !strings.Contains(err.Error(), "another nbpdns holds the database's lock") ||
		!strings.Contains(err.Error(), "stop it first, or give this one another database.url") {
		t.Errorf("error = %v", err)
	}

	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	l2, err := Acquire(db)
	if err != nil {
		t.Fatalf("Acquire() after Close() error = %v", err)
	}
	_ = l2.Close()
}

// TestLockHolderIsPrintable checks that the holder that a lock file names,
// which anyone who can write it chose, can't put control characters in an
// error.
func TestLockHolderIsPrintable(t *testing.T) {
	db := filepath.Join(t.TempDir(), "nbpdns.db")
	l, err := Acquire(db)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if err := os.WriteFile(db+".lock", []byte("\x1b[2Jhost evil\n\tPID 1\r\n"+strings.Repeat("x", 1000)), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = Acquire(db)
	var le *LockedError
	if !errors.As(err, &le) {
		t.Fatalf("Acquire() error = %v, want a LockedError", err)
	}
	if want := "[2Jhost evil PID 1 x"; !strings.HasPrefix(le.Holder, want) || len(le.Holder) > maxHolder {
		t.Errorf("Holder = %q, want it to start %q, and to be at most %d bytes", le.Holder, want, maxHolder)
	}
}

func TestLockErrors(t *testing.T) {
	_, err := Acquire(filepath.Join(t.TempDir(), "missing", "nbpdns.db"))
	if err == nil || !strings.Contains(err.Error(), "missing, doesn't exist; create it, or mount a volume there") {
		t.Errorf("Acquire() in a missing directory: error = %v", err)
	}
	if os.Getuid() == 0 {
		return // root can write to any directory
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil { //nolint:gosec // A directory nbpdns can't write to.
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) }) //nolint:gosec // The test's own directory, as it was.
	_, err = Acquire(filepath.Join(dir, "nbpdns.db"))
	if want := fmt.Sprintf("make %s writable by user %d", dir, os.Getuid()); err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("Acquire() in a read-only directory: error = %v, want it to contain %q", err, want)
	}
}

// TestStoreLock checks that one process at a time holds a database's lock,
// on each database, and that Close releases it.
func TestStoreLock(t *testing.T) {
	onEach(t, func(t *testing.T, db backend) {
		where := db.create(t)
		l, err := db.connect(t, where).Lock(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		// Another process, with a store of its own.
		other := db.connect(t, where)
		_, err = other.Lock(t.Context())
		var le *LockedError
		if !errors.As(err, &le) {
			t.Fatalf("a second Lock() error = %v, want a LockedError", err)
		}
		host, _ := os.Hostname()
		if want := fmt.Sprintf("host %s, PID %d, since ", host, os.Getpid()); !strings.HasPrefix(le.Holder, want) {
			t.Errorf("Holder = %q, want it to start %q", le.Holder, want)
		}
		if !strings.Contains(err.Error(), "another nbpdns holds the database's lock") {
			t.Errorf("error = %v", err)
		}
		if err := l.Close(); err != nil {
			t.Fatal(err)
		}
		l2, err := other.Lock(t.Context())
		if err != nil {
			t.Fatalf("Lock() after Close() error = %v", err)
		}
		_ = l2.Close()
	})
}
