//go:build windows && (amd64 || arm64)

package app

import (
	"image"

	"github.com/egoist/mygo/ui"
)

// takeVideoFrame 在 UI 线程取走最新一帧，并释放上一帧缓冲。
func (a *app) takeVideoFrame() {
	if a.surface == nil {
		return
	}
	pix, w, h, stride, ok := a.surface.TakeFrame()
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
		a.surface.ReleaseFrame(a.videoPix)
	}
	a.videoPix = pix
}
