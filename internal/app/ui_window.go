//go:build darwin || windows

package app

import (
	"math"
	"time"

	"github.com/egoist/mygo"
)

func (a *app) toggleFollowVideo() {
	if a.library == nil {
		return
	}
	next := a.library.Preferences
	next.FollowVideo = !next.FollowVideo
	a.savePreferences(next)
	if !next.FollowVideo && a.win != nil {
		a.normalMinimum()
	}
	a.fitWindowToVideo(true)
	a.revealControls()
}

// 固定界面占位在视频比例之外；不能直接将整个窗口锁为 16:9。
func fittedWindow(width, height int, aspect float64, side, header, maxWidth, maxHeight int, preferWidth bool) (int, int) {
	if aspect <= 0 {
		return width, height
	}
	if preferWidth {
		height = int(math.Round(float64(max(1, width-side))/aspect)) + header
	} else {
		width = int(math.Round(float64(max(1, height-header))*aspect)) + side
	}
	if width < 680 {
		width = 680
		height = int(math.Round(float64(width-side)/aspect)) + header
	}
	if height < 360 {
		height = 360
		width = int(math.Round(float64(height-header)*aspect)) + side
	}
	if height > maxHeight {
		height = maxHeight
		width = int(math.Round(float64(max(1, height-header))*aspect)) + side
	}
	if width > maxWidth {
		width = maxWidth
		height = int(math.Round(float64(max(1, width-side))/aspect)) + header
	}
	return width, height
}

func (a *app) fitWindowToVideo(preferWidth bool) {
	if a.win == nil || a.closing || a.aspectBusy || a.companion {
		return
	}
	current := a.win.ContentBounds()
	if a.page != "player" || !a.shown || a.library == nil || !a.library.Preferences.FollowVideo || a.win.IsFullScreen() || a.stats.Width <= 0 || a.stats.Height <= 0 {
		a.aspectW, a.aspectH = current.Width, current.Height
		return
	}
	if !preferWidth && a.aspectW > 0 {
		dw, dh := abs(current.Width-a.aspectW), abs(current.Height-a.aspectH)
		preferWidth = dw > dh
	}
	side := 0
	side += int(2 * (watchFrameInset + watchFrameBorder))
	if a.chatVisible() {
		side += int(a.preferredChatWidth())
	}
	if !a.focusMode {
		side += int(sidebarWidth)
	}
	display := mygo.Screen.DisplayMatching(a.win.Bounds()).WorkArea
	maxWidth, maxHeight := display.Width-16, display.Height-16
	if maxWidth <= 0 || maxHeight <= 0 {
		maxWidth, maxHeight = 1920, 1080
	}
	header := int(watchHeaderHeight + 2*(watchFrameInset+watchFrameBorder))
	width, height := fittedWindow(current.Width, current.Height, float64(a.stats.Width)/float64(a.stats.Height), side, header, maxWidth, maxHeight, preferWidth)
	a.aspectW, a.aspectH = width, height
	if abs(width-current.Width) <= 1 && abs(height-current.Height) <= 1 {
		return
	}
	a.aspectBusy = true
	defer func() { a.aspectBusy = false }()
	a.win.SetMinimumSize(680, 360)
	current.Width, current.Height = width, height
	if display.Width > 0 {
		current.X = max(display.X+8, min(current.X, display.X+display.Width-width-8))
		current.Y = max(display.Y+8, min(current.Y, display.Y+display.Height-height-8))
	}
	a.win.SetContentBounds(current)
}
func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

// 拖动过程中不再反复改写系统窗口尺寸，在缩放停下后一次贴合比例。
func (a *app) windowResized() {
	if a.aspectBusy || a.closing || a.win == nil || a.companion {
		return
	}
	if a.library == nil || !a.library.Preferences.FollowVideo || a.page != "player" {
		a.aspectW, a.aspectH = a.win.ContentSize()
		return
	}
	if a.aspectTimer != nil {
		a.aspectTimer.Stop()
	}
	a.resizeDeadline = time.Now().Add(160 * time.Millisecond)
	a.aspectTimer = time.AfterFunc(160*time.Millisecond, func() {
		a.win.Update(func() {
			if !a.closing && !time.Now().Before(a.resizeDeadline) {
				a.fitWindowToVideo(false)
			}
		})
	})
}
