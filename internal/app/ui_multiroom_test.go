//go:build darwin || windows

package app

import "testing"

func TestChoosingAudioMutesOtherWindowsWithoutStoppingThem(t *testing.T) {
	root := polishApp(t)
	first := polishApp(t)
	second := polishApp(t)
	first.parent, second.parent = root, root
	root.extraWindows = []*app{first, second}
	root.shown, first.shown, second.shown = true, true, true
	first.claimAudio()
	if root.mediaOwner != first || !root.stats.Muted || !second.stats.Muted || !root.shown || !second.shown {
		t.Fatal("切换主音轨停止了其他直播或没有静音")
	}
	if len(first.allPlaybackWindows()) != 3 {
		t.Fatal("子窗口没有共享资源预算")
	}
}
