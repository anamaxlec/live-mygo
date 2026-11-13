//go:build darwin || windows

package app

import (
	"runtime"
	"time"

	"github.com/egoist/mygo/ui"
)

// 沉浸时一并收起导航侧栏，退出时恢复进入前的状态。
func (a *app) toggleImmersive() {
	if a.page != "player" || a.companion || a.snap || a.win != nil && a.win.IsFullScreen() {
		return
	}
	if a.immersive {
		a.suspendImmersive()
	} else {
		a.immersive = true
		a.immersiveRestoreNav = !a.focusMode
		a.focusMode = true
	}
	if a.library != nil && a.parent == nil {
		next := a.library.Preferences
		next.Immersive = a.immersive
		a.savePreferences(next)
	}
	a.featuredViewer().closePlayerMenus()
	a.featuredViewer().revealControls()
	a.fitWindowToVideo(true)
}

// 离开播放页或进入小窗时只清除当前窗口的布局状态。
func (a *app) suspendImmersive() {
	if a.immersive && a.immersiveRestoreNav {
		a.focusMode = false
	}
	a.immersive, a.immersiveRestoreNav = false, false
}

// 主窗口再次进入播放页时恢复沉浸偏好。
func (a *app) restoreImmersive() {
	if a.immersive || a.parent != nil || a.snap || a.companion ||
		a.library == nil || !a.library.Preferences.Immersive {
		return
	}
	a.immersive = true
	a.immersiveRestoreNav = true
	a.focusMode = true
}

func (a *app) watchHeaderTarget() float32 {
	if a.immersive {
		return 0
	}
	return watchHeaderHeight
}

// 悬浮顶栏与播放控制条共享画面内的显隐和淡出进度。
func (a *app) immersiveHeader(c *ui.Context, viewer *app, opacity float32) {
	panel := floatingWatchControls(ui.Box(c.Key("immersive-header"))).Absolute().
		Left(12).Right(12).Top(transportBottom).Height(52).Opacity(opacity).Hidden(opacity < .005).
		Label("沉浸式浮动顶栏")
	if panel.Hovered() || viewer.controlsKeyboard && panel.FocusWithin() {
		viewer.controlsHeld = true
		viewer.controlsUntil = c.Now().Add(2800 * time.Millisecond)
	}
	rightPad := float32(18)
	if runtime.GOOS == "windows" {
		rightPad = max(18, titlebarRight(c, 12)-12)
	}
	panel.Children(func() {
		a.watchHeaderControls(c, viewer, 18, rightPad, 0, true)
	})
}

// 播放页和监控室共用系统窗口按钮的显隐状态。
func (a *app) syncWatchWindowButtons(hide bool) {
	if a.companion || hide == a.watchButtonsHidden {
		return
	}
	a.watchButtonsHidden = hide
	a.reapplyWindowButtons()
}

// MyGo 在聚焦和改变尺寸时会重排标题栏按钮，需要按当前模式再应用一次。
func (a *app) reapplyWindowButtons() {
	if a.surface == nil {
		return
	}
	hide := a.companion || a.watchButtonsHidden
	a.surface.SetWindowButtonsVisible(!hide)
	if !hide && a.win != nil && runtime.GOOS == "darwin" && !a.win.IsFullScreen() {
		// AppKit 会在重新显示按钮时重排标题栏，交给 MyGo 恢复窗口配置的坐标。
		a.win.SetTitle(a.win.Title())
	}
}
