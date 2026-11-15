package version

import (
	"runtime/debug"
	"testing"
)

func TestString_ReleaseVersionWins(t *testing.T) {
	orig := version
	version = "1.1.0"
	t.Cleanup(func() { version = orig })
	if got := String(); got != "1.1.0" {
		t.Errorf("String() = %q, want the build-time version 1.1.0", got)
	}
}

func TestDevVersion(t *testing.T) {
	settings := func(kv ...string) *debug.BuildInfo {
		info := &debug.BuildInfo{}
		for i := 0; i < len(kv); i += 2 {
			info.Settings = append(info.Settings, debug.BuildSetting{Key: kv[i], Value: kv[i+1]})
		}
		return info
	}
	cases := []struct {
		name string
		info *debug.BuildInfo
		ok   bool
		want string
	}{
		{"no build info", nil, false, "dev"},
		{"no vcs info (go run, go test)", settings(), true, "dev"},
		{"clean checkout", settings("vcs.revision", "a70b97c1234567890abcdef", "vcs.modified", "false"), true, "dev+a70b97c12345"},
		{"uncommitted changes", settings("vcs.revision", "a70b97c1234567890abcdef", "vcs.modified", "true"), true, "dev+a70b97c12345-dirty"},
		{"short revision", settings("vcs.revision", "abc123"), true, "dev+abc123"},
	}
	for _, c := range cases {
		if got := devVersion(c.info, c.ok); got != c.want {
			t.Errorf("%s: devVersion = %q, want %q", c.name, got, c.want)
		}
	}
}
