//go:build darwin || windows

package app

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"live-mygo/internal/platforms"
)

func (a *app) view(c *ui.Context) {
	if a.pendingPage != "" {
		page := a.pendingPage
		a.pendingPage = ""
		a.openPage(page)
	}
	if a.win != nil && (!a.iconSet || a.iconDark != c.Theme().Dark) {
		a.iconSet = true
		a.iconDark = c.Theme().Dark
		png := appIconPNG
		if a.iconDark {
			png = appIconDarkPNG
		}
		_ = mygo.App.Dock.SetIcon(png)
		_ = a.win.SetIcon(png)
	}
	a.applyTheme(c)
	defer a.quickSearchView(c)
	defer func() {
		if a.toast != "" {
			c.Toast(a.toast)
			a.toast = ""
		}
	}()
	fullScreen := a.page == "player" && a.win != nil && a.win.IsFullScreen()
	if fullScreen {
		c.SetTheme(ui.DarkTheme())
	}
	p := colors(c.Theme())
	if c.Vibrancy() || a.page == "player" {
		c.Root().Background(ui.Transparent)
	}
	c.OnShortcut(ui.Cmd, ui.KeyF, func() {
		if a.page == "player" {
			a.openQuickSearch()
		} else {
			a.searchHandle.Focus(c)
		}
	})
	c.OnShortcut(ui.Cmd, ui.KeyL, a.openQuickSearch)
	c.OnShortcut(ui.Cmd, ui.KeyK, a.openQuickSearch)
	c.OnShortcut(ui.Cmd, ui.KeyComma, func() { a.settingsWindow(0) })
	c.OnShortcut(ui.Cmd, ui.KeyW, func() { a.win.Close() })
	c.OnShortcut(fullscreenModifiers, fullscreenKey, func() {
		if a.page == "player" {
			a.win.ToggleFullScreen()
		}
	})
	for i, page := range []string{"bilibili", "douyu", "favorites", "history"} {
		target := page
		c.OnShortcut(ui.Cmd, ui.Key1+ui.Key(i), func() { a.openPage(target) })
	}
	c.OnShortcut(ui.Cmd, ui.KeyR, func() {
		if a.page == "player" {
			a.reloadStream()
		} else if a.page == "favorites" {
			a.refreshFavorites(true)
		} else if a.page == "home" || a.page == "huya" || a.page == "bilibili" || a.page == "douyu" || a.page == "douyin" || a.page == "qie" || a.page == string(platforms.FLive) {
			a.loadCatalog(1)
		}
	})
	if !a.searchHandle.Focused(c) && a.page == "player" {
		c.OnShortcut(0, ui.KeySpace, a.togglePause)
		c.OnShortcut(0, ui.KeyK, func() { a.revealControls(); a.focusControls = true; a.controlsKeyboard = true })
		c.OnShortcut(0, ui.KeyM, a.toggleMute)
		c.OnShortcut(0, ui.KeyD, func() { a.toggleBarrage(); a.revealControls() })
		c.OnShortcut(0, ui.KeyEscape, func() {
			if a.companion {
				a.toggleCompanion()
				return
			}
			if a.playerMenuOpen() {
				a.closePlayerMenus()
				a.revealControls()
			} else if a.win.IsFullScreen() {
				a.win.SetFullScreen(false)
			} else {
				a.backToBrowse()
			}
		})
	}
	if a.companion {
		a.companionView(c)
		return
	}
	if fullScreen {
		a.navMeasured = true
		a.navWidth = 0
		a.contentView(c)
		return
	}
	desktop := ui.Row(c.Key("desktop-layout")).Fill().AlignItems(ui.Stretch)
	target := sidebarWidth
	if a.focusMode {
		target = 0
	}
	duration := 220 * time.Millisecond
	if a.page == "player" && a.library != nil && a.library.Preferences.FollowVideo {
		duration = time.Nanosecond
	}
	if a.page != "player" {
		// 浏览页同样滑出侧栏，而不是瞬间切换宽度。
		duration = 200 * time.Millisecond
	}
	a.navWidth = desktop.AnimateTo("navigation-width", target, ui.Animation{Duration: duration, Ease: ui.EaseInOut})
	a.navMeasured = true
	desktop.Children(func() {
		side := ui.Column(c.Key("navigation")).Width(a.navWidth).Shrink(0).FillHeight().Clip().Hidden(a.navWidth < .1)
		if !c.Vibrancy() || a.customAppearance() {
			side.Background(p.sidebar)
		}
		side.Children(func() { ui.Box(c).Width(sidebarWidth).FillHeight().Shrink(0).Children(func() { a.sidebar(c) }) })
		edge := float32(0)
		if a.navWidth > .1 && a.page != "player" {
			edge = 1
		}
		content := ui.Box(c.Key("content")).Grow(1).MinWidth(0).BorderWidth(0, 0, 0, edge).BorderColor(p.line)
		if a.page != "player" {
			content.Background(p.bg)
		}
		content.Children(func() {
			if a.page == "player" {
				a.contentView(c)
			} else {
				if a.surface != nil {
					a.surface.Show(false)
				}
				a.browseView(c)
			}
		})
	})
	if a.page == "player" {
		// 标题栏跨过侧栏边界，导航按钮放在标题左侧，不挤占播放器的对齐线。
		ui.Box(c.Key("watch-header-overlay")).Absolute().Left(0).Right(0).Top(0).Height(watchHeaderHeight).Children(func() { a.watchHeader(c) })
	} else {
		if iconButton(c.Key("browse-sidebar"), iconSidebar, "显示 / 隐藏导航侧栏", !a.focusMode).Absolute().Left(browseSidebarLeft(c)).Top((watchHeaderHeight - 30) / 2).Clicked() {
			a.focusMode = !a.focusMode
		}
	}
}

