package powerdns

import (
	"fmt"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/httpclient"
)

// UnreachableError means a primary couldn't be reached, or didn't answer in
// time, after every retry.
type UnreachableError = httpclient.UnreachableError

// AuthError means PowerDNS rejected the API key.
type AuthError struct {
	Group string
	// Detail is PowerDNS's own explanation, such as "Unauthorized".
	Detail string
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("PowerDNS rejected the API key of server group %s (%s); check %s.%s.primary.api_key",
		e.Group, e.Detail, config.GroupsKey, e.Group)
}

// ServerNotFoundError means the API has no server with the configured ID.
type ServerNotFoundError struct {
	Group, ServerID string
}

func (e *ServerNotFoundError) Error() string {
	return fmt.Sprintf("the PowerDNS API of server group %s has no server %s; check %s.%s.primary.server_id, which is usually localhost",
		e.Group, e.ServerID, config.GroupsKey, e.Group)
}

// NotAuthoritativeError means the server isn't a PowerDNS Authoritative
// Server, such as a Recursor.
type NotAuthoritativeError struct {
	Group string
	// DaemonType is what the server says it is, such as recursor.
	DaemonType string
}

func (e *NotAuthoritativeError) Error() string {
	return fmt.Sprintf("the primary of server group %s is a PowerDNS %s, not an authoritative server", e.Group, e.DaemonType)
}

// VersionError means the server isn't a supported release (REQ-041).
type VersionError struct {
	Group, Version string

	supported []string
}

func (e *VersionError) Error() string {
	return fmt.Sprintf("the primary of server group %s runs PowerDNS %s, which isn't supported; nbpdns supports PowerDNS %s",
		e.Group, e.Version, seriesText(e.supported))
}

// ZoneNotFoundError means the server has no zone of that name.
type ZoneNotFoundError struct {
	Group, Zone string
}

func (e *ZoneNotFoundError) Error() string {
	return fmt.Sprintf("the primary of server group %s has no zone %s", e.Group, e.Zone)
}

// APIError is any other error response from the API.
type APIError struct {
	Group  string
	Status int
	// Path is the request's path and query.
	Path string
	// Detail is PowerDNS's explanation, if it gave one.
	Detail string
}

func (e *APIError) Error() string {
	s := fmt.Sprintf("the PowerDNS API of server group %s answered GET %s with status %d", e.Group, e.Path, e.Status)
	if e.Detail != "" {
		s += ": " + e.Detail
	}
	return s
}
