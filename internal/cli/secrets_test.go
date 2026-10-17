package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/secrets"
)

// keyFile writes a new master key to a file, and returns it and the file.
func keyFile(t *testing.T) (secrets.MasterKey, string) {
	t.Helper()
	k := secrets.NewMasterKey()
	path := filepath.Join(t.TempDir(), "master.key")
	if err := os.WriteFile(path, []byte(k.Base64()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return k, path
}

// TestSecretSettings stores a secret setting, sealed, and reads it back only
// with the master key, before and after a rotation.
func TestSecretSettings(t *testing.T) {
	db := filepath.Join(t.TempDir(), "nbpdns.db")
	noKey := map[string]string{"NBPDNS_DATABASE_URL": "sqlite://" + db}
	key, keyPath := keyFile(t)
	withKey := map[string]string{"NBPDNS_DATABASE_URL": noKey["NBPDNS_DATABASE_URL"], "NBPDNS_DATABASE_MASTER_KEY_FILE": keyPath}
	if code, _, stderr := run(t, noKey, "db", "migrate"); code != exitOK {
		t.Fatalf("db migrate: exit %d:\n%s", code, stderr)
	}
	const token = "nbt_abc.s3cretS3cretS3cret"
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte(token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if code, _, stderr := run(t, withKey, "settings", "set", "netbox.token", token); code != exitUsage ||
		!strings.Contains(stderr, "netbox.token is a secret: pass it with --from-file, not on the command line") || strings.Contains(stderr, "s3cret") {
		t.Errorf("a secret on the command line: exit %d:\n%s", code, stderr)
	}
	if code, _, stderr := run(t, noKey, "settings", "set", "netbox.token", "--from-file", tokenFile); code != exitError ||
		!strings.Contains(stderr, "database.master_key, which isn't set") {
		t.Errorf("a secret without a master key: exit %d:\n%s", code, stderr)
	}
	code, stdout, stderr := run(t, withKey, "settings", "set", "netbox.token", "--from-file", tokenFile, "--reason", "rotated")
	if code != exitOK || !strings.HasPrefix(stdout, "Stored [redacted] for netbox.token, sealed with the master key.\n") {
		t.Fatalf("settings set of a secret: exit %d: %q\n%s", code, stdout, stderr)
	}
	code, stdout, _ = run(t, withKey, "settings", "list")
	if code != exitOK || !regexp.MustCompile(`(?m)^netbox\.token +\[redacted\] +database +\[redacted\]$`).MatchString(stdout) {
		t.Errorf("settings list: exit %d:\n%s", code, stdout)
	}

	// No secret is in the database's files unsealed, not even in the audit
	// trail.
	for _, f := range []string{db, db + "-wal"} {
		b, err := os.ReadFile(f)
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if bytes.Contains(b, []byte("s3cret")) {
			t.Errorf("%s holds the secret unsealed", f)
		}
	}
	if _, stdout, _ = run(t, withKey, "audit", "list", "-o", "json"); strings.Contains(stdout, "s3cret") || !strings.Contains(stdout, "[redacted]") {
		t.Errorf("the audit trail shows the secret, or doesn't redact it:\n%s", stdout)
	}

	// Without the master key, or with another, the secret doesn't open.
	if code, _, stderr := run(t, noKey, "settings", "list"); code != exitError ||
		!strings.Contains(stderr, "the database stores a secret for netbox.token, which doesn't open: it's sealed with master key "+key.ID()+", and database.master_key isn't set") {
		t.Errorf("settings list without the master key: exit %d:\n%s", code, stderr)
	}
	otherKey, otherPath := keyFile(t)
	other := map[string]string{"NBPDNS_DATABASE_URL": noKey["NBPDNS_DATABASE_URL"], "NBPDNS_DATABASE_MASTER_KEY_FILE": otherPath}
	if code, _, stderr := run(t, other, "settings", "list"); code != exitError || !strings.Contains(stderr, "it's sealed with master key "+key.ID()+", but database.master_key is") {
		t.Errorf("settings list with another master key: exit %d:\n%s", code, stderr)
	}
	// serve refuses to start with a secret it can't open.
	serveEnv := configFile(t, twoGroups)
	serveEnv["NBPDNS_NETBOX_URL"], serveEnv["NBPDNS_DATABASE_URL"] = "http://127.0.0.1:1", noKey["NBPDNS_DATABASE_URL"]
	if code, _, stderr := run(t, serveEnv, "serve"); code != exitError || !strings.Contains(stderr, "the database stores a secret for netbox.token, which doesn't open") {
		t.Errorf("serve without the master key: exit %d:\n%s", code, stderr)
	}

	// Rotate to the other key: the old one no longer opens the secret, and
	// the new one does.
	code, stdout, stderr = run(t, withKey, "secrets", "rotate-master-key", "--new-key-file", otherPath, "--reason", "yearly")
	if code != exitOK || !strings.HasPrefix(stdout, "Re-wrapped 1 secret with master key ") {
		t.Fatalf("rotate-master-key: exit %d: %q\n%s", code, stdout, stderr)
	}
	if code, _, stderr := run(t, withKey, "settings", "list"); code != exitError || !strings.Contains(stderr, "but database.master_key is master key "+key.ID()) {
		t.Errorf("the old key after the rotation: exit %d:\n%s", code, stderr)
	}
	if code, stdout, stderr := run(t, other, "settings", "get", "netbox.token"); code != exitOK || stdout != "[redacted]\n" {
		t.Errorf("the new key after the rotation: exit %d: %q\n%s", code, stdout, stderr)
	}
	// A secret sealed with the old key is refused, so the database keeps one
	// master key.
	if code, _, stderr := run(t, withKey, "settings", "set", "netbox.token", "--from-file", tokenFile); code != exitError ||
		!strings.Contains(stderr, "the database's secrets are sealed with master key "+otherKey.ID()+", but database.master_key is master key "+key.ID()) {
		t.Errorf("a secret sealed with the old key after the rotation: exit %d:\n%s", code, stderr)
	}
	if code, stdout, _ := run(t, other, "audit", "list", "--action", "master_key.rotated"); code != exitOK || !strings.Contains(stdout, "yearly") {
		t.Errorf("no rotation event: exit %d:\n%s", code, stdout)
	}
	if code, _, stderr := run(t, other, "secrets", "rotate-master-key", "--new-key-file", otherPath); code != exitError ||
		!strings.Contains(stderr, "the new master key is database.master_key") {
		t.Errorf("rotating to the same key: exit %d:\n%s", code, stderr)
	}
}

func TestSettingValueFromStdin(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("0123456789abcdef\n"))
	v, err := settingValue(cmd, "", "-")
	if err != nil || v != "0123456789abcdef" {
		t.Errorf("settingValue() = %q, %v", v, err)
	}
	for _, tt := range []struct {
		arg, from, want string
	}{
		{"v", "f", "not both"},
		{"", "", "give the value as an argument, or, for a secret, with --from-file"},
		{"", filepath.Join(t.TempDir(), "missing"), "reading the secret file"},
	} {
		if _, err := settingValue(cmd, tt.arg, tt.from); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("settingValue(%q, %q) = %v, want %q", tt.arg, tt.from, err, tt.want)
		}
	}
}
