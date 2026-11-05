//go:build darwin || windows

package app

import (
	"github.com/egoist/mygo"
	"live-mygo/internal/platforms"
	"testing"
)

func TestLiveMediaInfoHasNoSeekOrFictionalDuration(t *testing.T) {
	a := polishApp(t)
	a.title = "当前直播"
	a.targetRoom = platforms.Room{Platform: platforms.Douyu, ID: "1", Anchor: "主播"}
	info := a.mediaInfo()
	if info.Title != "当前直播" || info.Artist != "主播" || info.Duration != 0 || info.Commands&mygo.MediaSeek != 0 {
		t.Fatal("系统把直播当成了可拖动进度的点播", info)
	}
	a.shown = true
	a.handleMediaCommand(mygo.MediaPause)
	if !a.stats.Paused {
		t.Fatal("系统暂停命令没有进入播放器")
	}
	a.handleMediaCommand(mygo.MediaPlay)
	if a.stats.Paused {
		t.Fatal("系统播放命令没有进入播放器")
	}
}
