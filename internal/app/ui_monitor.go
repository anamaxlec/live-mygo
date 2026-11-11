//go:build darwin || windows

package app

import (
	"maps"
	"slices"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"
	"live-mygo/internal/library"
	"live-mygo/internal/platforms"
	"live-mygo/internal/player"
)

const monitorMaxRooms = 9

func playbackYesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func (a *app) monitorState() library.MonitorState {
	if a.library != nil && a.library.Monitor != nil {
		return *a.library.Monitor
	}
	return library.MonitorState{}
}

func (a *app) monitorRooms() []platforms.Room {
	var rooms []platforms.Room
	seen := make(map[string]bool)
	for _, room := range a.monitorState().Rooms {
		if room.Platform == "" || room.ID == "" || seen[roomKey(room)] {
			continue
		}
		rooms = append(rooms, room)
		seen[roomKey(room)] = true
		if len(rooms) == monitorMaxRooms {
			break
		}
	}
	return rooms
}

func (a *app) saveMonitor(state library.MonitorState) bool {
	if a.library == nil || a.library.SetMonitor(state) != nil {
		a.toast = "未能保存监控室"
		return false
	}
	return true
}

func (a *app) addMonitorRoom(room platforms.Room) {
	p, id, err := platforms.ParseRoom(room.Platform, room.ID)
	if err != nil || p == platforms.FLive && !a.raceMenuUnlocked() {
		a.toast = "无法添加这个直播间"
		return
	}
	room.Platform, room.ID = p, id
	rooms := a.monitorRooms()
	if slices.ContainsFunc(rooms, func(existing platforms.Room) bool { return roomKey(existing) == roomKey(room) }) {
		a.toast = "该直播间已在监控室"
		return
	}
	if len(rooms) >= monitorMaxRooms {
		a.toast = "监控室最多添加 9 路直播"
		return
	}
	next := a.monitorState()
	next.Rooms = append(rooms, room)
	if a.saveMonitor(next) {
		a.monitorSelected = roomKey(room)
		a.toast = "已加入监控室"
		if a.page == "monitor" {
			a.syncMonitorPlayback()
		}
		a.invalidateMonitor()
	}
}

func (a *app) removeMonitorRoom(key string) {
	next := a.monitorState()
	next.Rooms = slices.DeleteFunc(a.monitorRooms(), func(room platforms.Room) bool { return roomKey(room) == key })
	next.Audible = maps.Clone(next.Audible)
	delete(next.Audible, key)
	next.Layout = maps.Clone(next.Layout)
	delete(next.Layout, key)
	next.Settings = maps.Clone(next.Settings)
	delete(next.Settings, key)
	if next.Audio == key {
		next.Audio = ""
	}
	if a.saveMonitor(next) {
		if a.monitorDrag.key == key {
			a.monitorDrag = monitorDrag{}
		}
		if viewer := a.monitorViewer(key); viewer != nil {
			a.retireMonitorViewer(viewer)
		}
		a.invalidateMonitor()
	}
}

func (a *app) monitorAudioEnabled(key string) bool {
	state := a.monitorState()
	return state.Audible[key] || state.Audio != "" && state.Audio == key
}

func (a *app) toggleMonitorAudio(key string) {
	next := a.monitorState()
	next.Audible = maps.Clone(next.Audible)
	if next.Audible == nil {
		next.Audible = make(map[string]bool)
	}
	if next.Audio != "" {
		next.Audible[next.Audio] = true
		next.Audio = ""
	}
	enabled := !next.Audible[key]
	if enabled {
		next.Audible[key] = true
	} else {
		delete(next.Audible, key)
	}
	if a.saveMonitor(next) {
		a.monitorSelected = key
		if viewer := a.monitorViewer(key); viewer != nil {
			viewer.stats.Muted = !enabled
			viewer.command("set", "mute", playbackYesNo(!enabled))
		}
	}
}

func (a *app) monitorVolume(key string) float64 {
	if value := a.monitorState().Settings[key].Volume; value != nil {
		return max(0, min(100, *value))
	}
	if a.volume > 0 {
		return a.volume
	}
	return 60
}

