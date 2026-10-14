package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestDevelopmentBuildUsesEmbeddedRevision(t *testing.T) {
	info := &debug.BuildInfo{
		Main: debug.Module{Version: "(devel)"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "0123456789abcdef0123"},
			{Key: "vcs.modified", Value: "true"},
		},
	}
	version, commit := fromBuildInfo("dev", "", info)
	if version != "dev" || commit != "0123456789abcdef0123-dirty" {
		t.Fatalf("development build = %q %q", version, commit)
	}
	version, commit = fromBuildInfo("v1.2.3", "release", info)
	if version != "v1.2.3" || commit != "release" {
		t.Fatal("link-time version or commit was replaced")
	}
	version, _ = fromBuildInfo("dev", "", &debug.BuildInfo{Main: debug.Module{Version: "v0.4.0"}})
	if version != "v0.4.0" {
		t.Fatal("module version was not used for a development build")
	}
}

func TestUserAgentNamesTheBuild(t *testing.T) {
	version, commit := Version, Commit
	t.Cleanup(func() { Version, Commit = version, commit })
	Version, Commit = "v1.2.3", "0123abcd"
	if got := UserAgent(); got != "openbao-operator-console/v1.2.3" {
		t.Fatalf("user agent = %q", got)
	}
}