func (a *app) sidebar(c *ui.Context) {
	p := colors(c.Theme())
	top := max(c.TitleBar().Height, float32(38)) + 12
	ui.Column(c).Fill().Padding(top, 14, 14, 14).Gap(19).DragWindow().Children(func() {
		ui.Row(c).Height(32).Radius(8).Padding(0, 8).Gap(10).AlignItems(ui.Center).Background(p.hover).Children(func() {
			ui.Box(c).Size(20, 20).Shrink(0).Center().Children(func() {
				ui.Icon(c, iconSearch).Size(14, 14).TextColor(p.muted)
			})
			if ui.TextInputBase(c, &a.roomInput).Bind(&a.searchHandle).Placeholder("搜索直播").Label("搜索主播、房间号或直播间链接").Tooltip(shortcutLabel("输入主播名搜索，或粘贴直播间链接 · ⌘F")).FontSize(12.5).Height(30).Grow(1).Submitted() {
				a.submitSearch()
			}
			if a.roomInput != "" && iconButton(c.Key("clear-search"), iconClose, "清除搜索").Size(20, 20).Clicked() {
				a.roomInput = ""
				if a.catalogQuery != "" && a.page != "player" {
					a.clearSearch()
				}
			}
		})
		ui.Scroll(c.Key("navigation")).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Gap(3).Children(func() {
				a.navItem(c, "home", "首页", iconHome, 0)
				a.navItem(c, "huya", "虎牙", logoHuya, 0)
				a.navItem(c, "bilibili", "哔哩哔哩", logoBilibili, 0)
				a.navItem(c, "douyu", "斗鱼", logoDouyu, 0)
				a.navItem(c, "douyin", "抖音", logoDouyin, 0)
				a.navItem(c, "qie", "企鹅体育", logoQie, 0)
				if a.raceMenuUnlocked() {
					a.navItem(c, string(platforms.FLive), platforms.FLive.Name(), logoFLive, 0)
				}
				ui.Box(c).Height(15)
				count := 0
				if a.library != nil {
					count = len(a.library.Favorites)
				}
				a.navItem(c, "favorites", "关注", iconHeart, count)
				a.navItem(c, "history", "最近观看", iconHistory, 0)
			})
		})
		if a.stream != nil || a.targetRoom.ID != "" {
			a.nowPlayingCard(c)
		}
		ui.Box(c).Height(1).Margin(0, 6).Background(p.line)
		row := ui.ButtonBase(c.Key("accounts")).Height(35).PaddingX(8).Gap(10).Radius(8).AlignItems(ui.Center).Label("设置")
		bg := ui.Transparent
		if row.Hovered() {
			bg = p.hover
		}
		row.Background(row.AnimateColor("bg", bg, ui.Animation{Duration: 110 * time.Millisecond}))
		row.Children(func() {
			ui.Box(c).Size(20, 20).Shrink(0).Center().Children(func() {
				ui.Icon(c, iconSettings).Size(17, 17).TextColor(p.muted)
			})
			ui.Text(c, "设置").FontSize(12.5).TextColor(p.text).Grow(1)
			ui.Icon(c, iconChevron).Size(12, 12).TextColor(p.faint)
		})
		if row.Clicked() {
			a.settingsWindow(0)
		}
	})
}