func (a *app) saveMonitorRoomSettings(key string, settings library.MonitorRoomSettings) bool {
	next := a.monitorState()
	next.Settings = maps.Clone(next.Settings)
	if next.Settings == nil {
		next.Settings = make(map[string]library.MonitorRoomSettings)
	}
	next.Settings[key] = settings
	return a.saveMonitor(next)
}

func (a *app) setMonitorVolume(key string, volume float64) {
	settings := a.monitorState().Settings[key]
	settings.Volume = &volume
	if a.saveMonitorRoomSettings(key, settings) {
		if viewer := a.monitorViewer(key); viewer != nil {
			viewer.volume = volume
			viewer.command("set", "volume", strconv.FormatFloat(volume, 'f', 0, 64))
		}
	}
}

func (a *app) toggleMonitorDanmaku(key string) {
	settings := a.monitorState().Settings[key]
	settings.Danmaku = !settings.Danmaku
	if !a.saveMonitorRoomSettings(key, settings) {
		return
	}
	a.monitorSelected = key
	if viewer := a.monitorViewer(key); viewer != nil {
		if settings.Danmaku && viewer.shown {
			viewer.startChat(false)
		} else if !settings.Danmaku {
			viewer.resetChat("")
		}
	}
}

func (a *app) invalidateMonitor() {
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *app) monitorViewer(key string) *app {
	for _, viewer := range a.monitorViewers {
		if viewer.monitorKey == key && !viewer.closing {
			return viewer
		}
	}
	return nil
}

func (a *app) syncMonitorPlayback() {
	if a.win == nil || a.closing {
		return
	}
	if a.page != "monitor" {
		a.monitorDrag = monitorDrag{}
		for _, viewer := range a.monitorViewers {
			a.retireMonitorViewer(viewer)
		}
		if previous := a.monitorPreviousAudio; previous != nil {
			a.monitorPreviousAudio = nil
			if !previous.closing && (a.mediaOwner == previous || a.mediaOwner == nil && previous == a) {
				previous.stats.Muted = false
				previous.command("set", "mute", "no")
			}
		}
		return
	}
	owner := a.mediaOwner
	if owner == nil {
		owner = a
	}
	if a.monitorPreviousAudio == nil && owner.win == a.win && owner.shown && !owner.stats.Muted {
		a.monitorPreviousAudio = owner
		owner.stats.Muted = true
		owner.command("set", "mute", "yes")
	}
	rooms := a.monitorRooms()
	for _, viewer := range a.monitorViewers {
		if !slices.ContainsFunc(rooms, func(room platforms.Room) bool { return roomKey(room) == viewer.monitorKey }) {
			a.retireMonitorViewer(viewer)
		}
	}
	for _, room := range rooms {
		if len(a.monitorViewers) >= monitorMaxRooms {
			break
		}
		if !slices.ContainsFunc(a.monitorViewers, func(viewer *app) bool { return viewer.monitorKey == roomKey(room) }) {
			a.startMonitorViewer(room)
		}
	}
}

func (a *app) startMonitorViewer(room platforms.Room) {
	child := a.newRoomViewer(room)
	child.monitorKey, child.embedded, child.win = roomKey(room), true, a.win
	child.volume = a.monitorVolume(child.monitorKey)
	child.status = "正在初始化播放器"
	a.monitorViewers = append(a.monitorViewers, child)
	surface, err := player.NewMonitorSurface(a.win.NativeHandle())
	if err != nil {
		child.status = err.Error()
		return
	}
	child.surface = surface
	child.surface.SetFrameCallback(a.win.Invalidate)
	child.playerDone = make(chan struct{})
	go child.runPlayer(surface)
}

func (a *app) retireMonitorViewer(viewer *app) {
	if viewer.closing {
		return
	}
	if viewer.player != nil {
		viewer.surface.Show(false)
	}
	viewer.quit()
}

func (a *app) removeMonitorViewer(viewer *app) {
	a.monitorViewers = slices.DeleteFunc(a.monitorViewers, func(child *app) bool { return child == viewer })
	if !a.closing {
		a.invalidateMonitor()
	}
}

