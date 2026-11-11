//go:build darwin || windows

package app

import (
	"runtime"
	"time"

	"github.com/egoist/mygo/ui"
	"live-mygo/internal/platforms"
	"live-mygo/internal/player"
)

func (a *app) hasSideViewer() bool { return a.sideViewer != nil && !a.sideViewer.closing }

func (a *app) featuredViewer() *app {
	if a.hasSideViewer() && a.sideFeatured {
		return a.sideViewer
	}
	return a
}

func (a *app) secondaryViewer() *app {
	if a.sideFeatured {
		return a
	}
	return a.sideViewer
}

func (a *app) canAddSideRoom(room platforms.Room) bool {
	return a.parent == nil && a.win != nil && a.player != nil && !a.closing && !a.companion && a.systemPicture == nil &&
		a.featuredViewer().shown && room.ID != "" && roomKey(room) != roomKey(a.featuredViewer().targetRoom) &&
		(a.hasSideViewer() && a.secondaryViewer().player != nil || a.sideViewer == nil && len(a.extraWindows) < maxExtraWindows)
}

func (a *app) openSideRoom(c *ui.Context, room platforms.Room) {
	if !a.canAddSideRoom(room) {
		a.toast = "请先播放一场直播，再添加另一场"
		return
	}
	if a.page != "player" {
		a.openFromCard(c, ui.Rect{})
	}
	if a.hasSideViewer() {
		a.secondaryViewer().playRoomHere(room)
	} else {
		child := a.newRoomViewer(room)
		child.embedded, child.win = true, a.win
		backend := ""
		if a.library != nil {
			backend = a.library.Preferences.PlayerBackend
		}
		surface, err := player.NewSurface(a.win.NativeHandle(), backend)
		if err != nil {
			child.cancel()
			a.toast = "无法创建副画面"
			return
		}
		child.surface = surface
		surface.SetFrameCallback(a.win.Invalidate)
		child.playerDone = make(chan struct{})
		child.status = "正在连接直播间"
		a.sideViewer = child
		go child.runPlayer(surface)
	}
	a.win.Invalidate()
}

func (a *app) swapFeaturedViewer() {
	if !a.hasSideViewer() || !a.secondaryViewer().shown {
		return
	}
	before, after := a.featuredViewer(), a.secondaryViewer()
	before.closePlayerMenus()
	before.resetChat("")
	a.sideFeatured = !a.sideFeatured
	after.startChat(false)
	after.revealControls()
	if !before.stats.Muted && before.volume > 0 && after.stats.Muted {
		after.toggleMute()
	}
}

func (a *app) closeSideViewer(restoreMain bool) {
	if !a.hasSideViewer() {
		return
	}
	child := a.sideViewer
	if a.sideFeatured {
		a.sideFeatured = false
		if restoreMain {
			a.startChat(false)
		}
	}
	if restoreMain && a.mediaOwner == child && child.shown && !child.stats.Muted && a.shown && a.stats.Muted {
		a.toggleMute()
	}
	if child.player != nil {
		child.surface.Show(false)
	}
	child.quit()
}

func (a *app) removeSideViewer(child *app) {
	if a.sideViewer != child {
		return
	}
	a.sideViewer = nil
	a.sideFeatured = false
	if a.mediaOwner == child {
		a.mediaOwner = nil
		a.publishMediaInfo()
	}
	if !a.closing {
		a.win.Invalidate()
	}
}

const splitVideoGap = 2

func splitVideoWidths(width float32) (primary, secondary float32) {
	secondary = max(1, (width-splitVideoGap)*.4)
	return max(1, width-secondary-splitVideoGap), secondary
}

func (a *app) splitVideoView(c *ui.Context, frame ui.Rect) {
	primary := a.featuredViewer()
	if a.snap || !a.hasSideViewer() {
		a.videoView(c, frame)
		return
	}
	secondary := a.secondaryViewer()
	var width float32
	frame.W, width = splitVideoWidths(frame.W)
	if primary.surface != nil {
		primary.surface.SetCorners(0, false)
	}
	if secondary.surface != nil && secondary.player != nil {
		secondary.surface.SetCorners(0, false)
	}
	primary.videoView(c, frame)
	ui.Box(c).Width(splitVideoGap).FillHeight().Shrink(0).Background(colors(c.Theme()).line)
	secondaryFrame := ui.Rect{X: frame.X + frame.W + splitVideoGap, Y: frame.Y, W: width, H: frame.H}
	a.secondaryVideoView(c, secondary, secondaryFrame)
}

func (a *app) secondaryVideoView(c *ui.Context, viewer *app, frame ui.Rect) {
	viewer.takeVideoFrame()
	video := ui.Box(c.Key("secondary-video")).Width(frame.W).FillHeight().Shrink(0).Clip().Label("副画面")
	if runtime.GOOS == "windows" || viewer.waitingForVideo() || viewer.audioOnly() {
		video.Background(ui.Hex("#0b0b0e"))
	}
	if room, ok := a.readLiveLinkDrop(video); ok {
		a.openSideRoom(c, room)
	}
	video.Children(func() {
		if viewer.videoBitmap != nil && !viewer.waitingForVideo() && !viewer.audioOnly() {
			ui.Image(c, viewer.videoBitmap).Absolute().Fill().Fit(ui.FillBox).PassThrough()
		}
		if viewer.waitingForVideo() || viewer.audioOnly() {
			ui.Column(c).Fill().Center().Gap(8).Padding(10).Children(func() {
				status := viewer.status
				if viewer.audioOnly() && viewer.shown {
					status = "只听声音"
				}
				ui.Text(c, status).FontSize(12).MaxLines(3).TextColor(ui.RGBA(255, 255, 255, .85))
				if !viewer.busy && !viewer.shown && viewer.player != nil && ui.Button(c, "重连副画面").Clicked() {
					viewer.reloadStream()
				}
			})
		}
		ui.Row(c).Absolute().Top(10).Left(10).Right(10).Height(36).PaddingX(6).Gap(4).AlignItems(ui.Center).Radius(8).Background(ui.RGBA(15, 15, 18, .78)).Children(func() {
			ui.Text(c, viewer.title).Grow(1).MinWidth(0).FontSize(12).SingleLine().TextColor(ui.RGB(255, 255, 255))
			sound := iconVolume
			if viewer.stats.Muted || viewer.volume == 0 {
				sound = iconMuted
			}
			if viewer.mediaButton(c, sound, "切换副画面声音").Disabled(!viewer.shown).Clicked() {
				viewer.toggleMute()
			}
			if viewer.mediaButton(c, iconSwap, "交换主副画面").Disabled(!viewer.shown).Clicked() {
				a.swapFeaturedViewer()
			}
			if viewer.mediaButton(c, iconClose, "退出双画面，保留最初的直播").Clicked() {
				a.closeSideViewer(true)
			}
		})
	})
	if viewer.surface != nil && viewer.player != nil && !viewer.closing {
		_, height := c.Size()
		if !viewer.surface.SetBounds(float64(frame.X), float64(frame.Y), float64(frame.W), float64(frame.H), float64(height)) {
			c.After(12 * time.Millisecond)
		}
		viewer.surface.Show(viewer.surfaceShouldShow())
	}
}
