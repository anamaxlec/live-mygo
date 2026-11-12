//go:build darwin || windows

package app

import (
	"context"
	"log"
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
	a.cancelMonitorSave()
	if a.library == nil || a.library.SetMonitor(state) != nil {
		a.toast = "未能保存监控室"
		return false
	}
	return true
}

func (a *app) cancelMonitorSave() {
	if a.monitorSaveTimer != nil {
		a.monitorSaveTimer.Stop()
		a.monitorSaveTimer = nil
	}
}

// 音量拖动时只改内存，停下后合并写盘。
func (a *app) previewMonitor(state library.MonitorState) {
	if a.library == nil {
		return
	}
	a.library.Monitor = &state
	a.cancelMonitorSave()
	if a.win == nil {
		return
	}
	var timer *time.Timer
	timer = time.AfterFunc(300*time.Millisecond, func() {
		a.win.Update(func() {
			if a.monitorSaveTimer == timer && a.library.Monitor != nil {
				a.saveMonitor(*a.library.Monitor)
			}
		})
	})
	a.monitorSaveTimer = timer
}

func (a *app) flushMonitor() {
	if a.monitorSaveTimer != nil && a.library != nil && a.library.Monitor != nil {
		a.saveMonitor(*a.library.Monitor)
	}
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
	if a.saveMonitor(next) {
		if a.monitorDrag.key == key {
			a.monitorDrag = monitorDrag{}
		}
		if a.monitorVolumeRoom == key {
			a.monitorVolumeRoom = ""
		}
		if viewer := a.monitorViewer(key); viewer != nil {
			a.retireMonitorViewer(viewer)
		}
		a.invalidateMonitor()
	}
}

func (a *app) monitorAudioEnabled(key string) bool {
	return a.monitorState().Audible[key]
}