func (a *app) closeMonitor() {
	page := a.monitorReturnPage
	if page == "" || page == "monitor" {
		page = "home"
	}
	a.openPage(page)
}

const monitorToolbarHeight = watchHeaderHeight

func (a *app) monitorLayer(c *ui.Context) {
	// 返回时保留同一层，退出动效才能覆盖已经恢复的浏览页。
	ui.Box(c.Key("monitor-layer")).Absolute().Fill().Clip().PassThrough().Children(func() {
		if a.page == "monitor" {
			width, _ := c.Size()
			ui.Box(c.Key("monitor-page")).Absolute().Fill().
				Transition(ui.ElementTransition{Duration: 260 * time.Millisecond, Ease: ui.EaseInOut,
					Exit: &ui.Motion{X: width, Opacity: 1}}).
				Children(func() { a.monitorView(c) })
		}
	})
}

func (a *app) monitorView(c *ui.Context) {
	rooms := a.monitorRooms()
	width, height := c.Size()
	canvas := ui.Rect{W: max(1, width), H: max(1, height-monitorToolbarHeight)}
	state := a.monitorState()
	frames := monitorFrames(canvas, rooms, state.Layout)
	selected := slices.IndexFunc(rooms, func(room platforms.Room) bool { return roomKey(room) == a.monitorSelected })
	if selected < 0 && len(rooms) > 0 {
		selected = len(rooms) - 1
		a.monitorSelected = roomKey(rooms[selected])
	}
	if a.surface != nil {
		a.surface.Show(false)
	}
	background := ui.Hex("#111114")
	if len(rooms) == 0 {
		background = colors(c.Theme()).bg
	}
	ui.Box(c.Key("monitor-canvas")).Absolute().Top(monitorToolbarHeight).Bottom(0).Left(0).Right(0).
		Background(background).Clip().Children(func() {
		if len(rooms) == 0 {
			emptyStateAction(c, iconMonitor, "监控室", "从关注中添加直播间，或将链接拖到这里。", "添加直播间", a.openQuickSearch)
			return
		}
		drawRoom := func(room platforms.Room) {
			key := roomKey(room)
			frame := frames[key]
			if a.monitorDrag.key == key {
				frame = a.monitorDrag.frame
			}
			a.monitorTile(c, room, frame, canvas)
		}
		for i, room := range rooms {
			if i != selected {
				drawRoom(room)
			}
		}
		drawRoom(rooms[selected])
	})
	var active platforms.Room
	if selected >= 0 {
		active = rooms[selected]
	}
	a.monitorToolbar(c, active, rooms, len(state.Layout) != 0)
}

