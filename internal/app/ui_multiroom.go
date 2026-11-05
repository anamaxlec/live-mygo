//go:build darwin || windows

package app

import (
	"context"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"live-mygo/internal/platforms"
	"live-mygo/internal/player"
)

const maxExtraWindows = 2

func (a *app) rootApp() *app {
	if a.parent != nil {
		return a.parent.rootApp()
	}
	return a
}

func (a *app) allPlaybackWindows() []*app {
	root := a.rootApp()
	return append([]*app{root}, root.extraWindows...)
}

// 一个窗口取消静音时，将其他窗口静音，避免多路声音叠在一起。
func (a *app) claimAudio() {
	root := a.rootApp()
	root.mediaOwner = a
	for _, other := range a.allPlaybackWindows() {
		if other == a || other.closing || !other.shown {
			continue
		}
		other.stats.Muted = true
		other.command("set", "mute", "yes")
		if other.win != nil {
			other.win.Invalidate()
		}
	}
	a.publishMediaInfo()
}

func (a *app) openExtraRoom(room platforms.Room) {
	root := a.rootApp()
	if root.closing || root.win == nil {
		return
	}
	if len(root.extraWindows) >= maxExtraWindows {
		a.toast = "最多同时观看三个直播间"
		return
	}
	ctx, cancel := context.WithCancel(root.ctx)
	child := &app{parent: root, ctx: ctx, cancel: cancel, library: root.library, vault: root.vault, client: platforms.NewClient(root.vault), opts: options{platform: string(room.Platform), room: room.ID, mute: true}, started: time.Now(), volume: root.volume, platformLabel: room.Platform.Name(), targetRoom: room, title: room.Title, page: "player", focusMode: true, companion: true, accountStatus: map[platforms.Platform]string{}, covers: map[string]*ui.Bitmap{}, coverLoading: map[string]bool{}, coverSlots: make(chan struct{}, 4), categories: map[platforms.Platform][]platforms.Category{}, categoryLoading: map[platforms.Platform]bool{}, playerOps: make(chan func(*player.Player), 64)}
	if child.volume == 0 {
		child.volume = 60
	}
	if child.library != nil {
		child.client.SetDouyuChatCompatibility(child.library.Preferences.DouyuLegacyTLS)
	}
	child.win = mygo.NewWindow(mygo.WindowOptions{Title: a.displayRoomName(room) + " · Live MyGo", Width: 640, Height: 360, MinWidth: 320, MinHeight: 210, TitleBarStyle: mygo.TitleBarHidden, TitleBarHeight: 58, TrafficLightPosition: &mygo.Point{X: 16, Y: 22}, Vibrancy: mygo.VibrancySidebar, BackgroundColor: "light-dark(#ffffff, #141416)", Content: ui.View(child.view)})
	child.win.SetAlwaysOnTop(true)
	child.win.OnResize(child.windowResized)
	child.win.OnFocus(func() {
		if !child.companion && !child.win.IsFullScreen() {
			child.win.SetTitle(child.win.Title())
		}
	})
	child.win.OnLeaveFullScreen(func() { child.fitWindowToVideo(true) })
	child.companionBounds = child.win.ContentBounds()
	child.companionBounds.Width, child.companionBounds.Height = 1180, 780
	child.win.OnClose(func(e *mygo.CloseEvent) {
		if child.clean {
			return
		}
		e.PreventDefault()
		if !child.closing {
			child.quit()
		}
	})
	child.win.OnClosed(func() {
		for i, viewer := range root.extraWindows {
			if viewer == child {
				root.extraWindows = append(root.extraWindows[:i], root.extraWindows[i+1:]...)
				break
			}
		}
		if root.mediaOwner == child {
			root.mediaOwner = nil
			root.publishMediaInfo()
		}
		if !root.closing {
			root.win.Invalidate()
		}
	})
	child.win.OnMinimize(func() {
		child.minimized = true
		if child.surface != nil {
			child.surface.Show(false)
		}
	})
	child.win.OnRestore(func() { child.minimized = false; child.win.Invalidate() })
	backend := ""
	if child.library != nil {
		backend = child.library.Preferences.PlayerBackend
	}
	surface, err := player.NewSurface(child.win.NativeHandle(), backend)
	if err != nil {
		cancel()
		child.clean = true
		child.win.Close()
		a.toast = "无法创建播放窗口"
		return
	}
	child.surface = surface
	surface.SetWindowButtonsVisible(false)
	surface.SetFrameCallback(child.win.Invalidate)
	child.playerDone = make(chan struct{})
	root.extraWindows = append(root.extraWindows, child)
	child.refreshAccountStatus()
	go child.runPlayer(surface)
}
