package netbox

import (
	"context"
	"errors"
	"slices"
	"strings"
)

// PluginName is the NetBox DNS plugin's name in NetBox's status.
const PluginName = "netbox_dns"

// A Release is a NetBox release series, such as 4.7, and the DNS plugin's
// release series built for it, such as 1.7.
type Release struct {
	NetBox string
	Plugin string
}

// Supported lists the releases nbpdns supports (REQ-040), newest first. The
// lab runs each one (internal/lab), and the integration tests read from it.
//
// NetBox and the plugin are each checked against this list on their own:
// NetBox won't load a plugin release that doesn't support it, so a running
// pair is always one the plugin supports.
var Supported = []Release{
	{NetBox: "4.7", Plugin: "1.7"},
	{NetBox: "4.6", Plugin: "1.6"},
}

// SupportedSeries returns the supported NetBox and plugin release series, as
// text such as "4.7.x or 4.6.x".
func SupportedSeries() (netBox, plugin string) { return seriesText(Supported) }

func seriesText(releases []Release) (netBox, plugin string) {
	var nb, pl []string
	for _, r := range releases {
		nb, pl = append(nb, r.NetBox+".x"), append(pl, r.Plugin+".x")
	}
	return strings.Join(nb, " or "), strings.Join(pl, " or ")
}

// Status is what NetBox reports about itself at /api/status/.
type Status struct {
	// NetBoxVersion is NetBox's version, such as 4.7.1.
	NetBoxVersion string `json:"netbox-version"`
	// Plugins maps each installed plugin's name to its version.
	Plugins map[string]string `json:"plugins"`
}

// PluginVersion returns the DNS plugin's version, or "" if it isn't
// installed.
func (s *Status) PluginVersion() string { return s.Plugins[PluginName] }

// Check returns a *VersionError if the DNS plugin isn't installed, or if
// NetBox or the plugin isn't a supported release.
func (s *Status) Check() error { return s.check(Supported) }

// NetBoxSupported reports whether NetBox is a supported release.
func (s *Status) NetBoxSupported() bool { return s.netBoxSupported(Supported) }

// PluginSupported reports whether the DNS plugin is installed, in a
// supported release.
func (s *Status) PluginSupported() bool { return s.pluginSupported(Supported) }

func (s *Status) netBoxSupported(supported []Release) bool {
	nb := series(s.NetBoxVersion)
	return slices.ContainsFunc(supported, func(r Release) bool { return r.NetBox == nb })
}

func (s *Status) pluginSupported(supported []Release) bool {
	plugin := series(s.PluginVersion())
	return s.PluginVersion() != "" && slices.ContainsFunc(supported, func(r Release) bool { return r.Plugin == plugin })
}

func (s *Status) check(supported []Release) error {
	if !s.netBoxSupported(supported) || !s.pluginSupported(supported) {
		return &VersionError{NetBox: s.NetBoxVersion, Plugin: s.PluginVersion(), supported: supported}
	}
	return nil
}

// series returns a version's release series: its first two numbers, such as
// 4.7 for 4.7.1.
func series(version string) string {
	parts := strings.SplitN(strings.TrimPrefix(version, "v"), ".", 3)
	if len(parts) < 2 {
		return version
	}
	return parts[0] + "." + parts[1]
}

// Status reads NetBox's status.
func (c *Client) Status(ctx context.Context) (*Status, error) {
	var s Status
	if err := c.get(ctx, c.endpoint("api/status/", nil), &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Connect reads NetBox's status and checks its releases, before a command
// reads from NetBox. A missing DNS plugin is an error. An unsupported release
// only logs a warning, since reading may still work (ADR-0020).
func (c *Client) Connect(ctx context.Context) (*Status, error) {
	s, err := c.Status(ctx)
	if err != nil {
		return nil, err
	}
	var ve *VersionError
	if err := s.check(c.supported); errors.As(err, &ve) {
		if ve.Plugin == "" {
			return nil, err
		}
		c.log.WarnContext(ctx, "this NetBox release isn't supported, so reading from it may fail or give wrong results",
			"err", err)
	}
	return s, nil
}
