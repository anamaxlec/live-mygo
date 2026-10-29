//go:build darwin

package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"live-mygo/internal/player"
)

func (a *app) revealControls() { a.controlsUntil = time.Now().Add(2800 * time.Millisecond) }
func (a *app) playerMenuOpen() bool {
	return a.qualityOpen || a.sourceOpen || a.danmakuOpen || a.volumeOpen
}
func (a *app) closePlayerMenus() {
	a.qualityOpen, a.sourceOpen, a.danmakuOpen, a.volumeOpen = false, false, false, false
}

const watchHeaderHeight float32 = 58
const chatPanelWidth float32 = 286
const watchFrameInset float32 = 4
const watchFrameBorder float32 = .5
const watchFrameRadius float32 = 14

type watchLayout struct {
	Frame     ui.Rect
	Video     ui.Rect
	ChatWidth float32
}

func watchGeometry(width, height, header, sidebar, chat float32) watchLayout {
	inset, edge := float32(0), float32(0)
	if header > 0 {
		inset, edge = watchFrameInset, watchFrameBorder
	}
	frame := ui.Rect{X: sidebar + inset, Y: header + inset, W: max(1, width-sidebar-2*inset), H: max(1, height-header-2*inset)}
	return watchLayout{Frame: frame, Video: ui.Rect{X: frame.X + edge, Y: frame.Y + edge, W: max(1, frame.W-2*edge-chat), H: max(1, frame.H-2*edge)}, ChatWidth: chat}
}

func (a *app) contentView(c *ui.Context) {
	full := a.win != nil && a.win.IsFullScreen()
	root := ui.Column(c.Key("watch-layout")).Fill()
	target := float32(0)
	if a.chatVisible() {
		target = chatPanelWidth
	}
	duration := 220 * time.Millisecond
	if a.library != nil && a.library.Preferences.FollowVideo {
		// Duration == 0 会启用弹簧；跟随窗口比例时使用一步完成的过渡。
		duration = time.Nanosecond
	}
	chatWidth := root.AnimateTo("chat-width", target, ui.Animation{Duration: duration, Ease: ui.EaseInOut})
	header, sidebar := float32(0), float32(0)
	if !full {
		header = watchHeaderHeight
	}
	if !full && !a.focusMode {
		sidebar = sidebarWidth
	}
	if !full && a.navMeasured {
		sidebar = a.navWidth
	}
	width, height := c.Size()
	layout := watchGeometry(width, height, header, sidebar, chatWidth)
	inset, edge, radius := float32(0), float32(0), float32(0)
	if !full {
		inset, edge, radius = watchFrameInset, watchFrameBorder, watchFrameRadius
	}
	if a.surface != nil {
		a.surface.SetCorners(float64(max(0, radius-edge)), chatWidth < .1)
	}
	p := colors(c.Theme())
	chrome := a.watchChrome(c)
	root.Children(func() {
		if !full {
			a.watchHeader(c)
		}
		body := ui.Box(c.Key("watch-body")).Grow(1).MinHeight(0).FillWidth().Padding(inset)
		body.Draw(func(painter *ui.Painter, r ui.Rect) {
			if inset == 0 {
				return
			}
			// 只绘制容器外的底板；视频所在的内部区域保持透明，原生画面无需转绘。
			painter.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W, H: inset}, chrome, 0)
			painter.Fill(ui.Rect{X: r.X, Y: r.Y + r.H - inset, W: r.W, H: inset}, chrome, 0)
			painter.Fill(ui.Rect{X: r.X, Y: r.Y + inset, W: inset, H: r.H - 2*inset}, chrome, 0)
			painter.Fill(ui.Rect{X: r.X + r.W - inset, Y: r.Y + inset, W: inset, H: r.H - 2*inset}, chrome, 0)
		})
		body.Children(func() {
			frame := ui.Box(c.Key("watch-frame")).Fill().Radius(radius).Label("直播间内容容器")
			if !full {
				ambient, contact := float32(.20), float32(.14)
				if p.bg.R < 128 {
					ambient, contact = .38, .24
				}
				// 柔和的环境阴影拉开层次，窄接触阴影保留贴合感；不增加外框宽度。
				frame.Shadow(0, 1, 7, 0, ui.RGBA(0, 0, 0, ambient)).
					Shadow(0, 0, 2, 0, ui.RGBA(0, 0, 0, contact))
			}
			frame.Children(func() {
				ui.Column(c).Fill().Padding(edge).Radius(radius).Clip().Children(func() {
					ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
						a.videoView(c, layout.Video)
						ui.Box(c.Key("chat-reveal")).Width(chatWidth).Shrink(0).FillHeight().Radius(0, max(0, radius-edge), max(0, radius-edge), 0).Clip().Hidden(chatWidth < .1).Children(func() { a.chatView(c) })
					})
				})
				if !full {
					ui.Box(c.Key("watch-frame-edge")).Absolute().Left(0).Top(0).Right(0).Bottom(0).Radius(radius).PassThrough().Role(ui.RoleNone).Draw(func(painter *ui.Painter, r ui.Rect) {
						paintWatchFrame(painter, r, p, radius)
					})
				}
			})
		})
	})
}

