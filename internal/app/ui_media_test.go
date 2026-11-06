//go:build darwin || windows

package app

import (
	"bytes"
	"image"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"live-mygo/internal/platforms"
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

func TestSystemMediaArtworkTracksCurrentRoomAndOwner(t *testing.T) {
	thumbnail := func(width int) coverThumbnail {
		data := encodeCover(image.NewRGBA(image.Rect(0, 0, width, 12)))
		bitmap, err := ui.DecodeBitmap(data)
		if err != nil {
			t.Fatal(err)
		}
		return coverThumbnail{bitmap: bitmap, data: data}
	}
	root, child := polishApp(t), polishApp(t)
	child.parent = root
	root.mediaCommandsOff = func() {}
	defer mygo.NowPlaying.Clear()
	root.shown, child.shown = true, true
	root.title, child.title = "主窗口", "小窗"
	root.targetRoom = platforms.Room{Platform: platforms.Douyu, ID: "1", Cover: "https://example.test/root.png"}
	child.targetRoom = platforms.Room{Platform: platforms.Douyu, ID: "2", Cover: "https://example.test/child.png"}
	root.publishMediaInfo()
	info, showing := mygo.NowPlaying.Info()
	if !showing || !bytes.Equal(info.Artwork, appIconPNG) {
		t.Fatal("封面未就绪时没有回退到应用图标")
	}
	rootArt, childArt := thumbnail(20), thumbnail(28)
	root.mediaOwner = child
	child.publishMediaInfo()
	root.finishCover(root.targetRoom.Cover, rootArt)
	info, _ = mygo.NowPlaying.Info()
	if info.Title != child.title || !bytes.Equal(info.Artwork, appIconPNG) {
		t.Fatal("非当前窗口的迟到封面覆盖了媒体信息")
	}
	child.stats.Paused = true
	child.finishCover(child.targetRoom.Cover, childArt)
	info, _ = mygo.NowPlaying.Info()
	if info.Title != child.title || !bytes.Equal(info.Artwork, childArt.data) || info.State != mygo.PlaybackPaused {
		t.Fatal("封面更新未保留当前窗口或暂停状态")
	}
	child.shown = false
	child.updateSystemPlayback()
	child.finishCover(child.targetRoom.Cover, childArt)
	if _, showing := mygo.NowPlaying.Info(); showing {
		t.Fatal("停止后迟到的封面重新发布了媒体会话")
	}
	root.mediaOwner = nil
	root.publishMediaInfo()
	info, showing = mygo.NowPlaying.Info()
	if !showing || !bytes.Equal(info.Artwork, rootArt.data) {
		t.Fatal("主窗口接回音频时未复用已经下载的封面")
	}
}
