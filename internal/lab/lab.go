// Package lab describes the development lab, deploy/dev/compose.yaml, for
// integration tests: where each NetBox and PowerDNS server listens, and how to
// reach it as its admin. `make test-integration` starts the lab before it runs the tests,
// which have the build tag "integration".
package lab

import (
	"net"
	"net/url"
	"os"
	"strconv"
)

// AdminToken is the v2 API token of the superuser in every lab NetBox. Tests
// use it to create fixtures, never to read them back.
const AdminToken = "nbt_nbpdnslabadm.nbpdnsLabAdminTokenNotForProduction00000" //nolint:gosec // The lab's published credential. gitleaks:allow

// A NetBox is one of the lab's NetBox instances.
type NetBox struct {
	// Name is its compose service.
	Name string
	// Version is the NetBox release series it runs, such as 4.7.
	Version string
	// PluginVersion is the NetBox DNS plugin's release series, such as 1.7.
	PluginVersion string
	// Port is the port it's published on, on the lab's host.
	Port int
}

// NetBoxes are the lab's NetBox instances, one per supported version
// (REQ-040). Each one costs the CI job a NetBox and a PostgreSQL server, so
// a version is only added when the runners have room for it (ADR-0023).
var NetBoxes = []NetBox{
	{Name: "netbox-47", Version: "4.7", PluginVersion: "1.7", Port: 8047},
}

// URL returns the instance's base URL.
func (n NetBox) URL() string {
	return "http://" + net.JoinHostPort(Host(), strconv.Itoa(n.Port))
}

// PowerDNSAPIKey is the API key of every lab PowerDNS server. PowerDNS has
// one key per server, so tests create fixtures with the key that nbpdns
// reads with.
const PowerDNSAPIKey = "nbpdns-lab-powerdns-api-key-not-for-production" //nolint:gosec // The lab's published credential. gitleaks:allow

// A PowerDNS is one of the lab's PowerDNS servers: the primary of one server
// group (ADR-0024).
type PowerDNS struct {
	// Name is its compose service.
	Name string
	// Group is the server group it's the primary of.
	Group string
	// Version is the PowerDNS release series it runs, such as 5.1.
	Version string
	// Port is its API's port on the lab's host.
	Port int
}

// PowerDNSes are the lab's PowerDNS servers, one per supported release
// (REQ-041), each the primary of its own group.
var PowerDNSes = []PowerDNS{
	{Name: "powerdns-51", Group: "lab-a", Version: "5.1", Port: 8151},
	{Name: "powerdns-50", Group: "lab-b", Version: "5.0", Port: 8150},
}

// URL returns the server's API base URL.
func (p PowerDNS) URL() string {
	return "http://" + net.JoinHostPort(Host(), strconv.Itoa(p.Port))
}

// Host returns the host that the lab's ports are published on: the host of
// a tcp:// DOCKER_HOST, as with Docker-in-Docker in CI, or else localhost.
func Host() string {
	return hostOf(os.Getenv("DOCKER_HOST"))
}

func hostOf(dockerHost string) string {
	u, err := url.Parse(dockerHost)
	if err == nil && u.Scheme == "tcp" && u.Hostname() != "" {
		return u.Hostname()
	}
	return "localhost"
}