// 描边在内容之上；阴影交给完整容器，确保重绘范围包含容器外侧。
func paintWatchFrame(p *ui.Painter, r ui.Rect, palette palette, radius float32) {
	half := watchFrameBorder / 2
	p.Stroke(ui.Rect{X: r.X + half, Y: r.Y + half, W: r.W - watchFrameBorder, H: r.H - watchFrameBorder}, palette.line, radius-half, watchFrameBorder)
}

func (a *app) watchChrome(c *ui.Context) ui.Color {
	if c.Vibrancy() && !a.customAppearance() {
		// 透出窗口已经启用的 NSVisualEffectView，而不是用半透明灰色模拟磨砂。
		return ui.Transparent
	}
	return colors(c.Theme()).sidebar
}

func (a *app) watchHeader(c *ui.Context) {
	p := colors(c.Theme())
	inset := float32(12)
	if a.focusMode || a.navMeasured && a.navWidth < 100 {
		offset := float32(0)
		if a.navMeasured {
			offset = a.navWidth
		}
		inset = max(12, max(100, c.TitleBar().Left+14)-offset)
	}
	ui.Row(c).FillWidth().Height(watchHeaderHeight).Shrink(0).Padding(0, 16, 0, inset).Gap(5).AlignItems(ui.Center).Background(a.watchChrome(c)).DragWindow().Children(func() {
		if iconButton(c.Key("watch-back"), iconBack, "返回列表 · Esc").Clicked() {
			a.backToBrowse()
		}
		if iconButton(c.Key("watch-sidebar"), iconSidebar, "显示 / 隐藏导航侧栏", !a.focusMode).Clicked() {
			a.focusMode = !a.focusMode
			a.fitWindowToVideo(false)
		}
		ui.Box(c).Width(1).Height(23).MarginX(9).Background(p.line)
		ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
			ui.Text(c, a.title).FontSize(13.5).FontWeight(600).TextColor(p.text).SingleLine().Tooltip(a.title)
			meta := a.targetRoom.Platform.Name()
			if a.targetRoom.Anchor != "" {
				meta += " · " + a.targetRoom.Anchor
			}
			if a.targetRoom.Category != "" {
				meta += " · " + a.targetRoom.Category
			}
			ui.Text(c, meta).FontSize(10.5).TextColor(p.muted).SingleLine()
		})
		if a.busy {
			ui.Spinner(c).Size(12, 12).MarginX(6)
		}
		if iconButton(c.Key("watch-search"), iconSearch, "快捷搜索直播 · ⌘L").Clicked() {
			a.openQuickSearch()
		}
		if a.stream != nil && a.library != nil {
			icon, label := iconHeart, "关注直播间"
			if a.library.Has(a.stream.Room) {
				icon, label = iconHeartFilled, "取消关注"
			}
			if iconButton(c.Key("watch-follow"), icon, label).Clicked() {
				a.toggleFavorite(a.stream.Room)
			}
		}
		more := iconButton(c.Key("room-options"), iconMore, "直播间选项")
		more.Menu(func(m *ui.Menu) {
			if a.library != nil && m.Item("窗口跟随画面比例").Checked(a.library.Preferences.FollowVideo).Chosen() {
				a.toggleFollowVideo()
			}
			if a.targetRoom.ID != "" {
				m.Separator()
				a.roomMenu(c, m, a.targetRoom)
			}
			m.Separator()
			if m.Item("结束观看").Chosen() {
				a.stop()
				a.backToBrowse()
			}
		})
		if iconButton(c.Key("watch-settings"), iconSettings, "设置 · ⌘,").Clicked() {
			a.settingsWindow(0)
		}
	})
}

