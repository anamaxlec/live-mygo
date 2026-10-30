//go:build darwin

package app

import (
	"github.com/egoist/mygo/ui"
	"live-mygo/internal/library"
	"time"
)

func (a *app) customAppearance() bool {
	return a.library != nil && a.library.Preferences.Appearance != "system" && a.library.Preferences.Appearance != ""
}

func (a *app) applyTheme(c *ui.Context) {
	if !a.customAppearance() {
		return
	}
	base := c.Theme()
	t := ui.LightTheme()
	if a.library.Preferences.Appearance == "dark" {
		t = ui.DarkTheme()
	}
	t.Accent, t.AccentHover, t.AccentPressed, t.Focus = base.Accent, base.AccentHover, base.AccentPressed, base.Focus
	c.SetTheme(t)
}

func (a *app) savePreferences(next library.Preferences) {
	if a.library == nil {
		return
	}
	before := a.library.Preferences
	if err := a.library.SetPreferences(next); err != nil {
		a.toast = "偏好设置未能保存"
		if a.win != nil {
			a.win.Invalidate()
		}
		return
	}
	if a.preferencesTimer != nil {
		a.preferencesTimer.Stop()
		a.preferencesTimer = nil
	}
	a.preferencesDeadline = time.Time{}
	if a.client != nil && before.DouyuLegacyTLS != next.DouyuLegacyTLS {
		a.client.SetDouyuChatCompatibility(next.DouyuLegacyTLS)
		if a.stream != nil && a.stream.Room.Platform == "douyu" {
			a.startChat(true)
		}
	}
	a.updateCaptionPreferences(before, next)
	if a.win != nil {
		a.win.Invalidate()
	}
	if a.accountWin != nil && !a.accountWin.IsDestroyed() {
		a.accountWin.Invalidate()
	}
	if a.filterWin != nil && !a.filterWin.IsDestroyed() {
		a.filterWin.Invalidate()
	}
}

func (a *app) leavePlayer() {
	if a.page == "player" && a.busy {
		a.generation++
		if a.resolveCancel != nil {
			a.resolveCancel()
		}
		a.busy = false
	}
	if a.page == "player" && a.library != nil && a.library.Preferences.PauseWhenBrowsing && a.shown && !a.stats.Paused {
		a.togglePause()
	}
}

// 滑块即时预览，停下后合并写盘，避免拖动时每一帧都同步文件。
func (a *app) previewPreferences(next library.Preferences) {
	if a.library == nil {
		return
	}
	before := a.library.Preferences
	a.library.Preferences = next
	a.updateCaptionPreferences(before, next)
	if a.preferencesTimer != nil {
		a.preferencesTimer.Stop()
	}
	a.preferencesDeadline = time.Now().Add(250 * time.Millisecond)
	if a.win == nil {
		return
	}
	a.win.Invalidate()
	a.preferencesTimer = time.AfterFunc(250*time.Millisecond, func() {
		a.win.Update(func() {
			if !a.closing && !time.Now().Before(a.preferencesDeadline) {
				a.savePreferences(a.library.Preferences)
			}
		})
	})
}
