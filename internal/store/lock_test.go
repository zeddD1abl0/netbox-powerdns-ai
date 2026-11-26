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

// wantHolder returns how a lock's holder on db names this process: on
// SQLite, by its host's whole name; on PostgreSQL, as its application_name
// has it.
func wantHolder(db backend) string {
	if db.name == "sqlite" {
		host, _ := hostname()
		return fmt.Sprintf("host %s, PID %d, since ", host, os.Getpid())
	}
	return strings.TrimPrefix(holderName(), "nbpdns: ") + ", since "
}

// TestApplicationName checks that the name of a process's sessions keeps
// its PID, in the 63 bytes that PostgreSQL keeps, whatever its host's name.
func TestApplicationName(t *testing.T) {
	runner := "runner-d7rzzl1zg-project-22-concurrent-0-6pxrh5d4"
	tests := []struct {
		name, host string
		pid        int
		want       string
	}{
		{"a short host name", "nbpdns-0", 1, "nbpdns: host nbpdns-0, PID 1"},
		{"a host name that just fits", strings.Repeat("a", 40), 1234, "nbpdns: host " + strings.Repeat("a", 40) + ", PID 1234"},
		{"one byte longer", strings.Repeat("a", 40) + "b", 1234, "nbpdns: host " + strings.Repeat("a", 19) + "..." + strings.Repeat("a", 17) + "b, PID 1234"},
		{"a GitLab runner's", runner, 6368, "nbpdns: host runner-d7rzzl1zg-pr...current-0-6pxrh5d4, PID 6368"},
		{"the longest, with the largest PID", strings.Repeat("h", 253) + "x", 4194304,
			"nbpdns: host " + strings.Repeat("h", 17) + "..." + strings.Repeat("h", 16) + "x, PID 4194304"},
		{"characters that PostgreSQL would escape", "hôst\x1b", 7, "nbpdns: host h?st?, PID 7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applicationName(tt.host, tt.pid)
			if got != tt.want || len(got) > maxApplicationName {
				t.Errorf("applicationName(%q, %d) = %q (%d bytes), want %q, at most %d bytes", tt.host, tt.pid, got, len(got), tt.want, maxApplicationName)
			}
		})
	}
}

// TestLongHostName checks that a lock's holder on a host with a long name
// names the host and its PID, on each database (ITEM-0120): PostgreSQL
// keeps 63 bytes of a session's application_name.
func TestLongHostName(t *testing.T) {
	long := "nbpdns-" + strings.Repeat("0123456789", 6) + "-x2k4q"
	hostname = func() (string, error) { return long, nil }
	t.Cleanup(func() { hostname = os.Hostname })
	onEach(t, func(t *testing.T, db backend) {
		where := db.create(t)
		l, err := db.connect(t, where).Lock(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()
		_, err = db.connect(t, where).Lock(t.Context())
		var le *LockedError
		if !errors.As(err, &le) {
			t.Fatalf("a second Lock() error = %v, want a LockedError", err)
		}
		want := wantHolder(db)
		if !strings.HasPrefix(le.Holder, want) || !strings.Contains(le.Holder, fmt.Sprintf(", PID %d, since ", os.Getpid())) {
			t.Errorf("Holder = %q, want it to start %q, with the PID", le.Holder, want)
		}
		if db.name != "sqlite" && !strings.Contains(want, "...") {
			t.Errorf("on PostgreSQL, the holder %q doesn't say that the host's name is cut short", want)
		}
	})
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
		if want := wantHolder(db); !strings.HasPrefix(le.Holder, want) {
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
