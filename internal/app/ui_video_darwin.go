//go:build darwin

package app

import (
	"image/png"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo/ui"
	"live-mygo/internal/player"
)

// 平时保留原生视频视图。返回列表时才在播放器线程取一帧给变形层。
func (a *app) takeVideoFrame() {}

func (a *app) captureReturnFrame() bool {
	if a.win == nil || a.player == nil || a.closing || a.page != "player" ||
		a.waitingForVideo() || a.audioOnly() || a.companion || a.win.IsFullScreen() ||
		(a.returnFrame != nil && !a.cardOpenAt.IsZero()) ||
		(a.stream == nil && a.targetRoom.ID == "") {
		return false
	}
	if a.returnCapture != nil {
		return true
	}

	win, generation := a.win, a.generation
	const wait = 250 * time.Millisecond
	expires := time.Now().Add(wait)
	var timer *time.Timer
	finish := func(frame *ui.Bitmap) { a.finishReturnCapture(timer, generation, frame) }
	timer = time.AfterFunc(wait, func() { win.Update(func() { finish(nil) }) })
	a.returnCapture = timer
	select {
	case a.playerOps <- func(p *player.Player) {
		if time.Now().After(expires) {
			return
		}
		frame := capturePlayerReturnFrame(p)
		win.Update(func() { finish(frame) })
	}:
		return true
	default:
		timer.Stop()
		a.returnCapture = nil
		return false
	}
}

func capturePlayerReturnFrame(p *player.Player) *ui.Bitmap {
	dir, err := os.MkdirTemp("", "live-mygo-return-")
	if err != nil {
		return nil
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "frame.png")
	if err := p.Command("screenshot-to-file", path, "video"); err != nil {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	img, err := png.Decode(file)
	if err != nil {
		return nil
	}
	return ui.NewBitmap(img)
}
