//go:build integration

package store

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/lab"
)

// sslRequest is the code of PostgreSQL's SSLRequest message, with which a
// client asks for TLS before its startup message.
const sslRequest = 80877103

// A tlsServer is a PostgreSQL server with TLS, for the tests, since the
// lab's has none: it takes TLS sessions as PostgreSQL does, with a
// certificate for localhost from a CA of its own, requires a client
// certificate from that CA, and forwards each session, decrypted, to the
// lab's PostgreSQL. It refuses sessions without TLS.
type tlsServer struct {
	addr string
	// ca is the CA's certificate, and cert and key a client certificate
	// from it, with its key, each a PEM file.
	ca, cert, key string
	config        *tls.Config

	mu sync.Mutex
	// clients are the common names of the client certificates that
	// sessions presented.
	clients []string
	conns   []net.Conn
}

func newTLSServer(t *testing.T) *tlsServer {
	t.Helper()
	dir := t.TempDir()
	ca, caKey, caPEM := newCA(t, "nbpdns test CA")
	serverPEM, serverKey := issue(t, ca, caKey, &x509.Certificate{
		Subject:     pkix.Name{CommonName: "localhost"},
		DNSNames:    []string{"localhost"},
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	clientPEM, clientKey := issue(t, ca, caKey, &x509.Certificate{
		Subject:     pkix.Name{CommonName: lab.PostgresUser},
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	s := &tlsServer{
		ca:   writeFile(t, dir, "ca.pem", caPEM),
		cert: writeFile(t, dir, "client.pem", clientPEM),
		key:  writeFile(t, dir, "client-key.pem", clientKey),
	}
	serverCert, err := tls.X509KeyPair(serverPEM, serverKey)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	s.config = &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    pool,
		MinVersion:   tls.VersionTLS12,
	}
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s.addr = ln.Addr().String()
	var wg sync.WaitGroup
	t.Cleanup(func() {
		_ = ln.Close()
		s.mu.Lock()
		for _, c := range s.conns {
			_ = c.Close()
		}
		s.mu.Unlock()
		wg.Wait()
	})
	wg.Go(func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			s.mu.Lock()
			s.conns = append(s.conns, c)
			s.mu.Unlock()
			wg.Go(func() { s.serve(c, &wg) })
		}
	})
	return s
}

// serve takes a session's TLS, then forwards it to the lab's PostgreSQL.
func (s *tlsServer) serve(c net.Conn, wg *sync.WaitGroup) {
	defer c.Close()
	var req [8]byte
	if _, err := io.ReadFull(c, req[:]); err != nil ||
		binary.BigEndian.Uint32(req[:4]) != 8 || binary.BigEndian.Uint32(req[4:]) != sslRequest {
		return
	}
	if _, err := c.Write([]byte{'S'}); err != nil {
		return
	}
	tc := tls.Server(c, s.config)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := tc.HandshakeContext(ctx); err != nil {
		return
	}
	s.mu.Lock()
	s.clients = append(s.clients, tc.ConnectionState().PeerCertificates[0].Subject.CommonName)
	s.mu.Unlock()
	backend, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(lab.Host(), strconv.Itoa(lab.PostgresPort)))
	if err != nil {
		return
	}
	defer backend.Close()
	wg.Go(func() {
		_, _ = io.Copy(backend, tc)
		_ = backend.Close()
	})
	_, _ = io.Copy(tc, backend)
}

// sawClient reports whether a session presented a client certificate for
// name.
func (s *tlsServer) sawClient(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.clients {
		if c == name {
			return true
		}
	}
	return false
}

// newCA makes a CA, and returns its certificate, its key, and the
// certificate in PEM.
func newCA(t *testing.T, name string) (*x509.Certificate, *ecdsa.PrivateKey, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return ca, key, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

// issue signs tmpl with the CA, and returns the certificate and a new key,
// in PEM.
func issue(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, tmpl *x509.Certificate) (cert, key []byte) {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl.SerialNumber = big.NewInt(time.Now().UnixNano())
	tmpl.NotBefore, tmpl.NotAfter = time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
	tmpl.KeyUsage = x509.KeyUsageDigitalSignature
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &k.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	b, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: b})
}

