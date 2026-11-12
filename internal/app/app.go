//go:build darwin || windows

package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"live-mygo/internal/accounts"
	"live-mygo/internal/library"
	"live-mygo/internal/platforms"
	"live-mygo/internal/player"
)

type options struct {
	platform, room, file, report string
	duration                     time.Duration
	mute                         bool
}
type app struct {
	companion       bool
	companionBounds mygo.Rectangle
	sleepTimer      *time.Timer
	sleepUntil      time.Time
	chatResizing    bool
	chatDragPx      float32
	chatDragWidth   float32

	win                                    *mygo.Window
	surface                                player.VideoSurface
	player                                 *player.Player
	videoBitmap                            *ui.Bitmap
	videoPix                               []byte
	playerOps                              chan func(*player.Player)
	vault                                  *accounts.Vault
	client                                 *platforms.Client
	ctx                                    context.Context
	cancel                                 context.CancelFunc
	resolveCancel                          context.CancelFunc
	generation                             int
	platformLabel, roomInput, qualityLabel string
	status, title                          string
	busy, shown, closing, clean            bool
	volume                                 float64
	stats                                  player.Stats
	stream                                 *platforms.Stream
	targetRoom                             platforms.Room
	roomDanmaku, roomChatPanel             *bool
	accountWin                             *mygo.Window
	settings                               settingsState
	quick                                  quickSearchState
	categoryWin                            *mygo.Window
	accountStatus                          map[platforms.Platform]string
	opts                                   options
	started                                time.Time
	samples                                []player.Stats
	firstFrameMS                           int64
	decodedMS                              int64
	renderedAtStart                        uint64
	playStarted                            time.Time
	page                                   string
	library                                *library.State
	catalog                                []platforms.Room
	catalogPage                            int
	catalogQuery, catalogError             string
	catalogLoading, catalogMore            bool
	catalogCancel                          context.CancelFunc
	catalogGeneration                      int
	homeCatalog                            homeCatalogState
	grid                                   ui.ListState
	categories                             map[platforms.Platform][]platforms.Category
	categoryLoading                        map[platforms.Platform]bool
	category                               platforms.Category
	libraryPlatform                        string
	focusMode                              bool
	pausedOnLeave                          bool
	chatCancel                             context.CancelFunc
	chatGeneration                         int
	chatRoomKey                            string
	chatMessages                           []platforms.ChatMessage
	chatDropped                            uint64
	chatStatus                             string
	chatList                               ui.ListState
	chatNext                               uint64
	iconSet, iconDark                      bool
	covers                                 map[string]coverThumbnail
	coverOrder                             []string
	coverSlots                             chan struct{}
	coverLoading                           map[string]bool
	coverRetry                             map[string]time.Time
	captionSlots                           chan struct{}
	libraryRooms                           map[string]platforms.Room
	libraryRefresh                         time.Time
	libraryRefreshing                      bool
	searchHandle                           ui.Handle
	transportHandle                        ui.Handle
	returnPage, toast                      string
	controlsUntil                          time.Time
	pointerX, pointerY                     float32
	pointerIn                              bool
	controlsHeld                           bool
	controlsKeyboard                       bool
	focusControls                          bool
	pendingPage                            string
	playControl                            ui.Handle
	navWidth                               float32
	navMeasured                            bool
	navReturn                              uint64
	skipBrowseEnter                        bool
	builtPage                              string
	returnShrink                           bool
	returnShrinkAt                         time.Time
	returnFromOpen                         bool
	returnWasBusy                          bool
	returnFrame                            *ui.Bitmap
	returnCapture                          *time.Timer
	cardOpenAt                             time.Time
	cardOpenFrom                           ui.Rect
	cardOpenFrames                         uint64
	cardShatter                            cardShatterState
	playerNavWidth, playerChatWidth        float32
	snap                                   bool
	snapNav, snapChat                      float32
	nowCardBounds                          ui.Rect
	qualityOpen, sourceOpen                bool
	danmakuOpen, volumeOpen                bool
	chatSettingsOpen                       bool
	chatSeen                               uint64
	chatRecent                             []recentChat
	captions                               []flyingCaption
	pendingCaptions                        []platforms.ChatMessage
	danmakuTab                             int
	chatOnlySC                             bool
	filterWin                              *mygo.Window
	filterDraft                            string
	fitMode                                string
	aspectBusy                             bool
	aspectW, aspectH                       int
	aspectTimer                            *time.Timer
	resizeDeadline                         time.Time
	minimized                              bool
	preferencesTimer                       *time.Timer
	preferencesDeadline                    time.Time
	reconnectTimer                         *time.Timer
	reconnectAttempts                      int
	reconnectSources                       []string
	recentFaults                           []diagnosticFault
	diagnosticsExporting                   bool
	updates                                releaseCheckState
	chatFailureExpanded                    bool
	favoriteEditor                         favoriteEditorState
	favoriteGroup                          string
	backupBusy                             bool
	audioModeChanging                      bool
	notificationTimer                      *time.Timer
	notificationKnown                      map[string]bool
	notificationLast                       time.Time
	mediaCommandsOff                       func()
	parent                                 *app
	extraWindows                           []*app
	sideViewer                             *app
	monitorViewers                         []*app
	monitorSaveTimer                       *time.Timer
	monitorReturnPage                      string
	monitorKey                             string
	monitorSelected                        string
	monitorVolumeRoom                      string
	monitorDrag                            monitorDrag
	immersive, watchButtonsHidden          bool
	immersiveHeaderVisible                 bool
	immersiveRestoreNav                    bool
	sideFeatured                           bool
	embedded                               bool
	systemPicture                          systemPictureInPicture
	mediaOwner                             *app
	playerDone                             chan struct{}
	captureBusy                            bool
	recordingPath, lastCapturePath         string
	recordingUntil                         time.Time
	recordingTimer                         *time.Timer
	chatClearRun                           uint64
	chatClearActive                        bool
	restartAfterClose                      bool
}

