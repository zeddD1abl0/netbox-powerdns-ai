package netbox

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/lab"
)

func TestStatusCheck(t *testing.T) {
	tests := []struct {
		netbox, plugin string
		want           string // part of the error, or "" for supported
	}{
		{"4.7.1", "1.7.2", ""},
		{"v4.7.1", "1.7.2", ""},
		{"4.6.10", "1.6.1", "NetBox 4.6.10 with the DNS plugin 1.6.1 isn't supported; nbpdns supports NetBox 4.7.x, with the plugin 1.7.x"},
		{"4.7.1", "1.6.1", "isn't supported"},
		{"4.7.1", "1.8.0", "isn't supported"},
		{"5.0.0", "1.7.2", "isn't supported"},
		{"4.7", "", "NetBox 4.7 doesn't have the NetBox DNS plugin installed"},
		{"", "", "doesn't have the NetBox DNS plugin installed"},
	}
	for _, tt := range tests {
		s := &Status{NetBoxVersion: tt.netbox, Plugins: map[string]string{"other_plugin": "1.7.2"}}
		if tt.plugin != "" {
			s.Plugins[PluginName] = tt.plugin
		}
		err := s.Check()
		if (s.NetBoxSupported() && s.PluginSupported()) != (err == nil) {
			t.Errorf("NetBox %s, plugin %s: NetBoxSupported and PluginSupported disagree with Check: %v", tt.netbox, tt.plugin, err)
		}
		var ve *VersionError
		switch {
		case tt.want == "" && err != nil:
			t.Errorf("NetBox %s, plugin %s: %v", tt.netbox, tt.plugin, err)
		case tt.want != "" && (!errors.As(err, &ve) || !strings.Contains(err.Error(), tt.want)):
			t.Errorf("NetBox %s, plugin %s: %v, want a VersionError containing %q", tt.netbox, tt.plugin, err, tt.want)
		case tt.want != "" && ve.Plugin != tt.plugin:
			t.Errorf("VersionError.Plugin = %q, want %q", ve.Plugin, tt.plugin)
		}
	}
}

// TestStatusCheckReleases checks a status against a list of several
// releases, as Supported was and will be again (ADR-0023).
func TestStatusCheckReleases(t *testing.T) {
	releases := []Release{{NetBox: "4.7", Plugin: "1.7"}, {NetBox: "4.6", Plugin: "1.6"}}
	tests := []struct {
		netbox, plugin string
		want           string // part of the error, or "" for supported
	}{
		{"4.7.1", "1.7.2", ""},
		{"4.6.10", "1.6.1", ""},
		{"4.7.0", "1.6.1", ""}, // each is checked on its own
		{"4.5.3", "1.5.0", "nbpdns supports NetBox 4.7.x or 4.6.x, with the plugin 1.7.x or 1.6.x"},
		{"4.6.10", "1.8.0", "isn't supported"},
	}
	for _, tt := range tests {
		s := &Status{NetBoxVersion: tt.netbox, Plugins: map[string]string{PluginName: tt.plugin}}
		err := s.check(releases)
		if (err == nil) != (tt.want == "") || (err != nil && !strings.Contains(err.Error(), tt.want)) {
			t.Errorf("NetBox %s, plugin %s: %v, want an error containing %q", tt.netbox, tt.plugin, err, tt.want)
		}
	}
}

// TestSupportedMatchesLab checks that the lab runs every supported release,
// and nothing else, so the integration tests cover exactly them.
func TestSupportedMatchesLab(t *testing.T) {
	var lab2 []Release
	for _, nb := range lab.NetBoxes {
		lab2 = append(lab2, Release{NetBox: nb.Version, Plugin: nb.PluginVersion})
	}
	if !slices.Equal(Supported, lab2) {
		t.Errorf("Supported = %v, but the lab runs %v", Supported, lab2)
	}
}
