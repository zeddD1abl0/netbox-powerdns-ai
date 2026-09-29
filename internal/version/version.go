// Package version reports what build of nbpdns is running, from the build
// information the Go toolchain embeds in every binary.
package version

import (
	"runtime"
	"runtime/debug"
)

// Info describes a build of nbpdns.
type Info struct {
	// Version is the module version: a release tag such as v1.2.3, a
	// pseudo-version for an untagged commit, or "(devel)".
	Version string `json:"version"`
	// Commit is the VCS revision the binary was built from, if known.
	Commit string `json:"commit,omitempty"`
	// CommitTime is the commit's time, in RFC 3339 form, if known.
	CommitTime string `json:"commit_time,omitempty"`
	// Modified reports whether the working tree had uncommitted changes.
	Modified bool `json:"modified"`
	// GoVersion is the Go toolchain that built the binary.
	GoVersion string `json:"go_version"`
	// Platform is the operating system and architecture, such as linux/amd64.
	Platform string `json:"platform"`
}

// Get returns the running binary's build information.
func Get() Info {
	info := Info{Version: "(devel)", GoVersion: runtime.Version(), Platform: runtime.GOOS + "/" + runtime.GOARCH}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return info
	}
	return fromBuildInfo(bi, info)
}

// fromBuildInfo fills in info from bi.
func fromBuildInfo(bi *debug.BuildInfo, info Info) Info {
	if bi.Main.Version != "" {
		info.Version = bi.Main.Version
	}
	if bi.GoVersion != "" {
		info.GoVersion = bi.GoVersion
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			info.Commit = s.Value
		case "vcs.time":
			info.CommitTime = s.Value
		case "vcs.modified":
			info.Modified = s.Value == "true"
		}
	}
	return info
}