type systemSecrets struct{}

func (systemSecrets) Get(k string) ([]byte, error) {
	b, err := mygo.Secrets.Service("dev.livemygo.verify").Get(k)
	if errors.Is(err, mygo.ErrSecretNotFound) {
		return nil, nil
	}
	return b, err
}
func (systemSecrets) Set(k string, b []byte) error {
	return mygo.Secrets.Service("dev.livemygo.verify").Set(k, b)
}

func Run(version string) {
	appVersion = strings.TrimSpace(version)
	var o options
	flag.StringVar(&o.platform, "platform", "", "启动平台；留空使用启动页设置")
	flag.StringVar(&o.room, "room", "", "启动时播放指定房间")
	flag.StringVar(&o.file, "file", "", "启动时播放本地测试视频")
	flag.StringVar(&o.report, "report", "", "保存不含 Cookie 与播放 URL 的验证报告")
	flag.DurationVar(&o.duration, "duration", 0, "验证完成后自动退出；0 表示不自动退出")
	flag.BoolVar(&o.mute, "mute", false, "静音启动，保留音频解码")
	flag.Parse()
	config, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	if custom := os.Getenv("LIVE_MYGO_DATA_DIR"); custom != "" {
		config = custom
	} else {
		config = filepath.Join(config, "LiveMyGo")
	}
	ctx, cancel := context.WithCancel(context.Background())
	a := &app{ctx: ctx, cancel: cancel, opts: o, started: time.Now(), volume: 60, platformLabel: "哔哩哔哩", roomInput: o.room, status: "正在初始化播放器", title: "输入房间号或直播间链接", accountStatus: map[platforms.Platform]string{}}
	a.playerOps = make(chan func(*player.Player), 64)
	if o.mute {
		a.volume = 0
	}
	if o.platform == "douyu" {
		a.platformLabel = "斗鱼"
	}
	a.vault = &accounts.Vault{Dir: filepath.Join(config, "accounts"), Secrets: systemSecrets{}}
	a.client = platforms.NewClient(a.vault)
	a.library, err = library.Load(filepath.Join(config, "library.json"))
	if err != nil {
		a.library = nil
		log.Print("读取本地关注和历史失败：", err)
		a.toast = "无法读取本机关注和观看历史"
	} else if !o.mute {
		a.volume = a.library.Preferences.Volume
	}
	if a.library != nil {
		a.client.SetDouyuChatCompatibility(a.library.Preferences.DouyuLegacyTLS)
	}
	a.page = initialPage(a.library, o)
	if a.page == "huya" || a.page == "douyu" || a.page == "bilibili" || a.page == "douyin" || a.page == "qie" {
		a.platformLabel = platforms.Platform(a.page).Name()
	}
	if a.page == string(platforms.FLive) {
		a.platformLabel = platforms.FLive.Name()
	}
	a.covers = map[string]coverThumbnail{}
	a.coverSlots = make(chan struct{}, 4)
	a.coverLoading = map[string]bool{}
	a.categories = map[platforms.Platform][]platforms.Category{}
	a.categoryLoading = map[platforms.Platform]bool{}
	mygo.App.SetName("Live MyGo")
	mygo.App.SetWindowStateStore(appWindowStateStore{app: a})
	if a.library != nil {
		mygo.Theme.SetSource(mygo.ThemeSource(a.library.Preferences.Appearance))
	}
	mygo.App.WhenReady(func() { a.start() })
	mygo.App.OnDidBecomeActive(func() {
		for _, window := range a.allPlaybackWindows() {
			if window.surface != nil && !window.closing {
				window.surface.Show(window.surfaceShouldShow())
				if window.win != nil {
					window.win.Invalidate()
				}
			}
		}
	})
	mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
		if !a.clean {
			e.PreventDefault()
			a.quit()
		}
	})
	if err = mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func (a *app) start() {
	_ = mygo.App.Dock.SetIcon(appIconPNG)
	a.win = mygo.NewWindow(mygo.WindowOptions{Title: "Live MyGo", StateKey: "main", Width: 1180, Height: 780, MinWidth: 1000, MinHeight: 580, TitleBarStyle: mygo.TitleBarHidden, TitleBarHeight: 58, TrafficLightPosition: &mygo.Point{X: 16, Y: 22}, Vibrancy: mygo.VibrancySidebar, BackgroundColor: "light-dark(#ffffff, #141416)", Content: ui.View(a.view)})
	_ = a.win.SetIcon(appIconPNG)
	a.registerMediaControls()
	a.fitInitialWindow()
	a.win.OnResize(a.windowResized)
	a.win.OnFocus(func() {
		if a.surface != nil {
			a.surface.Show(a.surfaceShouldShow())
		}
		if a.watchButtonsHidden {
			a.reapplyWindowButtons()
		} else if !a.companion && !a.win.IsFullScreen() {
			a.win.SetTitle(a.win.Title())
		}
		a.refreshFavorites(false)
	})
	a.win.OnMinimize(func() {
		a.minimized = true
		if a.surface != nil {
			a.surface.Show(false)
		}
		for _, viewer := range a.allPlaybackWindows() {
			if viewer.embedded && !viewer.closing && viewer.player != nil {
				viewer.surface.Show(false)
			}
		}
	})
	a.win.OnRestore(func() {
		a.minimized = false
		if a.surface != nil {
			a.surface.Show(a.surfaceShouldShow())
		}
		a.win.Invalidate()
		a.doPlayer(func(*player.Player) error { return nil }, nil)
	})
	a.win.OnLeaveFullScreen(func() { a.fitWindowToVideo(true) })
	a.win.OnClose(func(e *mygo.CloseEvent) {
		if a.clean {
			return
		}
		e.PreventDefault()
		if a.closing {
			return
		}
		if a.companion {
			// 小窗的关闭按钮只退出小窗，不应结束整个应用。
			a.toggleCompanion()
			return
		}
		a.quit()
	})
	backend := ""
	if a.library != nil {
		backend = a.library.Preferences.PlayerBackend
	}
	s, err := player.NewSurface(a.win.NativeHandle(), backend)
	if err != nil {
		a.status = err.Error()
		a.toast = err.Error()
		a.clean = true
		return
	}
	a.surface = s
	s.SetFrameCallback(a.win.Invalidate)
	a.playerDone = make(chan struct{})
	go a.runPlayer(s)
	a.refreshAccountStatus()
	if a.page != "welcome" && a.opts.file == "" && a.opts.room == "" && a.sessionToResume() == nil {
		a.loadCatalog(1)
	}
	a.loadCategories(a.platform())
	a.refreshFavorites(false)
	a.armNotifications()
	if a.opts.duration > 0 {
		go func() {
			timer := time.NewTimer(a.opts.duration)
			defer timer.Stop()
			select {
			case <-timer.C:
				a.win.Update(a.quit)
			case <-a.ctx.Done():
			}
		}()
	}
}