func (a *app) toggleMonitorAudio(key string) {
	next := a.monitorState()
	next.Audible = maps.Clone(next.Audible)
	if next.Audible == nil {
		next.Audible = make(map[string]bool)
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
	next := a.monitorState()
	settings := next.Settings[key]
	settings.Volume = &volume
	next.Settings = maps.Clone(next.Settings)
	if next.Settings == nil {
		next.Settings = make(map[string]library.MonitorRoomSettings)
	}
	next.Settings[key] = settings
	a.previewMonitor(next)
	if viewer := a.monitorViewer(key); viewer != nil {
		viewer.volume = volume
		viewer.command("set", "volume", strconv.FormatFloat(volume, 'f', 0, 64))
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

// 监控室各路只在进入、离开或增删房间时同步，不在每帧对齐。
func (a *app) syncMonitorPlayback() {
	if a.win == nil || a.closing {
		return
	}
	if a.page != "monitor" {
		a.monitorDrag = monitorDrag{}
		a.monitorVolumeRoom = ""
		a.flushMonitor()
		for _, viewer := range a.monitorViewers {
			a.retireMonitorViewer(viewer)
		}
		return
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

// 主画面和副画面在监控室里看不到，暂停而不是只静音，避免后台继续拉流解码。
func (a *app) pauseForMonitor() {
	for _, viewer := range []*app{a, a.sideViewer} {
		if viewer != nil && !viewer.closing && viewer.shown && !viewer.stats.Paused {
			viewer.setPlaybackPaused(true, nil)
			viewer.pausedOnLeave = true
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
		if a.page == "monitor" {
			a.syncMonitorPlayback()
		}
		a.invalidateMonitor()
	}
}

func (a *app) closeMonitor() {
	page := a.monitorReturnPage
	if page == "player" && a.stream == nil && a.targetRoom.ID == "" || page == "" || page == "monitor" {
		page = "home"
	}
	a.openPage(page)
}

// 从监控室打开一路：直接用这一路已解析的播放地址，不再重新解析房间；
// 其余各路随离开监控室释放，返回监控室时重新连接。
func (a *app) openMonitorRoom(c *ui.Context, room platforms.Room, frame ui.Rect) {
	if !a.cardOpenAt.IsZero() {
		return
	}
	if a.player == nil {
		a.toast = "播放器正在初始化"
		return
	}
	if a.monitorReturnPage == "player" {
		a.monitorReturnPage = a.returnPage
	}
	viewer := a.monitorViewer(roomKey(room))
	if viewer == nil || viewer.stream == nil || !viewer.shown {
		a.playRoomHere(room)
		return
	}
	stream := *viewer.stream
	bitmap := viewer.videoBitmap
	a.returnPage = "monitor"
	a.adoptStream(room, &stream)
	if bitmap != nil && !c.Preferences().ReduceMotion {
		// 从格子的位置展开到播放页，展开期间先显示格子里的最后一帧。
		a.returnFrame = bitmap
		a.openFromCard(c, ui.Rect{X: frame.X, Y: frame.Y + monitorToolbarHeight, W: frame.W, H: frame.H})
	} else {
		a.enterPlayerFromCard(false)
	}
}

// adoptStream 用已取得的播放地址加载主播放器，沿用 resolveRoom 的成功路径但跳过网络解析。
func (a *app) adoptStream(room platforms.Room, s *platforms.Stream) {
	if a.reconnectTimer != nil {
		a.reconnectTimer.Stop()
		a.reconnectTimer = nil
	}
	a.stopRecording()
	a.rememberHomeCatalog()
	a.focusMode = true
	if a.shown {
		a.command("stop")
	}
	a.reconnectAttempts = 0
	a.pausedOnLeave = false
	a.resetChat("等待直播间连接")
	a.clearFlyingChat()
	a.shown = false
	a.firstFrameMS = 0
	if s.Room.Anchor == "" {
		s.Room.Anchor = room.Anchor
	}
	if s.Room.Cover == "" {
		s.Room.Cover = room.Cover
	}
	a.selectTargetRoom(s.Room)
	if a.resolveCancel != nil {
		a.resolveCancel()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.resolveCancel = cancel
	a.generation++
	gen := a.generation
	a.busy = true
	a.stream = s
	a.title = s.Room.Title
	a.status = "正在连接直播流"
	if a.surface != nil {
		a.renderedAtStart = a.surface.Metrics().RenderedFrames
	}
	audioOnly := a.audioOnly()
	a.doPlayer(func(p *player.Player) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		mode := "auto"
		if audioOnly {
			mode = "no"
		}
		if err := p.Command("set", "vid", mode); err != nil {
			return err
		}
		return p.Load(s.URL, s.Headers)
	}, func(err error) {
		if gen != a.generation {
			return
		}
		a.busy = false
		if err != nil {
			// 格子里的地址已经失效时退回正常解析。
			a.resolveRoom(room.Platform, room.ID, -1, roomOpenOptions{keepBrowsing: true})
			return
		}
		if a.library != nil {
			if err := a.library.WatchSession(s.Room, s.QualityID, s.SourceID); err != nil {
				log.Print("保存观看历史失败：", err)
			}
		}
		a.qualityLabel = ""
		for _, q := range s.Qualities {
			if q.ID == s.QualityID {
				a.qualityLabel = q.Name
			}
		}
		a.shown = true
		a.stats = player.Stats{Muted: a.stats.Muted}
		a.publishMediaInfo()
		a.revealControls()
		a.playStarted = time.Now()
		a.decodedMS = 0
		a.startChat(false)
	})
}

const monitorToolbarHeight = watchHeaderHeight

func (a *app) monitorLayer(c *ui.Context) {
	// 返回时保留同一层，退出动效才能覆盖已经恢复的浏览页。
	ui.Box(c.Key("monitor-layer")).Absolute().Fill().Clip().PassThrough().Children(func() {
		if a.page == "monitor" {
			width, _ := c.Size()
			transition := ui.ElementTransition{Duration: 260 * time.Millisecond, Ease: ui.EaseInOut}
			if a.cardOpenAt.IsZero() {
				transition.Exit = &ui.Motion{X: width, Opacity: 1}
			}
			ui.Box(c.Key("monitor-page")).Absolute().Fill().
				Transition(transition).
				Children(func() { a.monitorView(c) })
		}
	})
}

func (a *app) monitorView(c *ui.Context) {
	rooms := a.monitorRooms()
	width, height := c.Size()
	canvas := ui.Rect{W: max(1, width), H: max(1, height-monitorToolbarHeight)}
	state := a.monitorState()
	selected := slices.IndexFunc(rooms, func(room platforms.Room) bool { return roomKey(room) == a.monitorSelected })
	if selected < 0 && len(rooms) > 0 {
		selected = len(rooms) - 1
		a.monitorSelected = roomKey(rooms[selected])
	}
	frames := monitorFrames(canvas, rooms, state, a.monitorSelected)
	if a.surface != nil && a.cardOpenAt.IsZero() {
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
	a.monitorToolbar(c, rooms, state)
}

// 顶栏只放影响整个监控室的操作；每一路的声音、弹幕和音量在各自画面上调整。
func (a *app) monitorToolbar(c *ui.Context, rooms []platforms.Room, state library.MonitorState) {
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
		if len(rooms) > 0 {
			ui.Text(c, strconv.Itoa(len(rooms))+" / "+strconv.Itoa(monitorMaxRooms)).FontSize(11).TextColor(p.muted).Shrink(0)
		}
		ui.Box(c).Grow(1)
		label := monitorArrangementLabel(state.Arrange)
		if len(state.Layout) != 0 {
			label = "自由布局"
		}
		arrange := ui.ButtonBase(c.Key("monitor-arrange")).Height(30).PaddingX(9).Gap(6).AlignItems(ui.Center).Radius(7).
			Label("监控室布局").Tooltip("选择固定排列；拖动画面可自由摆放").Disabled(len(rooms) == 0)
		buttonFeedback(arrange, p)
		arrange.Children(func() {
			ui.Icon(c, iconMonitor).Size(14, 14).Shrink(0).TextColor(p.muted)
			ui.Text(c, label).SingleLine().FontSize(11.5).TextColor(p.text)
			ui.Icon(c, iconChevronDown).Size(11, 11).Shrink(0).TextColor(p.muted)
		})
		arrange.Menu(func(m *ui.Menu) {
			if len(state.Layout) != 0 {
				m.Item("自由布局").Radio(true)
				m.Separator()
			}
			for _, item := range monitorArrangements {
				if m.Item(item.label).Radio(len(state.Layout) == 0 && state.Arrange == item.id).Chosen() {
					a.setMonitorArrangement(item.id)
				}
			}
		})
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
	selected := key == a.monitorSelected
	tile := ui.Box(c.Key("monitor:"+key)).Absolute().Left(frame.X).Top(frame.Y).Size(frame.W, frame.H).Clip().
		Background(ui.Hex("#0b0b0e")).Label("监控画面 " + a.displayRoomName(room))
	hovered := tile.Hovered() || a.monitorDrag.key == key
	if tile.Clicked() {
		a.monitorSelected = key
	}
	if tile.DoubleClicked() {
		a.openMonitorRoom(c, room, frame)
	}
	tile.ContextMenu(func(m *ui.Menu) {
		label := "开启此路声音"
		if a.monitorAudioEnabled(key) {
			label = "关闭此路声音"
		}
		if m.Item(label).Chosen() {
			a.toggleMonitorAudio(key)
		}
		a.monitorRoomMenu(c, m, room, viewer, frame)
	})
	tile.Children(func() {
		if viewer != nil {
			viewer.takeVideoFrame()
			if ready && viewer.videoBitmap != nil {
				ui.Image(c, viewer.videoBitmap).Absolute().Fill().Fit(ui.Contain).PassThrough()
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
		if a.monitorAudioEnabled(key) {
			// 出声的画面常驻一个小喇叭，不用悬停也能看出哪几路在响。
			ui.Box(c.Key("audible")).Absolute().Top(8).Left(8).Size(24, 24).Radius(12).Center().
				Background(ui.RGBA(15, 15, 18, .72)).Label(name + " 正在出声").Children(func() {
				ui.Icon(c, iconVolume).Size(13, 13).TextColor(ui.RGB(255, 255, 255))
			})
		}
		opacity := float32(0)
		if hovered || !ready || a.monitorVolumeRoom == key {
			opacity = 1
		}
		if !c.Preferences().ReduceMotion {
			opacity = tile.Animate("controls", opacity, 140*time.Millisecond)
		}
		a.monitorTileControls(c, room, viewer, name, frame, canvas, opacity)
		if selected {
			ink := colors(c.Theme()).accent
			ui.Box(c.Key("selected")).Absolute().Fill().PassThrough().Role(ui.RoleNone).Draw(func(p *ui.Painter, r ui.Rect) {
				p.Stroke(ui.Rect{X: r.X + 1, Y: r.Y + 1, W: r.W - 2, H: r.H - 2}, ink, 0, 2)
			})
		}
	})
	if viewer != nil && viewer.player != nil && !viewer.closing {
		_, height := c.Size()
		if !viewer.surface.SetBounds(float64(frame.X), float64(frame.Y+monitorToolbarHeight), float64(frame.W), float64(frame.H), float64(height)) {
			c.After(12 * time.Millisecond)
		}
		viewer.surface.Show(viewer.surfaceShouldShow())
	}
}

func (a *app) monitorTileControls(c *ui.Context, room platforms.Room, viewer *app, name string, frame, canvas ui.Rect, opacity float32) {
	key := roomKey(room)
	ink := ui.RGBA(255, 255, 255, .84)
	ui.Row(c.Key("monitor-controls")).Absolute().Left(6).Right(6).Bottom(6).Height(32).PaddingX(4).
		Gap(2).AlignItems(ui.Center).Radius(8).Background(ui.RGBA(27, 28, 32, .87)).
		Opacity(opacity).Hidden(opacity < .005).Children(func() {
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
		if audible && frame.W >= 360 {
			a.monitorVolumeSlider(c.Key("volume"), key, name, 64)
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
		more := a.mediaButton(c.Key("more"), iconMore, "直播间操作 · "+name).Size(26, 26)
		more.Menu(func(m *ui.Menu) {
			a.monitorRoomMenu(c, m, room, viewer, frame)
		})
		if a.monitorVolumeRoom == key {
			open := true
			a.mediaPopover(c.Key("volume-popover"), more, &open, 190, func() {
				ui.Text(c, name).FontSize(12).FontWeight(600).SingleLine()
				a.monitorVolumeSlider(c, key, name, 162)
			})
			if !open {
				a.monitorVolumeRoom = ""
			}
		}
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

func (a *app) monitorVolumeSlider(c *ui.Context, key, name string, width float32) {
	volume := a.monitorVolume(key)
	if ui.Slider(c, &volume, 0, 100).Width(width).Label("调整 " + name + " 的音量").Changed() {
		a.setMonitorVolume(key, volume)
	}
}

func (a *app) monitorRoomMenu(c *ui.Context, m *ui.Menu, room platforms.Room, viewer *app, frame ui.Rect) {
	key := roomKey(room)
	if m.Item("调整音量").Chosen() {
		a.monitorVolumeRoom = key
	}
	if m.Item("打开直播间").Disabled(a.player == nil).Chosen() {
		a.openMonitorRoom(c, room, frame)
	}
	if m.Item("重新连接此路").Disabled(viewer == nil || viewer.busy).Chosen() {
		if viewer.player == nil {
			a.retireMonitorViewer(viewer)
		} else if viewer.stream != nil {
			viewer.reloadStream()
		} else {
			viewer.playRoomHere(room)
		}
	}
	m.Separator()
	if m.Item("移出监控室").Chosen() {
		a.removeMonitorRoom(key)
	}
}
