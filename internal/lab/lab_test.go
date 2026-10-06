package lab

import (
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestHostOf(t *testing.T) {
	tests := []struct {
		dockerHost string
		want       string
	}{
		{"", "localhost"},
		{"unix:///var/run/docker.sock", "localhost"},
		{"tcp://docker:2375", "docker"},
		{"tcp://10.1.2.3:2376", "10.1.2.3"},
		{"tcp://[fd00::1]:2375", "fd00::1"},
		{"ssh://user@remote", "localhost"},
		{"::not a url", "localhost"},
	}
	for _, tt := range tests {
		if got := hostOf(tt.dockerHost); got != tt.want {
			t.Errorf("hostOf(%q) = %q, want %q", tt.dockerHost, got, tt.want)
		}
	}
}

func TestURL(t *testing.T) {
	t.Setenv("DOCKER_HOST", "tcp://docker:2375")
	if got := NetBoxes[0].URL(); got != "http://docker:8047" {
		t.Errorf("URL() = %q", got)
	}
	t.Setenv("DOCKER_HOST", "tcp://[fd00::1]:2375")
	if got := NetBoxes[0].URL(); got != "http://[fd00::1]:8047" {
		t.Errorf("URL() = %q", got)
	}
}

func TestPowerDNSURL(t *testing.T) {
	t.Setenv("DOCKER_HOST", "tcp://docker:2375")
	for _, p := range PowerDNSes {
		if got, want := p.URL(), "http://docker:"+strconv.Itoa(p.Port); got != want {
			t.Errorf("%s: URL() = %q, want %q", p.Name, got, want)
		}
	}
}

func TestPowerDNSFixtureNames(t *testing.T) {
	f := DescribePowerDNSFixture("t0123abcd")
	for _, s := range f.RRsets {
		if !strings.HasSuffix(s.Name, f.Zone) {
			t.Errorf("%s %s isn't in %s", s.Name, s.Type, f.Zone)
		}
	}
	if len(f.Others) != 2 || f.RRsets[0].Type != "SOA" {
		t.Errorf("fixture = %+v", f)
	}
}

func TestDriftFixtureNames(t *testing.T) {
	f := DescribeDriftFixture("t0123abcd")
	zones := []string{f.InSync, f.Drift, f.Missing, f.Parked, f.Ignored, f.Unmanaged}
	for _, z := range zones {
		if !strings.Contains(z, f.ID) || !strings.HasSuffix(z, ".") {
			t.Errorf("zone %q isn't absolute, or doesn't contain the ID", z)
		}
		if strings.HasSuffix(f.Nameserver, "."+z) {
			t.Errorf("name server %s is inside %s", f.Nameserver, z)
		}
	}
	if slices.Sort(zones); len(slices.Compact(zones)) != 6 {
		t.Errorf("zones share a name: %v", zones)
	}
}
