// Package buildinfo identifies the running console build.
package buildinfo

import "runtime/debug"

// Version and Commit are set at link time:
//
//	-ldflags "-X github.com/kubebao/openbao-operator-console/internal/buildinfo.Version=v1.2.3
//	          -X github.com/kubebao/openbao-operator-console/internal/buildinfo.Commit=0123abcd"
var (
	Version = "dev"
	Commit  = ""
)

const product = "openbao-operator-console"

// Current returns the build version and commit. A development build without link-time
// values reports the module version and VCS revision that the Go toolchain embedded.
func Current() (version, commit string) {
	if Version != "dev" && Commit != "" {
		return Version, Commit
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return Version, Commit
	}
	return fromBuildInfo(Version, Commit, info)
}

func fromBuildInfo(version, commit string, info *debug.BuildInfo) (string, string) {
	if version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = info.Main.Version
	}
	if commit != "" {
		return version, commit
	}
	modified := false
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			commit = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	if commit != "" && modified {
		commit += "-dirty"
	}
	return version, commit
}

// UserAgent identifies the console in requests to Kubernetes API servers and identity
// providers. A development build includes a short commit so that requests stay traceable.
func UserAgent() string {
	version, commit := Current()
	if version == "dev" && commit != "" {
		version += "+" + commit[:min(12, len(commit))]
	}
	return product + "/" + version
}