func (a *app) navItem(c *ui.Context, id, label string, icon *ui.SVG, count int) {
	p := colors(c.Theme())
	b := ui.ButtonBase(c.Key(id)).Height(35).PaddingX(8).Gap(10).Radius(8).AlignItems(ui.Center).Label(label)
	bg := ui.Transparent
	ink := p.muted
	if a.page == id {
		bg = p.selected
		ink = p.accent
	} else if b.Hovered() {
		bg = p.hover
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	b.Children(func() {
		ui.Box(c).Size(20, 20).Shrink(0).Center().Children(func() {
			size := float32(17)
			if id == "bilibili" || id == "douyu" || id == "huya" || id == "douyin" || id == "qie" || id == string(platforms.FLive) {
				size = 20
			}
			ui.Icon(c, icon).Size(size, size).TextColor(ink)
		})
		ui.Text(c, label).FontSize(13).FontWeight(500).TextColor(p.text).Grow(1)
		if count > 0 {
			ui.Textf(c, "%d", count).FontSize(11).TextColor(p.muted)
		}
	})
	if b.Clicked() {
		a.openPage(id)
	}
}

func (a *app) playbackState() string {
	if a.busy || a.shown && !a.playStarted.IsZero() && a.decodedMS == 0 {
		if a.stream == nil {
			return "正在解析房间"
		}
		return "正在连接视频"
	}
	if !a.shown || a.stats.Idle {
		return "连接已中断"
	}
	if a.stats.Paused {
		return "已暂停"
	}
	return "直播中"
}
func (a *app) nowPlayingCard(c *ui.Context) {
	p := colors(c.Theme())
	r := a.targetRoom
	if a.stream != nil {
		r = a.stream.Room
	}
	ui.Column(c.Key("now-playing-card")).Padding(10).Gap(9).Radius(11).Background(p.hover).Border(1, p.line).Children(func() {
		b := ui.ButtonBase(c.Key("now-playing")).FillWidth().Gap(9).AlignItems(ui.Center).Radius(7).Label("返回正在播放的直播间").Tooltip(r.Title)
		b.Children(func() {
			ui.Box(c).Size(40, 40).Radius(7).Clip().Background(p.soft).Children(func() { a.coverImage(c, r.Cover, 7) })
			ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
				ui.Text(c, roomLabel(r)).FontSize(12).FontWeight(600).TextColor(p.text).SingleLine()
				ui.Text(c, r.Title).FontSize(10.5).TextColor(p.muted).SingleLine()
			})
		})
		if b.Clicked() {
			a.returnPage = a.page
			a.page = "player"
			a.focusMode = true
			a.revealControls()
			if a.pausedOnLeave {
				a.pausedOnLeave = false
				if a.shown && a.stats.Paused {
					a.togglePause()
				}
			}
			a.fitWindowToVideo(true)
		}
		ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
			dot := p.faint
			if a.shown && !a.stats.Paused && !a.busy && (a.decodedMS > 0 || a.playStarted.IsZero()) {
				dot = ui.Hex("#5ba789")
			}
			ui.Box(c).Size(4, 4).Radius(2).Background(dot)
			ui.Text(c, a.playbackState()).FontSize(10).TextColor(p.muted).Grow(1)
			if iconButton(c.Key("stop-current"), iconClose, "关闭当前直播").Size(24, 24).Clicked() {
				a.closePlayback()
			}
			icon, label := iconPause, "暂停当前直播"
			if a.stats.Paused {
				icon, label = iconPlay, "继续当前直播"
			}
			if iconButton(c, icon, label).Size(26, 26).Disabled(!a.shown).Clicked() {
				a.togglePause()
			}
			icon, label = iconVolume, "静音当前直播"
			if a.stats.Muted || a.volume == 0 {
				icon, label = iconMuted, "取消当前直播静音"
			}
			if iconButton(c, icon, label).Size(26, 26).Disabled(!a.shown).Clicked() {
				a.toggleMute()
			}
			if iconButton(c, iconRefresh, "重连当前直播").Size(26, 26).Disabled(a.busy).Clicked() {
				if a.stream != nil {
					a.reloadStreamInPlace()
				} else {
					a.resolveRoom(r.Platform, r.ID, -1, true)
				}
			}
		})
	})
}