func (a *app) videoView(c *ui.Context, frame ui.Rect) {
	video := ui.Box(c.Key("video")).Width(frame.W).FillHeight().Shrink(0).MinHeight(0).Role(ui.RoleGroup).Focusable().FocusRing(false).Bind(&a.transportHandle).Label("直播画面，双击切换全屏")
	if !a.shown {
		video.Background(ui.Hex("#0b0b0e"))
	}
	width := frame.W

	x, y, over := video.PointerPosition()
	// 画面可聚焦以接收快捷键；Space 必须先认领，避免退化为普通容器点击。
	if video.Focused() && video.Shortcut(0, ui.KeyTab) {
		a.controlsUntil = c.Now().Add(2800 * time.Millisecond)
		a.focusControls = true
		a.controlsKeyboard = true
	}
	if video.Focused() && video.Shortcut(0, ui.KeySpace) {
		a.togglePause()
	}
	if over && (!a.pointerIn || x != a.pointerX || y != a.pointerY) {
		a.controlsKeyboard = false
		a.revealControls()
	}
	a.pointerX, a.pointerY, a.pointerIn = x, y, over
	if video.DoubleClicked() && a.win != nil {
		a.win.ToggleFullScreen()
		a.revealControls()
	}
	if video.Clicked() {
		a.controlsKeyboard = false
		a.revealControls()
	}
	held := a.controlsHeld
	a.controlsHeld = false
	visible := !a.shown || a.busy || a.playerMenuOpen() || held || c.Now().Before(a.controlsUntil)
	target := float32(0)
	if visible {
		target = 1
	}
	opacity := video.Animate("controls", target, 160*time.Millisecond)
	if visible && a.shown && !a.busy && !a.playerMenuOpen() {
		if remaining := a.controlsUntil.Sub(c.Now()); remaining > 0 {
			c.After(remaining)
		}
	}
	video.Children(func() {
		a.barrageView(c, frame.W, frame.H)
		// 绘制层透明，原生视频就在其下；控制层始终位于视频内部。
		if opacity > .005 {
			ui.Box(c.Key("bottom-shade")).Absolute().Left(0).Right(0).Bottom(0).Height(132).Opacity(opacity).LinearGradient(ui.LinearGradient{Angle: 180, From: ui.Transparent, To: ui.RGBA(0, 0, 0, .28)})
		}
		if a.shown || a.stream != nil || a.targetRoom.ID != "" {
			a.transportView(c, width, opacity)
		}
		if a.win != nil && a.win.IsFullScreen() {
			ui.Row(c.Key("full-top")).Absolute().Left(18).Right(18).Top(16).Height(42).Opacity(opacity).Hidden(opacity < .005).Gap(10).AlignItems(ui.Center).Children(func() {
				if a.mediaButton(c, iconBack, "退出全屏").Clicked() {
					a.win.SetFullScreen(false)
				}
				ui.Text(c, a.title).FontSize(13).FontWeight(500).TextColor(ui.RGB(255, 255, 255)).SingleLine().Grow(1).MinWidth(0)
				if a.mediaButton(c, iconSearch, "快捷搜索直播 · ⌘L").Clicked() {
					a.openQuickSearch()
				}
			})
		}
	})
	if held && !a.controlsHeld {
		a.controlsUntil = c.Now().Add(2800 * time.Millisecond)
		c.After(2800 * time.Millisecond)
	}
	if a.surface != nil {
		_, height := c.Size()
		if !a.surface.SetBounds(float64(frame.X), float64(frame.Y), float64(frame.W), float64(frame.H), float64(height)) {
			c.After(12 * time.Millisecond)
		}
		a.surface.Show(a.shown && !a.minimized)
	}
}

