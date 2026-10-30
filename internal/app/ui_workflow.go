//go:build darwin || windows

package app

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"live-mygo/internal/platforms"
)

var startupLabels = map[string]string{"home": "首页", "favorites": "关注", "history": "最近观看", "bilibili": "哔哩哔哩", "douyu": "斗鱼", "huya": "虎牙", "douyin": "抖音", "qie": "企鹅体育"}

func startupPage(page string) string {
	if _, ok := startupLabels[page]; ok {
		return page
	}
	return "home"
}

func (a *app) loadHome() {
	if a.catalogCancel != nil {
		a.catalogCancel()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.catalogCancel = cancel
	a.catalogGeneration++
	gen := a.catalogGeneration
	a.catalogLoading = true
	a.catalogError = ""
	a.catalogMore = false
	a.refreshFavorites(false)
	go func() {
		sources := []platforms.Platform{platforms.Bilibili, platforms.Douyu, platforms.Huya, platforms.Douyin, platforms.Qie}
		results := make([][]platforms.Room, len(sources))
		failures := make([]string, len(sources))
		var wg sync.WaitGroup
		for i, p := range sources {
			wg.Add(1)
			go func(i int, p platforms.Platform) {
				defer wg.Done()
				request, done := context.WithTimeout(ctx, 12*time.Second)
				defer done()
				result, err := a.client.Recommend(request, p, 1)
				if err != nil {
					failures[i] = p.Name() + "：" + err.Error()
					return
				}
				results[i] = result.Rooms
			}(i, p)
		}
		wg.Wait()
		a.win.Update(func() {
			if a.closing || gen != a.catalogGeneration || ctx.Err() != nil {
				return
			}
			a.catalog = mergeRecommendations(results)
			a.catalogLoading = false
			if len(a.catalog) == 0 {
				a.catalogError = strings.Join(failures, "\n")
			}
		})
	}()
}
func mergeRecommendations(groups [][]platforms.Room) []platforms.Room {
	out := []platforms.Room{}
	seen := map[string]bool{}
	for i := 0; ; i++ {
		added := false
		for _, g := range groups {
			if i < len(g) {
				added = true
				r := g[i]
				if !seen[roomKey(r)] {
					out = append(out, r)
					seen[roomKey(r)] = true
				}
			}
		}
		if !added {
			return out
		}
	}
}
func (a *app) preferredChatWidth() float32 {
	if a.library != nil && a.library.Preferences.ChatWidth > 0 {
		return float32(max(240, min(560, a.library.Preferences.ChatWidth)))
	}
	return chatPanelWidth
}
func (a *app) chatWidth(c *ui.Context) float32 {
	w, _ := c.Size()
	side := float32(0)
	if !a.focusMode {
		side = sidebarWidth
	}
	return min(a.preferredChatWidth(), max(240, (w-side)*.48))
}
func (a *app) chatResizeHandle(c *ui.Context, width, edge float32) {
	grip := ui.Box(c.Key("chat-resize")).Absolute().Top(edge).Bottom(edge).Right(width + edge - 4).Width(8).Cursor(ui.CursorResizeEW).Label("调整弹幕栏宽度")
	_, _, pressed := grip.Dragged()
	if pressed && a.library != nil {
		// 拖动时分隔条本身会跟着宽度移动，不能用逐帧 dx 累加；
		// 以按下时的指针位置为基准计算绝对位移，否则手一停就再也拖不动。
		px, _, _ := grip.PointerPosition()
		if !a.chatResizing {
			a.chatResizing = true
			a.chatDragPx, a.chatDragWidth = px, width
		}
		target := float32(max(240, min(560, a.chatDragWidth+a.chatDragPx-px)))
		if target != float32(a.library.Preferences.ChatWidth) {
			next := a.library.Preferences
			next.ChatWidth = float64(target)
			a.previewPreferences(next)
		}
	} else if a.chatResizing {
		a.chatResizing = false
		a.fitWindowToVideo(true)
	}
	if grip.Hovered() || a.chatResizing {
		grip.Background(colors(c.Theme()).line)
	}
}
func (a *app) closePlayback() {
	a.cancelSleep()
	if a.companion {
		a.toggleCompanion()
	}
	a.stop()
	if a.page == "player" {
		a.page = "home"
		if a.win != nil {
			a.loadCatalog(1)
		}
	}
}
func (a *app) cancelSleep() {
	if a.sleepTimer != nil {
		a.sleepTimer.Stop()
		a.sleepTimer = nil
	}
	a.sleepUntil = time.Time{}
}
func (a *app) setSleep(minutes int) {
	a.cancelSleep()
	if minutes <= 0 {
		return
	}
	a.sleepUntil = time.Now().Add(time.Duration(minutes) * time.Minute)
	deadline := a.sleepUntil
	a.sleepTimer = time.AfterFunc(time.Until(deadline), func() {
		a.win.Update(func() {
			a.finishSleep(deadline)
		})
	})
}

func (a *app) finishSleep(deadline time.Time) {
	if !a.closing && a.sleepUntil == deadline {
		a.closePlayback()
		a.toast = "睡眠定时已停止播放"
	}
}
func (a *app) sleepMenu(m *ui.Menu) {
	if !a.sleepUntil.IsZero() {
		remaining := max(1, int(time.Until(a.sleepUntil).Minutes()+.999))
		m.Item(fmt.Sprintf("剩余 %d 分钟 · %s 停止", remaining, a.sleepUntil.Format("15:04"))).Disabled(true)
		if m.Item("取消定时").Chosen() {
			a.cancelSleep()
		}
		m.Separator()
	}
	for _, minutes := range []int{15, 30, 60, 90, 120} {
		if m.Item(fmt.Sprintf("%d 分钟后停止", minutes)).Chosen() {
			a.setSleep(minutes)
		}
	}
}
func (a *app) toggleCompanion() {
	if a.win == nil {
		return
	}
	if a.win.IsFullScreen() {
		a.win.SetFullScreen(false)
	}
	a.companion = !a.companion
	a.win.SetAlwaysOnTop(a.companion)
	if a.companion {
		a.companionBounds = a.win.ContentBounds()
		a.page = "player"
		a.win.SetMinimumSize(320, 210)
		r := a.companionBounds
		r.Width, r.Height = 480, 270
		if a.stats.Width > 0 && a.stats.Height > 0 {
			r.Height = max(210, int(480*float64(a.stats.Height)/float64(a.stats.Width)))
		}
		a.win.SetContentBounds(r)
	} else {
		a.win.SetMinimumSize(680, 360)
		a.win.SetContentBounds(a.companionBounds)
		// AppKit 显示窗口按钮后可能重新布局标题栏，交回 MyGo 恢复红绿灯位置。
		a.win.Update(func() {
			if !a.closing && !a.companion {
				a.win.SetTitle(a.win.Title())
			}
		})
	}
	if a.surface != nil {
		// 放在窗口尺寸变化之后，避免 MyGo 重新布局标题栏时又把按钮显示出来。
		a.surface.SetWindowButtonsVisible(!a.companion)
	}
	if a.companion {
		a.controlsUntil = time.Time{}
		a.controlsHeld, a.controlsKeyboard = false, false
	} else {
		a.revealControls()
	}
	a.win.Invalidate()
}
func (a *app) companionView(c *ui.Context) {
	w, h := c.Size()
	if a.surface != nil {
		a.surface.SetCorners(0, false)
	}
	a.videoView(c, ui.Rect{W: w, H: h})
}

func (a *app) companionHeader(c *ui.Context, opacity float32) {
	bar := ui.Row(c.Key("companion-title")).Absolute().Top(8).Left(8).Right(8).Height(34).PaddingX(6).Gap(5).AlignItems(ui.Center).Radius(10).DragWindow().Background(ui.RGBA(24, 24, 28, .8)).Opacity(opacity).Hidden(opacity < .005).Label("小窗标题栏")
	if bar.Hovered() || a.controlsKeyboard && bar.FocusWithin() {
		a.controlsHeld = true
		a.controlsUntil = c.Now().Add(2800 * time.Millisecond)
	}
	bar.Children(func() {
		ui.Text(c, a.title).FontSize(11).TextColor(ui.RGBA(255, 255, 255, .9)).SingleLine().Grow(1).MinWidth(0).MarginX(4)
		if a.mediaButton(c, iconExitFullscreen, "返回完整窗口").Size(26, 26).Clicked() {
			a.toggleCompanion()
		}
		if a.mediaButton(c, iconClose, "关闭当前直播").Size(26, 26).Clicked() {
			a.closePlayback()
		}
	})
}
func (a *app) companionControls(c *ui.Context, width, opacity float32) {
	panel := ui.Row(c.Key("companion-controls")).Absolute().Bottom(12).Left((width - 84) / 2).Width(84).Height(40).PaddingX(6).Radius(12).AlignItems(ui.Center).Justify(ui.Center).Background(ui.RGBA(24, 24, 28, .85)).Opacity(opacity).Hidden(opacity < .005)
	if panel.Hovered() || a.controlsKeyboard && panel.FocusWithin() {
		a.controlsHeld = true
		a.controlsUntil = c.Now().Add(2800 * time.Millisecond)
	}
	panel.Children(func() {
		icon := iconPause
		if a.stats.Paused {
			icon = iconPlay
		}
		if a.mediaButton(c, icon, "暂停 / 继续").Bind(&a.playControl).Clicked() {
			a.togglePause()
		}
		icon = iconVolume
		if a.stats.Muted || a.volume == 0 {
			icon = iconMuted
		}
		if a.mediaButton(c, icon, "快速静音").Clicked() {
			a.toggleMute()
		}
	})
	if a.focusControls && opacity > .01 {
		a.playControl.Focus(c)
		a.focusControls = false
	}
}
