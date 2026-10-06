package version

import (
	"runtime/debug"
	"testing"
)

func TestFromBuildInfo(t *testing.T) {
	base := Info{Version: "(devel)", GoVersion: "go1.0", Platform: "linux/amd64"}
	tests := []struct {
		name string
		bi   debug.BuildInfo
		want Info
	}{
		{
			name: "no module version or settings",
			bi:   debug.BuildInfo{},
			want: base,
		},
		{
			name: "release with VCS settings",
			bi: debug.BuildInfo{
				GoVersion: "go1.27.1",
				Main:      debug.Module{Version: "v1.2.3"},
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: "abc123"},
					{Key: "vcs.time", Value: "2026-09-29T10:00:00Z"},
					{Key: "vcs.modified", Value: "false"},
					{Key: "CGO_ENABLED", Value: "0"},
				},
			},
			want: Info{Version: "v1.2.3", Commit: "abc123", CommitTime: "2026-09-29T10:00:00Z", GoVersion: "go1.27.1", Platform: "linux/amd64"},
		},
		{
			name: "modified working tree",
			bi: debug.BuildInfo{
				Main:     debug.Module{Version: "v0.0.0-20260929100000-abc123abc123+dirty"},
				Settings: []debug.BuildSetting{{Key: "vcs.modified", Value: "true"}},
			},
			want: Info{Version: "v0.0.0-20260929100000-abc123abc123+dirty", Modified: true, GoVersion: "go1.0", Platform: "linux/amd64"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fromBuildInfo(&tt.bi, base); got != tt.want {
				t.Errorf("fromBuildInfo() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
