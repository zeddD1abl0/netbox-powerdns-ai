package netbox

import (
	"fmt"
	"strings"
)

// UnreachableError means NetBox couldn't be reached, or didn't answer in
// time, after every retry.
type UnreachableError struct {
	URL string
	Err error
}

func (e *UnreachableError) Error() string {
	return fmt.Sprintf("NetBox at %s isn't reachable: %v", e.URL, e.Err)
}

func (e *UnreachableError) Unwrap() error { return e.Err }

// AuthError means NetBox rejected the token, or none was sent.
type AuthError struct {
	// Detail is NetBox's own explanation, such as "Invalid v2 token".
	Detail string
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("NetBox rejected the token (%s); check netbox.token", e.Detail)
}

// PermissionError means the token's user may not view an object type that
// nbpdns reads.
type PermissionError struct {
	// ObjectType is the NetBox object type, such as netbox_dns.zone.
	ObjectType string
}

func (e *PermissionError) Error() string {
	return fmt.Sprintf("the token's user can't view %s objects in NetBox; give it the view permission on the DNS plugin's objects", e.ObjectType)
}

// VersionError means NetBox, or its DNS plugin, isn't a supported release
// (REQ-040). Plugin is empty if the plugin isn't installed.
type VersionError struct {
	NetBox, Plugin string

	supported []Release
}

func (e *VersionError) Error() string {
	if e.Plugin == "" {
		return fmt.Sprintf("NetBox %s doesn't have the NetBox DNS plugin installed", e.NetBox)
	}
	var nb, plugin []string
	for _, r := range e.supported {
		nb, plugin = append(nb, r.NetBox+".x"), append(plugin, r.Plugin+".x")
	}
	return fmt.Sprintf("NetBox %s with the DNS plugin %s isn't supported; nbpdns supports NetBox %s, with the plugin %s",
		e.NetBox, e.Plugin, strings.Join(nb, " or "), strings.Join(plugin, " or "))
}

// AmbiguousZoneError means a zone name is in more than one view, and no view
// was named to choose between them.
type AmbiguousZoneError struct {
	Zone  string
	Views []string
}

func (e *AmbiguousZoneError) Error() string {
	return fmt.Sprintf("zone %s is in more than one view: %s; name the view", e.Zone, strings.Join(e.Views, ", "))
}

// APIError is any other error response from NetBox.
type APIError struct {
	Status int
	// Path is the request's path and query.
	Path string
	// Detail is NetBox's explanation, if it gave one.
	Detail string
}

func (e *APIError) Error() string {
	s := fmt.Sprintf("NetBox answered GET %s with status %d", e.Path, e.Status)
	if e.Detail != "" {
		s += ": " + e.Detail
	}
	return s
}
