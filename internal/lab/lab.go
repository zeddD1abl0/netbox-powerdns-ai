// Package lab describes the development lab, deploy/dev/compose.yaml, for
// integration tests: where each NetBox listens, and how to reach it as its
// admin. `make test-integration` starts the lab before it runs the tests,
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