// writeFile writes b to name in dir, readable only by its owner, as libpq
// asks of a key, and returns its path.
func writeFile(t *testing.T, dir, name string, b []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// withURL returns where, a lab database's URL, with host as its host, if
// set, without its password if noPassword is set, and with params as its
// query.
func withURL(t *testing.T, where, host string, noPassword bool, params url.Values) string {
	t.Helper()
	u, err := url.Parse(where)
	if err != nil {
		t.Fatal(err)
	}
	if host != "" {
		u.Host = host
	}
	if noPassword {
		u.User = url.User(u.User.Username())
	}
	u.RawQuery = params.Encode()
	return u.String()
}

// TestPostgresTLS checks that the store connects to PostgreSQL as libpq's
// parameters say: with TLS and a client certificate, verifying the server's
// as sslmode asks, or refusing; and with the password from a passfile or
// libpq's environment (ADR-0046). No error holds the password.
func TestPostgresTLS(t *testing.T) {
	srv := newTLSServer(t)
	where := lab.NewPostgresDatabase(t)
	_, port, err := net.SplitHostPort(srv.addr)
	if err != nil {
		t.Fatal(err)
	}
	local, byIP := net.JoinHostPort("localhost", port), srv.addr
	_, _, otherPEM := newCA(t, "another CA")
	otherCAFile := writeFile(t, t.TempDir(), "other-ca.pem", otherPEM)
	passfile := writeFile(t, t.TempDir(), "pgpass", []byte("*:*:*:"+lab.PostgresUser+":"+lab.PostgresPassword+"\n"))
	tlsParams := func(mode string, extra ...string) url.Values {
		v := url.Values{"sslmode": {mode}, "sslcert": {srv.cert}, "sslkey": {srv.key}, "sslrootcert": {srv.ca}}
		for i := 0; i < len(extra); i += 2 {
			if extra[i+1] == "" {
				v.Del(extra[i])
			} else {
				v.Set(extra[i], extra[i+1])
			}
		}
		return v
	}
	tests := []struct {
		name string
		url  string
		env  map[string]string
		// wantTLS is whether the connection uses TLS, or wantErr the
		// error that refuses it.
		wantTLS bool
		wantErr string
	}{
		{name: "verify-full, with a client certificate", url: withURL(t, where, local, false, tlsParams("verify-full")), wantTLS: true},
		{name: "verify-ca, by the server's address", url: withURL(t, where, byIP, false, tlsParams("verify-ca")), wantTLS: true},
		{name: "require, which doesn't verify the server",
			url: withURL(t, where, byIP, false, tlsParams("require", "sslrootcert", "")), wantTLS: true},
		{name: "verify-full, by a name that the certificate lacks",
			url: withURL(t, where, byIP, false, tlsParams("verify-full")), wantErr: "cannot validate certificate for 127.0.0.1"},
		{name: "verify-ca, with another CA",
			url: withURL(t, where, local, false, tlsParams("verify-ca", "sslrootcert", otherCAFile)), wantErr: "certificate signed by unknown authority"},
		{name: "without the client certificate that the server requires",
			url: withURL(t, where, local, false, tlsParams("verify-full", "sslcert", "", "sslkey", "")), wantErr: "failed to connect"},
		{name: "a certificate without its key",
			url: withURL(t, where, local, false, tlsParams("verify-full", "sslkey", "")), wantErr: `both "sslcert" and "sslkey" are required`},
		{name: "require, on a server without TLS",
			url: withURL(t, where, "", false, url.Values{"sslmode": {"require"}}), wantErr: "server refused TLS connection"},
		{name: "prefer, libpq's default, on a server without TLS", url: withURL(t, where, "", false, nil)},
		{name: "the password from a passfile",
			url: withURL(t, where, "", true, url.Values{"sslmode": {"disable"}, "passfile": {passfile}})},
		{name: "the password from PGPASSWORD", url: withURL(t, where, "", true, url.Values{"sslmode": {"disable"}}),
			env: map[string]string{"PGPASSWORD": lab.PostgresPassword}},
		{name: "PGSSLMODE, under the URL's own sslmode", url: withURL(t, where, "", false, url.Values{"sslmode": {"disable"}}),
			env: map[string]string{"PGSSLMODE": "require"}},
		{name: "PGSSLMODE, where the URL has none", url: withURL(t, where, "", false, nil),
			env: map[string]string{"PGSSLMODE": "require"}, wantErr: "server refused TLS connection"},
		{name: "a wrong password", url: strings.Replace(where, lab.PostgresPassword, "not-"+lab.PostgresPassword, 1),
			wantErr: "password authentication failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			s, err := OpenPostgres(t.Context(), tt.url)
			if err != nil {
				if strings.Contains(err.Error(), lab.PostgresPassword) {
					t.Errorf("the error shows the password: %v", err)
				}
				if tt.wantErr == "" || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("OpenPostgres() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			defer s.Close()
			if tt.wantErr != "" {
				t.Fatalf("OpenPostgres() succeeded, want %q", tt.wantErr)
			}
			if s.TLS() != tt.wantTLS || s.Cleartext() == tt.wantTLS {
				t.Errorf("TLS() = %v, Cleartext() = %v, want TLS %v", s.TLS(), s.Cleartext(), tt.wantTLS)
			}
			// The session works, through the TLS server where there's one.
			if _, err := s.Status(t.Context()); err != nil {
				t.Errorf("Status() error = %v", err)
			}
		})
	}
	if !srv.sawClient(lab.PostgresUser) {
		t.Error("no session presented the client certificate")
	}
}
