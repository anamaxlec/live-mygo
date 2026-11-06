//go:build windows && (amd64 || arm64)

package app

import (
	"image"

	"github.com/egoist/mygo/ui"
	"live-mygo/internal/player"
)

func (a *app) captureReturnFrame() bool { return false }

// takeVideoFrame 在 UI 线程取走最新一帧，并释放上一帧缓冲。
func (a *app) takeVideoFrame() {
	if a.surface == nil {
		return
	}
	source, ok := a.surface.(player.FrameSource)
	if !ok {
		return
	}
	pix, w, h, stride, ok := source.TakeFrame()
	if !ok {
		return
	}
	img := &image.RGBA{Pix: pix, Stride: stride, Rect: image.Rect(0, 0, w, h)}
	if a.videoBitmap == nil {
		a.videoBitmap = ui.NewBitmap(img)
	} else {
		a.videoBitmap.Update(img)
	}
	if a.videoPix != nil {
		source.ReleaseFrame(a.videoPix)
	}
	a.videoPix = pix
}