// libmpv 的同步 API 可能等待 macOS 主线程；所有调用都由此后台循环串行处理。
func (a *app) runPlayer(surface player.VideoSurface) {
	p, err := player.New(surface)
	if err != nil {
		a.win.Update(func() { a.status = err.Error(); a.toast = err.Error(); a.surface.Close(); close(a.playerDone) })
		return
	}
	a.win.Update(func() {
		if a.closing {
			return
		}
		a.player = p
		a.command("set", "volume", strconv.FormatFloat(a.volume, 'f', 0, 64))
		if a.parent != nil {
			a.stats.Muted = a.monitorKey == "" || !a.parent.monitorAudioEnabled(a.monitorKey)
			a.command("set", "mute", playbackYesNo(a.stats.Muted))
		}
		a.status = "播放器已就绪"
		if a.opts.file != "" {
			a.playFile(a.opts.file)
		} else if a.opts.room != "" {
			a.resolve(a.platform(), a.opts.room, -1)
		} else if session := a.sessionToResume(); session != nil {
			a.stats.Muted = true
			a.command("set", "mute", "yes")
			a.targetRoom = session.Room
			a.resolve(session.Room.Platform, session.Room.ID, session.Quality, session.Source)
		}
	})
	tick := time.NewTicker(time.Second)
	defer tick.Stop()

	for {
		select {
		case <-a.ctx.Done():
			p.Close()
			a.win.Update(func() { a.surface.Close(); close(a.playerDone) })
			return
		case job := <-a.playerOps:
			job(p)
		case <-tick.C:
			a.acceptStats(p.Stats())
		}
	}
}

