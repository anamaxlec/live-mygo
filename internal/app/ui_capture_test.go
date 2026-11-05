//go:build darwin || windows

package app

import (
	"strings"
	"testing"
	"time"
)

func TestCaptureFilenameKeepsRoomIdentityAndRemovesPathCharacters(t *testing.T) {
	now := time.Date(2026, 11, 5, 11, 22, 33, 0, time.Local)
	got := captureFilename("  6657/今天:比赛\n", ".mkv", now)
	if strings.ContainsAny(got, "/:\n") || !strings.HasPrefix(got, "6657-今天-比赛-") || !strings.HasSuffix(got, "20261105-112233.mkv") {
		t.Fatal(got)
	}
	if got := captureFilename("..", ".png", now); !strings.HasPrefix(got, "LiveMyGo-") {
		t.Fatal(got)
	}
}