func (a *app) monitorToolbar(c *ui.Context, active platforms.Room, rooms []platforms.Room, customLayout bool) {
	p := colors(c.Theme())
	inset := titlebarLeft(c)
	if a.win != nil && a.win.IsFullScreen() {
		inset = 12
	}
	ui.Row(c.Key("monitor-toolbar")).Absolute().Top(0).Left(0).Right(0).Height(monitorToolbarHeight).
		Padding(0, titlebarRight(c, 12), 0, inset).Gap(8).AlignItems(ui.Center).Background(p.sidebar).
		DragWindow().Label("监控室工具栏").Children(func() {
		if iconButton(c.Key("monitor-exit"), iconBack, "退出监控室 · Esc").Clicked() {
			a.closeMonitor()
		}
		ui.Text(c, "监控室").FontSize(14).FontWeight(600).TextColor(p.text).Shrink(0)
		ui.Box(c).Grow(1)
		if active.ID != "" {
			key := roomKey(active)
			name := a.displayRoomName(active)
			button := ui.ButtonBase(c.Key("monitor-select-room")).Height(30).MaxWidth(170).PaddingX(8).
				Gap(6).AlignItems(ui.Center).Radius(7).Label("选择监控室直播间").Tooltip("选择直播间并移到前方")
			buttonFeedback(button, p)
			button.Children(func() {
				ui.Text(c, name).SingleLine().FontSize(11.5).TextColor(p.text)
				ui.Icon(c, iconChevronDown).Size(11, 11).Shrink(0).TextColor(p.muted)
			})
			button.Menu(func(m *ui.Menu) {
				for _, room := range rooms {
					label := a.displayRoomName(room) + " · " + room.Platform.Name()
					if m.Item(label).Checked(key == roomKey(room)).Chosen() {
						a.monitorSelected = roomKey(room)
					}
				}
			})
			ui.Icon(c, iconVolume).Size(15, 15).TextColor(p.muted)
			volume := a.monitorVolume(key)
			if ui.Slider(c.Key("monitor-volume:"+key), &volume, 0, 100).Width(104).Label("调整 " + name + " 的音量").Changed() {
				a.setMonitorVolume(key, volume)
			}
		}
		if ui.Button(c.Key("monitor-arrange"), "自动排列").Height(30).Disabled(!customLayout).Clicked() {
			next := a.monitorState()
			next.Layout = nil
			a.monitorDrag = monitorDrag{}
			a.saveMonitor(next)
		}
		if ui.Button(c.Key("monitor-add"), "添加直播间").Height(30).Disabled(len(rooms) >= monitorMaxRooms).Clicked() {
			a.openQuickSearch()
		}
		if a.win != nil {
			icon, label := iconFullscreen, "全屏显示监控室"
			if a.win.IsFullScreen() {
				icon, label = iconExitFullscreen, "退出全屏"
			}
			if iconButton(c.Key("monitor-fullscreen"), icon, label).Clicked() {
				a.win.ToggleFullScreen()
			}
		}
	})
}

func (a *app) monitorTile(c *ui.Context, room platforms.Room, frame, canvas ui.Rect) {
	key := roomKey(room)
	viewer := a.monitorViewer(key)
	ready := viewer != nil && !viewer.waitingForVideo()
	name := a.displayRoomName(room)
	if ready && viewer.targetRoom.Anchor != "" {
		name = a.displayRoomName(viewer.targetRoom)
	}
	tile := ui.Box(c.Key("monitor:"+key)).Absolute().Left(frame.X).Top(frame.Y).Size(frame.W, frame.H).Clip().
		Background(ui.Hex("#0b0b0e")).Label("监控画面 " + a.displayRoomName(room))
	if tile.Clicked() {
		a.monitorSelected = key
	}
	tile.ContextMenu(func(m *ui.Menu) {
		label := "开启此路声音"
		if a.monitorAudioEnabled(key) {
			label = "关闭此路声音"
		}
		if m.Item(label).Chosen() {
			a.toggleMonitorAudio(key)
		}
		a.monitorRoomMenu(m, room, viewer)
	})
	tile.Children(func() {
		if viewer != nil {
			viewer.takeVideoFrame()
			if ready && viewer.videoBitmap != nil {
				ui.Image(c, viewer.videoBitmap).Absolute().Fill().Fit(ui.FillBox).PassThrough()
				viewer.barrageView(c, frame.W, frame.H)
			}
		}
		if !ready {
			status := "正在连接直播间"
			if viewer != nil && viewer.status != "" {
				status = viewer.status
			}
			ui.Column(c).Fill().Center().Padding(12).Gap(7).Children(func() {
				ui.Text(c, name).FontSize(13).FontWeight(600).TextColor(ui.RGB(255, 255, 255)).SingleLine()
				ui.Text(c, status).FontSize(11).TextColor(ui.RGBA(255, 255, 255, .72)).MaxLines(2)
			})
		}
		a.monitorTileControls(c, room, viewer, name, frame, canvas)
	})
	if viewer != nil && viewer.player != nil && !viewer.closing {
		_, height := c.Size()
		if !viewer.surface.SetBounds(float64(frame.X), float64(frame.Y+monitorToolbarHeight), float64(frame.W), float64(frame.H), float64(height)) {
			c.After(12 * time.Millisecond)
		}
		viewer.surface.Show(viewer.surfaceShouldShow())
	}
}