func (a *app) doPlayer(operation func(*player.Player) error, after func(error)) {
	if a.player == nil || a.closing {
		return
	}
	a.playerOps <- func(p *player.Player) {
		err := operation(p)
		if after != nil {
			a.win.Update(func() {
				if !a.closing {
					after(err)
				}
			})
		}
	}
}

func (a *app) platform() platforms.Platform {
	if a.platformLabel == platforms.FLive.Name() {
		return platforms.FLive
	}
	if a.platformLabel == "抖音" {
		return platforms.Douyin
	}
	if a.platformLabel == "企鹅体育" {
		return platforms.Qie
	}
	if a.platformLabel == "虎牙" {
		return platforms.Huya
	}
	if a.platformLabel == "斗鱼" {
		return platforms.Douyu
	}
	return platforms.Bilibili
}

func (a *app) resolve(source platforms.Platform, input string, quality int, route ...string) {
	a.resolveRoom(source, input, quality, roomOpenOptions{}, route...)
}

type roomOpenOptions struct {
	keepBrowsing    bool
	rememberQuality bool
}

func (a *app) resolveRoom(source platforms.Platform, input string, quality int, opts roomOpenOptions, route ...string) {
	if a.reconnectTimer != nil {
		a.reconnectTimer.Stop()
		a.reconnectTimer = nil
	}
	p, id, err := platforms.ParseRoom(source, input)
	if err != nil {
		a.status = err.Error()
		a.toast = err.Error()
		return
	}
	a.stopRecording()
	if a.page != "player" && !opts.keepBrowsing {
		a.rememberHomeCatalog()
		a.returnPage = a.page
		a.focusMode = true
	}
	a.revealControls()
	if a.stream == nil || a.stream.Room.Platform != p || a.stream.Room.ID != id {
		a.reconnectAttempts = 0
		a.pausedOnLeave = false
		if a.shown {
			a.command("stop")
		}
		a.stream = nil
		a.qualityLabel = ""
		a.resetChat("等待直播间连接")
		a.clearFlyingChat()
		a.shown = false
		a.updateSystemPlayback()
		a.firstFrameMS = 0
		if a.targetRoom.Platform != p || a.targetRoom.ID != id {
			a.selectTargetRoom(platforms.Room{Platform: p, ID: id, Title: "直播间 " + id})
		}
		a.title = a.targetRoom.Title
	}
	if !opts.keepBrowsing {
		fromMonitor := a.page == "monitor"
		a.page = "player"
		if fromMonitor {
			a.syncMonitorPlayback()
		}
		a.transportHandle.Focus()
	}
	if a.resolveCancel != nil {
		a.resolveCancel()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.resolveCancel = cancel
	a.generation++
	gen := a.generation
	a.busy = true
	a.status = "正在解析" + p.Name() + "直播间"
	preference := platforms.QualityHighest
	var roomPreferences map[string]library.RoomPlaybackPreferences
	if a.library != nil {
		preference = platforms.NormalizeQualityPreference(a.library.Preferences.DefaultQuality)
		if !a.library.Preferences.DisableRoomPlaybackMemory {
			roomPreferences = a.library.Preferences.RoomPlayback
		}
	}
	go func() {
		s, err := a.client.Resolve(ctx, p, id, quality, route...)
		if err == nil && quality < 0 {
			if preferred, ok := preferredRoomQuality(s, roomPreferences, preference); ok && preferred != s.QualityID {
				// 平台默认档位不代表偏好档位，按偏好再取一次；平台拒绝时保留第一次结果。
				if next, retryErr := a.client.Resolve(ctx, p, id, preferred, route...); retryErr == nil {
					s = next
				}
			}
		}
		a.win.Update(func() {
			if gen != a.generation || a.closing {
				return
			}
			if err != nil {
				a.busy = false
				a.status = err.Error()
				if roomOfflineStatus(err.Error()) {
					a.removeOfflineRecommendation(p, id)
				}
				a.recordFailure("room", err.Error())
				if s != nil && s.Room.Platform == platforms.FLive {
					a.stream = s
					a.targetRoom, a.title = s.Room, s.Room.Title
				}
				if a.reconnectAttempts > 0 {
					a.scheduleReconnect()
				}
				return
			}
			// 房间身份先固定，再进入视频连接；标题、占位卡片和弹幕使用同一房间。
			if a.targetRoom.Platform == s.Room.Platform && a.targetRoom.ID == s.Room.ID {
				if s.Room.Cover == "" {
					s.Room.Cover = a.targetRoom.Cover
				}
				if s.Room.Anchor == "" {
					s.Room.Anchor = a.targetRoom.Anchor
				}
			}
			for _, card := range a.catalog {
				if card.Platform == s.Room.Platform && card.ID == s.Room.ID {
					if s.Room.Cover == "" {
						s.Room.Cover = card.Cover
					}
					if s.Room.Anchor == "" {
						s.Room.Anchor = card.Anchor
					}
					break
				}
			}
			a.stream = s
			a.targetRoom, a.title = s.Room, s.Room.Title
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
					a.status = err.Error()
					a.recordFailure("video", err.Error())
					a.shown = false
					a.updateSystemPlayback()
					a.scheduleReconnect()
					return
				}
				if !opts.keepBrowsing {
					a.page = "player"
				}
				if a.library != nil && a.monitorKey == "" {
					var err error
					if a.embedded || a.hasSideViewer() && a.sideFeatured {
						err = a.library.Watch(s.Room)
					} else {
						err = a.library.WatchSession(s.Room, s.QualityID, s.SourceID)
					}
					if err != nil {
						log.Print("保存观看历史失败：", err)
					}
				}
				a.qualityLabel = ""
				for _, q := range s.Qualities {
					if q.ID == s.QualityID {
						a.qualityLabel = q.Name
					}
				}
				rememberQuality := opts.rememberQuality && s.QualityID == quality
				if rememberQuality || a.roomDanmaku != nil || a.roomChatPanel != nil {
					a.rememberRoomPlayback(rememberQuality)
				}
				if quality >= 0 && s.QualityID != quality {
					actual := a.qualityLabel
					if actual == "" {
						actual = "可用清晰度"
					}
					a.toast = "所选清晰度暂不可用，已切换到" + actual
				}
				a.shown = true
				a.stats = player.Stats{Muted: a.stats.Muted}
				a.publishMediaInfo()
				a.revealControls()
				a.playStarted = time.Now()
				a.firstFrameMS = 0
				a.decodedMS = 0

				a.status = "正在连接直播流"
				if !opts.keepBrowsing {
					a.transportHandle.Focus()
				}
				if a.monitorKey != "" || a.rootApp().featuredViewer() == a || a.parent != nil && !a.embedded && !a.companion {
					a.startChat(false)
				}
			})
		})
	}()
}

