package lab

import "testing"

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
	if got := NetBoxes[1].URL(); got != "http://[fd00::1]:8046" {
		t.Errorf("URL() = %q", got)
	}
}