func (a *app) mediaButton(c *ui.Context, icon *ui.SVG, label string, active ...bool) ui.Element {
	b := ui.ButtonBase(c).Size(32, 32).Radius(8).Center().Label(label).Tooltip(label)
	ink := ui.RGBA(255, 255, 255, .79)
	if len(active) > 0 {
		b.Role(ui.RoleToggleButton).Checked(active[0])
		if active[0] {
			ink = ui.RGB(255, 255, 255)
		}
	}
	bg := ui.Transparent
	if len(active) > 0 && active[0] {
		bg = ui.RGBA(255, 255, 255, .07)
	}
	if b.Hovered() || b.FocusVisible() {
		a.controlsHeld = true
		a.controlsUntil = c.Now().Add(2800 * time.Millisecond)
		bg = ui.RGBA(255, 255, 255, .12)
		ink = ui.RGB(255, 255, 255)
	}
	if b.Pressed() {
		bg = ui.RGBA(255, 255, 255, .2)
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 100 * time.Millisecond}))
	b.Children(func() { ui.Icon(c, icon).Size(17, 17).TextColor(ink) })
	return b
}

func (a *app) mediaTextButton(c *ui.Context, text, label string) ui.Element {
	b := ui.ButtonBase(c).Height(32).PaddingX(8).Radius(7).Gap(4).AlignItems(ui.Center).Label(label).Tooltip(label)
	if b.Hovered() || b.FocusVisible() {
		a.controlsHeld = true
		a.controlsUntil = c.Now().Add(2800 * time.Millisecond)
		b.Background(ui.RGBA(255, 255, 255, .12))
	}
	if b.Pressed() {
		b.Background(ui.RGBA(255, 255, 255, .2))
	}
	b.Children(func() {
		ui.Text(c, text).FontSize(11.5).FontWeight(500).TextColor(ui.RGBA(255, 255, 255, .9)).SingleLine()
		ui.Icon(c, iconChevronDown).Size(10, 10).TextColor(ui.RGBA(255, 255, 255, .55))
	})
	return b
}

func mediaDivider(c *ui.Context) {
	ui.Box(c).Size(1, 18).MarginX(5).Background(ui.RGBA(255, 255, 255, .12))
}

