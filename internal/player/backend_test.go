//go:build darwin || windows

package player

import "testing"

func TestBackendSelectionKeepsCompatibilityAndExplicitChoice(t *testing.T) {
	t.Setenv("LIVE_MYGO_PLAYER", "")
	if got := selectedBackend(nil); got != "mpv" {
		t.Fatal(got)
	}
	if got := selectedBackend([]string{"unknown"}); got != "mpv" {
		t.Fatal(got)
	}
	want := "mpv"
	if NativeBackendAvailable() {
		want = "vt"
	}
	if got := selectedBackend([]string{"vt"}); got != want {
		t.Fatal(got)
	}
	t.Setenv("LIVE_MYGO_PLAYER", "mpv")
	if got := selectedBackend([]string{"vt"}); got != "mpv" {
		t.Fatal(got)
	}
}
