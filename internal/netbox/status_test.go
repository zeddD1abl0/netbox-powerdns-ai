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
		{"4.6.10", "1.6.1", ""},
		{"4.7.0", "1.6.1", ""}, // each is checked on its own
		{"v4.7.1", "1.7.2", ""},
		{"4.5.3", "1.5.0", "NetBox 4.5.3 with the DNS plugin 1.5.0 isn't supported; nbpdns supports NetBox 4.7.x or 4.6.x, with the plugin 1.7.x or 1.6.x"},
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

func TestZoneName(t *testing.T) {
	tests := []struct{ in, want, wantErr string }{
		{"example.com", "example.com", ""},
		{"Example.COM.", "example.com", ""},
		{" example.com ", "example.com", ""},
		{"xn--bcher-kva.example", "xn--bcher-kva.example", ""},
		{"bücher.example", "", "ASCII form"},
		{"exa mple.com", "", "ASCII form"},
		{".", "", "empty"},
		{"", "", "empty"},
	}
	for _, tt := range tests {
		got, err := ZoneName(tt.in)
		if got != tt.want || (err == nil) != (tt.wantErr == "") || (err != nil && !strings.Contains(err.Error(), tt.wantErr)) {
			t.Errorf("ZoneName(%q) = %q, %v; want %q, error %q", tt.in, got, err, tt.want, tt.wantErr)
		}
	}
}