func (a *app) openPage(page string) {
	if page == string(platforms.FLive) && !a.raceMenuUnlocked() {
		return
	}
	if a.companion {
		a.toggleCompanion()
	}
	if a.win != nil {
		a.normalMinimum()
	}
	if page == a.page && a.catalogQuery == "" {
		return
	}
	a.leavePlayer()
	a.page = page
	// 首页和收藏页都优先展示关注的开播状态；冷却时间避免每次切页都请求。
	a.refreshFavorites(false)
	a.grid = ui.ListState{}
	if page == "home" || page == "huya" || page == "bilibili" || page == "douyu" || page == "douyin" || page == "qie" || page == string(platforms.FLive) {
		if page == "douyin" {
			a.platformLabel = "抖音"
		} else if page == "qie" {
			a.platformLabel = "企鹅体育"
		} else if page == "huya" {
			a.platformLabel = "虎牙"
		} else if page == "douyu" {
			a.platformLabel = "斗鱼"
		} else {
			a.platformLabel = "哔哩哔哩"
		}
		if page == string(platforms.FLive) {
			a.platformLabel = platforms.FLive.Name()
		}
		a.catalogQuery = ""
		a.category = platforms.Category{}
		a.roomInput = ""
		a.catalog = nil
		a.loadCatalog(1)
		a.loadCategories(a.platform())
	}
}

func (a *app) backToBrowse() {
	if a.win != nil {
		a.normalMinimum()
	}
	a.leavePlayer()
	page := a.returnPage
	if page == "" || page == "player" {
		page = string(a.platform())
	}
	a.page = page
	// 从播放页返回浏览时恢复侧栏，搜索框和导航不必再靠左上角开关找回。
	a.focusMode = false
	if (page == "home" || page == "bilibili" || page == "douyu" || page == "huya" || page == "douyin" || page == "qie" || page == string(platforms.FLive)) && len(a.catalog) == 0 && !a.catalogLoading {
		a.loadCatalog(1)
	}
	a.refreshFavorites(false)
}

func (a *app) submitSearch() {
	query := strings.TrimSpace(a.roomInput)
	if query == "" {
		return
	}
	_, numericErr := strconv.ParseUint(query, 10, 64)
	if numericErr == nil || strings.Contains(query, "://") {
		if a.player != nil {
			a.resolve(a.platform(), query, -1)
		} else {
			a.toast = "播放器正在初始化"
		}
		return
	}
	a.page = string(a.platform())
	a.catalogQuery = query
	a.catalog = nil
	a.grid = ui.ListState{}
	a.loadCatalog(1)
}

func (a *app) playRoom(r platforms.Room) {
	a.targetRoom = r
	a.resolve(r.Platform, r.ID, -1)
}
func (a *app) toggleFavorite(r platforms.Room) {
	if a.library == nil {
		return
	}
	had := a.library.Has(r)
	if err := a.library.Toggle(r); err != nil {
		a.toast = "关注未能保存，请稍后重试"
		return
	}
	if had {
		a.toast = "已取消关注"
	} else {
		a.libraryRefresh = time.Time{}
		a.toast = "已关注 " + r.Anchor
		if r.Anchor == "" {
			a.toast = "已关注该直播间"
		}
	}
}