func (a *app) monitorTileControls(c *ui.Context, room platforms.Room, viewer *app, name string, frame, canvas ui.Rect) {
	key := roomKey(room)
	ink := ui.RGBA(255, 255, 255, .84)
	background := ui.RGBA(27, 28, 32, .87)
	if key == a.monitorSelected {
		background = ui.RGBA(48, 48, 54, .95)
	}
	ui.Row(c.Key("monitor-controls")).Absolute().Left(6).Right(6).Bottom(6).Height(32).PaddingX(4).
		Gap(2).AlignItems(ui.Center).Radius(8).Background(background).Children(func() {
		move := ui.Row(c.Key("move")).Grow(1).MinWidth(0).FillHeight().PaddingX(5).AlignItems(ui.Center).
			Cursor(ui.CursorMove).Label("移动 " + a.displayRoomName(room)).Tooltip("拖动房间名移动画面")
		a.dragMonitorTile(c, move, key, frame, canvas, false)
		move.Children(func() { ui.Text(c, name).SingleLine().FontSize(11).FontWeight(550).TextColor(ink).PassThrough() })
		playIcon, playLabel := iconPause, "暂停此路"
		if viewer != nil && viewer.stats.Paused {
			playIcon, playLabel = iconPlay, "继续此路"
		}
		if a.mediaButton(c.Key("pause"), playIcon, playLabel).Size(26, 26).Disabled(viewer == nil || !viewer.shown).Clicked() {
			a.monitorSelected = key
			viewer.togglePause()
		}
		audible := a.monitorAudioEnabled(key)
		soundIcon, soundLabel := iconMuted, "开启此路声音"
		if audible {
			soundIcon, soundLabel = iconVolume, "关闭此路声音"
		}
		if a.mediaButton(c.Key("sound"), soundIcon, soundLabel+" · "+name, audible).Size(26, 26).Clicked() {
			a.toggleMonitorAudio(key)
		}
		danmaku := a.monitorState().Settings[key].Danmaku
		danmakuIcon, danmakuLabel := iconDanmakuOff, "开启此路弹幕"
		if danmaku {
			danmakuIcon, danmakuLabel = iconDanmaku, "关闭此路弹幕"
		}
		danmakuButton := a.mediaButton(c.Key("danmaku"), danmakuIcon, danmakuLabel+" · "+name, danmaku).
			Size(26, 26).Disabled(room.Platform == platforms.FLive)
		if danmaku && viewer != nil && viewer.chatStatus != "" {
			danmakuButton.Tooltip(danmakuLabel + " · " + viewer.chatStatus)
		}
		if danmakuButton.Clicked() {
			a.toggleMonitorDanmaku(key)
		}
		a.mediaButton(c.Key("more"), iconMore, "直播间操作").Size(26, 26).Menu(func(m *ui.Menu) {
			a.monitorRoomMenu(m, room, viewer)
		})
		resize := ui.Box(c.Key("resize")).Size(22, 26).Center().Cursor(ui.CursorResizeNWSE).
			Label("缩放 " + a.displayRoomName(room)).Tooltip("拖动调整画面大小")
		a.dragMonitorTile(c, resize, key, frame, canvas, true)
		resize.Draw(func(p *ui.Painter, r ui.Rect) {
			var path ui.Path
			x, y := r.X+6, r.Y+7
			path.MoveTo(x, y+10).LineTo(x+10, y).MoveTo(x+5, y+10).LineTo(x+10, y+5)
			p.StrokePath(&path, 1, ink)
		})
	})
}

func (a *app) monitorRoomMenu(m *ui.Menu, room platforms.Room, viewer *app) {
	key := roomKey(room)
	if m.Item("重新连接此路").Disabled(viewer == nil || viewer.busy).Chosen() {
		if viewer.player == nil {
			a.retireMonitorViewer(viewer)
			a.invalidateMonitor()
		} else if viewer.stream != nil {
			viewer.reloadStream()
		} else {
			viewer.playRoomHere(room)
		}
	}
	if m.Item("打开直播间").Disabled(a.player == nil).Chosen() {
		a.playRoom(room)
	}
	if m.Item("移出监控室").Chosen() {
		a.removeMonitorRoom(key)
	}
}
