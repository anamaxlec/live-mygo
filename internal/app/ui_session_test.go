//go:build darwin || windows

package app

import (
	"live-mygo/internal/library"
	"live-mygo/internal/platforms"
	"testing"
)

func TestResumeRequiresOptInAndExplicitLaunchWins(t *testing.T) {
	a := polishApp(t)
	a.library.LastSession = &library.PlaybackSession{Room: platforms.Room{Platform: platforms.Douyu, ID: "6979222"}, Quality: 3, Source: "line2"}
	if a.sessionToResume() != nil {
		t.Fatal("未开启恢复就自动播放")
	}
	a.library.Preferences.ResumeSession = true
	if session := a.sessionToResume(); session == nil || session.Source != "line2" || session.Quality != 3 {
		t.Fatal("没有保留房间清晰度与线路")
	}
	a.opts.room = "6"
	if a.sessionToResume() != nil {
		t.Fatal("恢复会话覆盖了明确的启动房间")
	}
}
