package powerdns

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
)

// Supported lists the PowerDNS Authoritative Server release series nbpdns
// supports (REQ-041), newest first. The lab runs each one as the primary of
// a server group (internal/lab), and the integration tests read from it, so
// a release is only added here with its lab instance (ADR-0026).
var Supported = []string{"5.1"}

// SupportedSeries returns the supported release series, as text such as
// "5.1.x", or "5.1.x or 5.0.x" for more than one.
func SupportedSeries() string { return seriesText(Supported) }

func seriesText(releases []string) string {
	s := make([]string, len(releases))
	for i, r := range releases {
		s[i] = r + ".x"
	}
	return strings.Join(s, " or ")
}

// The daemon type of a PowerDNS Authoritative Server.
const authoritative = "authoritative"

// Server is what the API reports about the server.
type Server struct {
	ID string `json:"id"`
	// DaemonType is authoritative for an authoritative server.
	DaemonType string `json:"daemon_type"`
	// Version is its release, such as 5.1.4.
	Version string `json:"version"`
}

// Authoritative reports whether the server is a PowerDNS Authoritative
// Server.
func (s *Server) Authoritative() bool { return s.DaemonType == authoritative }

// Supported reports whether the server runs a supported release.
func (s *Server) Supported() bool { return s.supported(Supported) }

func (s *Server) supported(releases []string) bool {
	return slices.Contains(releases, series(s.Version))
}

// series returns a version's release series: its first two numbers, such as
// 5.1 for 5.1.4.
func series(version string) string {
	parts := strings.SplitN(strings.TrimPrefix(version, "v"), ".", 3)
	if len(parts) < 2 {
		return version
	}
	return parts[0] + "." + parts[1]
}

// Server reads what the API reports about the server.
func (c *Client) Server(ctx context.Context) (*Server, error) {
	var s Server
	notFound := func() error { return &ServerNotFoundError{Group: c.group, ServerID: c.serverID} }
	if err := c.get(ctx, c.url, &s, notFound); err != nil {
		return nil, err
	}
	return &s, nil
}

// Connect reads what the server is, before a command reads its zones. A
// server that isn't authoritative is an error. An unsupported release only
// logs a warning, since reading may still work (ADR-0026).
func (c *Client) Connect(ctx context.Context) (*Server, error) {
	s, err := c.Server(ctx)
	if err != nil {
		return nil, err
	}
	if !s.Authoritative() {
		return nil, &NotAuthoritativeError{Group: c.group, DaemonType: s.DaemonType}
	}
	if !s.supported(c.supported) {
		err := &VersionError{Group: c.group, Version: s.Version, supported: c.supported}
		c.log.WarnContext(ctx, "this PowerDNS release isn't supported, so reading from it may fail or give wrong results",
			"group", c.group, "err", err)
	}
	return s, nil
}

// Check returns an error if s isn't an authoritative server, or if it isn't
// a supported release.
func (s *Server) Check(group string) error {
	if !s.Authoritative() {
		return &NotAuthoritativeError{Group: group, DaemonType: s.DaemonType}
	}
	if !s.Supported() {
		return &VersionError{Group: group, Version: s.Version, supported: Supported}
	}
	return nil
}

// WriteReference writes the PowerDNS section of the supported versions page,
// docs/reference/supported-versions.md, from Supported.
func WriteReference(w io.Writer) error {
	var b strings.Builder
	p := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }
	p("\n## PowerDNS\n\n")
	p("nbpdns reads each server group's primary through the HTTP API of these\n")
	p("releases of the PowerDNS Authoritative Server. The development lab runs each\n")
	p("one as the primary of a server group, and the integration tests read from\n")
	p("every one.\n\n")
	p("| PowerDNS Authoritative Server |\n|---|\n")
	for _, r := range Supported {
		p("| %s.x |\n", r)
	}
	p("\nBefore it reads, each command asks the primary what it is:\n\n")
	p("- If it isn't an authoritative server, such as a PowerDNS Recursor, the\n")
	p("  command fails.\n")
	p("- If its release isn't in the table, the command logs a warning and carries\n")
	p("  on, since reading may still work. `nbpdns powerdns check` reports it as a\n")
	p("  failed check.\n\n")
	p("nbpdns reads with the server's API key, sent in the `X-API-Key` header.\n")
	_, err := io.WriteString(w, b.String())
	return err
}