func (a *app) loadCatalog(page int) {
	if a.page == "home" {
		a.loadHome()
		return
	}
	if a.catalogCancel != nil {
		a.catalogCancel()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.catalogCancel = cancel
	a.catalogGeneration++
	gen := a.catalogGeneration
	p, query, category := a.platform(), a.catalogQuery, a.category
	a.catalogLoading = true
	a.catalogError = ""
	go func() {
		var result platforms.Catalog
		var err error
		if query == "" {
			result, err = a.client.CategoryRooms(ctx, p, category, page)
		} else {
			result, err = a.client.Search(ctx, p, query, page)
		}
		a.win.Update(func() {
			if a.closing || gen != a.catalogGeneration {
				return
			}
			a.catalogLoading = false
			if err != nil {
				a.catalogError = err.Error()
				return
			}
			if page == 1 {
				a.catalog = result.Rooms
			} else {
				seen := map[string]bool{}
				for _, r := range a.catalog {
					seen[string(r.Platform)+":"+r.ID] = true
				}
				for _, r := range result.Rooms {
					key := string(r.Platform) + ":" + r.ID
					if !seen[key] {
						a.catalog = append(a.catalog, r)
						seen[key] = true
					}
				}
			}
			a.catalogPage = page
			a.catalogMore = result.HasMore
		})
	}()
}

func (a *app) cover(raw string) *ui.Bitmap {
	if raw == "" || a.ctx == nil || a.win == nil {
		return nil
	}
	if img := a.covers[raw]; img != nil {
		return img
	}
	if a.coverLoading[raw] || time.Now().Before(a.coverRetry[raw]) {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" && u.Scheme != "http" {
		return nil
	}
	a.coverLoading[raw] = true
	go func() {
		var bitmap *ui.Bitmap
		defer func() {
			a.win.Update(func() {
				a.finishCover(raw, bitmap)
			})
		}()
		select {
		case a.coverSlots <- struct{}{}:
		case <-a.ctx.Done():
			return
		}
		defer func() { <-a.coverSlots }()
		bitmap, _ = downloadCover(a.ctx, a.client.HTTP, raw)
	}()
	return nil
}

func (a *app) finishCover(raw string, bitmap *ui.Bitmap) {
	delete(a.coverLoading, raw)
	if a.closing {
		return
	}
	if bitmap == nil {
		if a.coverRetry == nil {
			a.coverRetry = map[string]time.Time{}
		}
		if len(a.coverRetry) >= 96 {
			for key := range a.coverRetry {
				delete(a.coverRetry, key)
				break
			}
		}
		a.coverRetry[raw] = time.Now().Add(30 * time.Second)
		return
	}
	delete(a.coverRetry, raw)
	if len(a.covers) >= 96 && len(a.coverOrder) > 0 {
		oldest := a.coverOrder[0]
		a.coverOrder = a.coverOrder[1:]
		delete(a.covers, oldest)
	}
	a.covers[raw] = bitmap
	a.coverOrder = append(a.coverOrder, raw)
	// 登记后即可丢弃 RGBA 像素；GPU 首次上传或重绘时按需从压缩缩略图重新解码。
	bitmap.ReleasePixels()
}

// 网络或解码失败不会永久占用 loading 标记；短暂失败在冷却后可重新请求。
func downloadCover(ctx context.Context, client *http.Client, raw string) (*ui.Bitmap, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", platforms.UserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("图片请求失败：%d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 8<<20 {
		return nil, fmt.Errorf("图片过大")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 8192 || cfg.Height > 8192 {
		return nil, fmt.Errorf("图片尺寸无效")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	bounds := img.Bounds()
	scale := min(640/float64(bounds.Dx()), 360/float64(bounds.Dy()), 1)
	w, h := max(1, int(float64(bounds.Dx())*scale)), max(1, int(float64(bounds.Dy())*scale))
	thumb := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.ApproxBiLinear.Scale(thumb, thumb.Bounds(), img, bounds, draw.Src, nil)
	data = encodeCover(thumb)
	if data == nil {
		return nil, fmt.Errorf("封面编码失败")
	}
	return ui.DecodeBitmap(data)
}

// encodeCover keeps the thumbnail compressed so that DecodeBitmap can drop
// the RGBA pixels after the GPU uploads them and decode them again on demand.
// Transparency, as emotes have, needs PNG; covers take the much smaller JPEG.
func encodeCover(thumb *image.RGBA) []byte {
	var buf bytes.Buffer
	opaque := true
	for i := 3; i < len(thumb.Pix); i += 4 {
		if thumb.Pix[i] != 255 {
			opaque = false
			break
		}
	}
	var err error
	if opaque {
		err = jpeg.Encode(&buf, thumb, &jpeg.Options{Quality: 90})
	} else {
		err = png.Encode(&buf, thumb)
	}
	if err != nil {
		return nil
	}
	return buf.Bytes()
}

func roomLabel(r platforms.Room) string {
	if r.Anchor != "" {
		return r.Anchor
	}
	return fmt.Sprintf("%s · %s", r.Platform.Name(), r.ID)
}