func (a *app) transportView(c *ui.Context, width, opacity float32) {
	compact := width < 600
	panelWidth := min(float32(660), width-36)
	h := float32(52)
	if compact {
		h = 90
	}
	panel := ui.Column(c.Key("embedded-controls")).Absolute().Left(max(12, (width-panelWidth)/2)).Bottom(22).Width(panelWidth).Height(h).Padding(9, 10).Radius(15).Background(ui.RGBA(27, 28, 32, .87)).Border(1, ui.RGBA(255, 255, 255, .14)).Shadow(0, 6, 22, 0, ui.RGBA(0, 0, 0, .24)).Opacity(opacity).Hidden(opacity < .005)
	if panel.Hovered() || a.controlsKeyboard && panel.FocusWithin() {
		a.controlsHeld = true
		a.controlsUntil = c.Now().Add(2800 * time.Millisecond)
	}
	panel.Children(func() {
		ui.Row(c).Height(32).Gap(3).AlignItems(ui.Center).Children(func() {
			a.playbackButtons(c, !compact)
			ui.Box(c).Grow(1).MinWidth(4)
			if !compact {
				a.streamControls(c)
			} else {
				status := "直播中"
				if a.stats.Paused {
					status = "已暂停"
				}
				ui.Box(c).Size(4, 4).Radius(2).Background(ui.Hex("#f47c88"))
				ui.Text(c, status).FontSize(10.5).TextColor(ui.RGBA(255, 255, 255, .65)).MarginX(6)
			}
		})
		if compact {
			ui.Row(c).Height(32).Margin(6, 0, 0, 0).Gap(3).AlignItems(ui.Center).Children(func() { a.streamControls(c) })
		}
	})
	if a.focusControls && opacity > .01 {
		a.playControl.Focus(c)
		a.focusControls = false
	}
}

func (a *app) playbackButtons(c *ui.Context, volumeSlider bool) {
	icon, label := iconPause, "暂停 · Space"
	if a.stats.Paused {
		icon, label = iconPlay, "继续 · Space"
	}
	if a.mediaButton(c.Key("play-pause"), icon, label).Bind(&a.playControl).Size(35, 32).Disabled(!a.shown).Clicked() {
		a.togglePause()
	}
	if a.mediaButton(c.Key("reload"), iconRefresh, "重新连接 · ⌘R").Disabled(a.busy || a.targetRoom.ID == "").Clicked() {
		a.reloadStream()
	}
	mediaDivider(c)
	volumeIcon := iconVolume
	if a.stats.Muted || a.volume == 0 {
		volumeIcon = iconMuted
	}
	if a.mediaButton(c.Key("mute"), volumeIcon, "静音 / 取消静音 · M").Clicked() {
		a.toggleMute()
	}
	if volumeSlider {
		// 白色的滑轨与圆形滑块，在深色控制条上保持足够对比。
		slider := ui.SliderBase(c.Key("volume"), &a.volume, 0, 100).Width(66).Height(26).PaddingX(5).Label("播放音量")
		value := a.volume
		slider.Draw(func(p *ui.Painter, r ui.Rect) {
			w := r.W - 10
			x := r.X + 5 + w*float32(value/100)
			p.Fill(ui.Rect{X: r.X + 5, Y: r.Y + 11.5, W: w, H: 3}, ui.RGBA(255, 255, 255, .24), 2)
			p.Fill(ui.Rect{X: r.X + 5, Y: r.Y + 11.5, W: x - r.X - 5, H: 3}, ui.RGBA(255, 255, 255, .75), 2)
			p.Fill(ui.Rect{X: x - 4.5, Y: r.Y + 8.5, W: 9, H: 9}, ui.RGB(255, 255, 255), 5)
		})
		if slider.Hovered() || slider.FocusVisible() {
			a.controlsHeld = true
			a.controlsUntil = c.Now().Add(2800 * time.Millisecond)
		}
		if slider.Changed() {
			a.setVolume()
		}
	} else {
		volume := a.mediaTextButton(c.Key("volume-popup"), fmt.Sprintf("%.0f%%", a.volume), "播放音量")
		if volume.Clicked() {
			a.volumeOpen = !a.volumeOpen
		}
		a.mediaPopover(c, volume, &a.volumeOpen, 190, func() {
			ui.Text(c, "音量").FontSize(12).FontWeight(600).TextColor(colors(c.Theme()).text).Margin(0, 0, 12, 0)
			if ui.Slider(c, &a.volume, 0, 100).Width(162).Label("播放音量").Changed() {
				a.setVolume()
			}
		})
	}
}

func (a *app) setVolume() {
	a.command("set", "volume", strconv.FormatFloat(a.volume, 'f', 0, 64))
	if a.volume > 0 && a.stats.Muted {
		a.stats.Muted = false
		a.command("set", "mute", "no")
	}
}