func (a *app) playFile(path string) {
	if !filepath.IsAbs(path) {
		absolute, err := filepath.Abs(path)
		if err != nil {
			a.status = err.Error()
			return
		}
		path = absolute
	}
	a.pausedOnLeave = false
	if a.surface != nil {
		a.renderedAtStart = a.surface.Metrics().RenderedFrames
	}
	a.doPlayer(func(p *player.Player) error { return p.Load(path, nil) }, func(err error) {
		if err != nil {
			a.status = err.Error()
			return
		}
		a.title = "本地音视频验证"
		a.page = "player"
		a.shown = true
		a.publishMediaInfo()
		a.playStarted = time.Now()
		a.status = "正在载入测试视频"
	})
}
func (a *app) command(args ...string) {
	a.doPlayer(func(p *player.Player) error { return p.Command(args...) }, func(err error) {
		if err != nil {
			a.status = err.Error()
		}
	})
}
func (a *app) stop() {
	a.closeSystemPicture()
	if a.parent == nil {
		a.closeSideViewer(false)
	}
	a.cancelReturnCapture()
	a.cardOpenAt = time.Time{}
	a.returnFrame = nil
	a.stopRecording()
	a.generation++
	if a.resolveCancel != nil {
		a.resolveCancel()
	}
	if a.reconnectTimer != nil {
		a.reconnectTimer.Stop()
		a.reconnectTimer = nil
	}
	a.busy = false
	a.resetChat("已停止")
	a.clearFlyingChat()
	a.command("stop")
	a.shown = false
	a.stream = nil
	a.targetRoom = platforms.Room{}
	a.roomDanmaku, a.roomChatPanel = nil, nil
	a.title = "输入房间号或直播间链接"
	a.status = "已停止"
	a.updateSystemPlayback()
}

