package store

import (
	"strings"
	"testing"
)

// TestPostgresOpenErrors checks that a PostgreSQL URL that can't be used
// fails with an error that doesn't show its password.
func TestPostgresOpenErrors(t *testing.T) {
	const pw = "s3cret-pw"
	for _, tt := range []struct{ name, url, want string }{
		{"no server", "postgres://nbpdns:" + pw + "@127.0.0.1:1/nbpdns?sslmode=disable&connect_timeout=5", "connect: connection refused"},
		{"a bad port", "postgres://nbpdns:" + pw + "@127.0.0.1:x/nbpdns", "cannot parse"},
		{"a bad sslmode", "postgres://nbpdns:" + pw + "@127.0.0.1/nbpdns?sslmode=sometimes", "sslmode is invalid"},
		{"a missing sslrootcert", "postgres://nbpdns:" + pw + "@127.0.0.1/nbpdns?sslmode=verify-full&sslrootcert=/nonexistent/ca.pem",
			"unable to read CA file"},
		{"a password in the query", "postgres://nbpdns@127.0.0.1:x/nbpdns?password=" + pw, "cannot parse"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := OpenPostgres(t.Context(), tt.url)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("OpenPostgres() error = %v, want %q", err, tt.want)
			}
			if strings.Contains(err.Error(), pw) {
				t.Errorf("the error shows the password: %v", err)
			}
		})
	}
}