func (a *app) streamControls(c *ui.Context) {
	a.qualitySelector(c)
	a.sourceSelector(c)
	ui.Box(c).Grow(1).MinWidth(1)
	mediaDivider(c)
	danmakuIcon, danmakuLabel := iconDanmaku, "关闭画面弹幕 · D"
	if !a.barrageVisible() {
		danmakuIcon, danmakuLabel = iconDanmakuOff, "开启画面弹幕 · D"
	}
	if a.mediaButton(c.Key("barrage"), danmakuIcon, danmakuLabel, a.barrageVisible()).Clicked() {
		a.toggleBarrage()
	}
	settings := a.mediaButton(c.Key("barrage-settings"), iconSettings, "弹幕与画面设置")
	if settings.Clicked() {
		a.danmakuOpen = !a.danmakuOpen
	}
	a.mediaPopover(c, settings, &a.danmakuOpen, 320, func() { a.barrageSettings(c) })
	chatIcon, chatLabel := iconPanelRightClose, "收起弹幕列表"
	if !a.chatVisible() {
		chatIcon, chatLabel = iconPanelRightOpen, "展开弹幕列表"
	}
	if a.mediaButton(c.Key("chat"), chatIcon, chatLabel, a.chatVisible()).Clicked() {
		a.toggleChat()
	}
	icon, label := iconFullscreen, "全屏 · ⌃⌘F"
	if a.win != nil && a.win.IsFullScreen() {
		icon, label = iconExitFullscreen, "退出全屏 · Esc"
	}
	if a.mediaButton(c.Key("fullscreen"), icon, label).Disabled(!a.shown).Clicked() {
		if a.win != nil {
			a.win.ToggleFullScreen()
		}
	}
}

func (a *app) mediaPopover(c *ui.Context, anchor ui.Element, open *bool, width float32, content func()) {
	p := colors(c.Theme())
	_, screenHeight := c.Size()
	animatedPopover(c, anchor, open, true, func(panel ui.Element) {
		panel.AttachTo(anchor, ui.AnchorTopEnd, ui.AnchorBottomEnd).Margin(0, 0, 12, 0).Width(width).MaxHeight(max(220, min(560, screenHeight-50)))
		panel.Label("播放器选项面板")
		floatingPanel(panel, p)
		ui.Column(c).Gap(3).Children(content)
	})
}

func popupChoice(c *ui.Context, label, detail string, selected bool) ui.Element {
	p := colors(c.Theme())
	b := ui.ButtonBase(c).Height(34).PaddingX(9).Gap(10).Radius(7).AlignItems(ui.Center).Label(label)
	if b.Hovered() || b.FocusVisible() {
		b.Background(p.hover)
	}
	if selected {
		b.Background(p.selected)
	}
	b.Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(p.text).Grow(1).MinWidth(0).SingleLine()
		if detail != "" {
			ui.Text(c, detail).FontSize(9.5).TextColor(p.muted)
		}
		if selected {
			ui.Icon(c, iconCheck).Size(13, 13).TextColor(p.accent)
		} else {
			ui.Box(c).Size(13, 13)
		}
	})
	return b
}

func (a *app) qualitySelector(c *ui.Context) {
	label := a.qualityLabel
	if label == "" {
		label = "清晰度"
	}
	anchor := a.mediaTextButton(c.Key("quality"), label, "切换清晰度")
	anchor.Disabled(a.stream == nil || a.busy)
	if anchor.Clicked() {
		a.qualityOpen = !a.qualityOpen
		a.sourceOpen = false
	}
	a.mediaPopover(c, anchor, &a.qualityOpen, 202, func() {
		p := colors(c.Theme())
		ui.Text(c, "清晰度").FontSize(11).FontWeight(600).TextColor(p.muted).Padding(8, 9)
		if a.stream == nil {
			return
		}
		for _, q := range a.stream.Qualities {
			if popupChoice(c.Key(q.ID), q.Name, "", q.ID == a.stream.QualityID).Clicked() {
				a.qualityOpen = false
				a.resolve(a.stream.Room.Platform, a.stream.Room.ID, q.ID, a.stream.SourceID)
			}
		}
	})
}