func (a *app) acceptStats(stats player.Stats) {
	a.win.Update(func() {
		if a.player == nil || a.closing {
			return
		}
		if a.surface != nil {
			a.surface.Show(a.surfaceShouldShow())
		}
		wasPaused := a.stats.Paused
		a.stats = stats
		if wasPaused != stats.Paused {
			a.updateSystemPlayback()
		}
		if a.opts.report != "" {
			a.samples = append(a.samples, a.stats)
		}
		if a.busy {
			return
		}
		if a.stats.Error != "" {
			a.stopRecording()
			if a.shown || a.status != a.stats.Error {
				a.recordFailure("video", a.stats.Error)
			}
			a.status = a.stats.Error
			a.shown = false
			a.scheduleReconnect()
			a.updateSystemPlayback()
			return
		}
		if a.audioOnly() && a.shown && a.stats.Idle {
			a.stopRecording()
			a.status = "音频连接已结束"
			a.shown = false
			a.updateSystemPlayback()
			a.recordFailure("audio", a.status)
			a.scheduleReconnect()
			return
		}
		if a.shown && a.stats.Idle && a.decodedMS > 0 {
			a.stopRecording()
			a.status = "播放已结束"
			a.shown = false
			a.updateSystemPlayback()
			a.scheduleReconnect()
			return
		}
		if a.shown && a.stats.Position > 0 && (a.stats.Width > 0 || a.audioOnly() && a.stats.AudioCodec != "") {
			if time.Since(a.playStarted) > 30*time.Second {
				a.reconnectAttempts = 0
			}
			if a.decodedMS == 0 {
				a.decodedMS = time.Since(a.playStarted).Milliseconds()
				a.fitWindowToVideo(true)
			}
			if a.firstFrameMS == 0 && a.surface != nil && a.stats.Visible && a.surface.Metrics().RenderedFrames > a.renderedAtStart {
				a.firstFrameMS = time.Since(a.playStarted).Milliseconds()
			}
			if a.stats.Paused {
				a.status = "已暂停"
			} else {
				a.status = "播放中"
			}
		}
	})
}