func (a *app) sourceSelector(c *ui.Context) {
	label := "线路"
	if a.stream != nil {
		for _, s := range a.stream.Sources {
			if s.ID == a.stream.SourceID {
				label = s.Name
				break
			}
		}
	}
	anchor := a.mediaTextButton(c.Key("source"), label, "切换播放线路")
	anchor.Disabled(a.stream == nil || a.busy)
	if anchor.Clicked() {
		a.sourceOpen = !a.sourceOpen
		a.qualityOpen = false
	}
	a.mediaPopover(c, anchor, &a.sourceOpen, 224, func() {
		p := colors(c.Theme())
		ui.Text(c, "播放线路").FontSize(11).FontWeight(600).TextColor(p.muted).Padding(8, 9)
		if a.stream == nil {
			return
		}
		ui.Scroll(c).MaxHeight(280).Children(func() {
			ui.Column(c).Gap(2).Children(func() {
				for _, s := range a.stream.Sources {
					if popupChoice(c.Key(s.ID), s.Name, strings.ToUpper(s.Format), s.ID == a.stream.SourceID).Clicked() {
						a.sourceOpen = false
						a.changeSource(s.ID)
					}
				}
			})
		})
	})
}

func (a *app) changeSource(id string) {
	if a.stream == nil || a.busy || a.stream.SourceID == id {
		return
	}
	// 重新取流会刷新短期令牌，同时维持当前实际清晰度。
	a.resolve(a.stream.Room.Platform, a.stream.Room.ID, a.stream.QualityID, id)
}

func (a *app) setFit(mode string) {
	a.fitMode = mode
	a.doPlayer(func(p *player.Player) error {
		aspect, panscan := "yes", "0"
		if mode == "拉伸" {
			aspect = "no"
		}
		if mode == "填满" {
			panscan = "1"
		}
		if err := p.Command("set", "keepaspect", aspect); err != nil {
			return err
		}
		return p.Command("set", "panscan", panscan)
	}, func(err error) {
		if err != nil {
			a.toast = err.Error()
		}
	})
}

func (a *app) togglePause() {
	if !a.shown || a.player == nil {
		return
	}
	a.revealControls()
	a.stats.Paused = !a.stats.Paused
	value := "no"
	if a.stats.Paused {
		value = "yes"
	}
	var next player.Stats
	a.doPlayer(func(p *player.Player) error {
		if err := p.Command("set", "pause", value); err != nil {
			return err
		}
		next = p.Stats()
		return nil
	}, func(err error) {
		if err != nil {
			a.toast = err.Error()
			return
		}
		a.acceptStats(next)
	})
}
func (a *app) toggleMute() {
	if a.player == nil {
		return
	}
	a.revealControls()
	a.stats.Muted = !a.stats.Muted
	value := "no"
	if a.stats.Muted {
		value = "yes"
	}
	a.command("set", "mute", value)
}
func (a *app) reloadStream() {
	if a.busy {
		return
	}
	a.reconnectAttempts = 0
	if a.stream == nil {
		if a.targetRoom.ID != "" {
			a.resolve(a.targetRoom.Platform, a.targetRoom.ID, -1)
		}
		return
	}
	a.resolve(a.stream.Room.Platform, a.stream.Room.ID, a.stream.QualityID, a.stream.SourceID)
}

func (a *app) reloadStreamInPlace() {
	if a.stream != nil {
		r := a.stream
		a.reconnectAttempts = 0
		a.resolveRoom(r.Room.Platform, r.Room.ID, r.QualityID, true, r.SourceID)
	}
}