func (a *app) quit() {
	if a.closing {
		return
	}
	a.closeSystemPicture()
	a.closing = true
	if a.recordingTimer != nil {
		a.recordingTimer.Stop()
		a.recordingTimer = nil
	}
	children := append([]*app(nil), a.extraWindows...)
	children = append(children, a.monitorViewers...)
	if a.sideViewer != nil {
		children = append(children, a.sideViewer)
	}
	for _, child := range children {
		child.quit()
	}
	if a.mediaCommandsOff != nil {
		a.mediaCommandsOff()
		a.mediaCommandsOff = nil
		mygo.NowPlaying.Clear()
	}
	a.cancelSleep()
	if a.notificationTimer != nil {
		a.notificationTimer.Stop()
		a.notificationTimer = nil
	}
	if a.reconnectTimer != nil {
		a.reconnectTimer.Stop()
	}
	if a.aspectTimer != nil {
		a.aspectTimer.Stop()
	}
	if a.preferencesTimer != nil {
		a.preferencesTimer.Stop()
		if a.library != nil {
			if err := a.library.SetPreferences(a.library.Preferences); err != nil {
				log.Print("保存偏好设置失败：", err)
			}
		}
	}
	if a.library != nil && !a.embedded {
		prefs := a.library.Preferences
		if !a.opts.mute {
			prefs.Volume = a.volume
		}
		if err := a.library.SetPreferences(prefs); err != nil {
			log.Print("保存偏好设置失败：", err)
		}
	}
	a.cancel()
	a.writeReport()
	go func() {
		if a.playerDone != nil {
			<-a.playerDone
		}
		for _, child := range children {
			if child.playerDone != nil {
				<-child.playerDone
			}
		}
		a.win.Update(func() {
			a.clean = true
			if a.monitorKey != "" {
				a.parent.removeMonitorViewer(a)
			} else if a.embedded {
				a.parent.removeSideViewer(a)
			} else if a.parent != nil {
				a.win.Close()
			} else if a.restartAfterClose {
				mygo.App.Relaunch()
			} else {
				mygo.App.Quit()
			}
		})
	}()
}

func (a *app) writeReport() {
	if a.opts.report == "" {
		return
	}
	userCPU, systemCPU, maxRSS := processUsage()
	report := struct {
		Started      time.Time            `json:"started"`
		Elapsed      float64              `json:"elapsed_seconds"`
		DecodedMS    int64                `json:"decoded_ready_observed_ms"`
		FirstFrameMS int64                `json:"first_frame_observed_ms"`
		Stream       *platforms.Stream    `json:"stream,omitempty"`
		Samples      []player.Stats       `json:"samples"`
		UserCPU      float64              `json:"user_cpu_seconds"`
		SystemCPU    float64              `json:"system_cpu_seconds"`
		MaxRSS       int64                `json:"max_rss_bytes"`
		Status       string               `json:"status"`
		Rendering    player.RenderMetrics `json:"rendering"`
	}{a.started, time.Since(a.started).Seconds(), a.decodedMS, a.firstFrameMS, a.stream, a.samples, userCPU, systemCPU, maxRSS, a.status, player.RenderMetrics{}}
	if a.surface != nil {
		report.Rendering = a.surface.Metrics()
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err == nil {
		err = os.MkdirAll(filepath.Dir(a.opts.report), 0755)
	}
	if err == nil {
		err = os.WriteFile(a.opts.report, append(data, '\n'), 0600)
	}
	if err != nil {
		log.Print("保存验证报告失败：", err)
	}
}

func (a *app) refreshAccountStatus() {
	for _, p := range []platforms.Platform{platforms.Bilibili, platforms.Douyu, platforms.Douyin, platforms.Qie} {
		cookies, err := a.vault.Load(p)
		if err != nil {
			a.accountStatus[p] = "读取失败"
		} else if accounts.HasLogin(p, cookies) {
			a.accountStatus[p] = fmt.Sprintf("已保存 · %d 项 Cookie", len(cookies))
		} else {
			a.accountStatus[p] = "未登录"
		}
	}
}
