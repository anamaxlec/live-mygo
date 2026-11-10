//go:build darwin || windows

package app

import (
	"context"
	"errors"
	"github.com/egoist/mygo/ui"
	"live-mygo/internal/platforms"
	"strconv"
	"strings"
	"sync"
	"time"
)

type quickSearchState struct {
	open, focus, loading            bool
	query, platform, err, submitted string
	rooms                           []platforms.Room
	handle                          ui.Handle
	cancel                          context.CancelFunc
	generation                      int
}

const allSearchSources = "全部来源"

type sourceSearchResult struct {
	rooms []platforms.Room
	err   error
}

func searchAcrossSources(ctx context.Context, sources []platforms.Platform, query string, search func(context.Context, platforms.Platform, string) ([]platforms.Room, error)) <-chan sourceSearchResult {
	results := make(chan sourceSearchResult, len(sources))
	var pending sync.WaitGroup
	for _, source := range sources {
		pending.Go(func() {
			rooms, err := search(ctx, source, query)
			results <- sourceSearchResult{rooms, err}
		})
	}
	go func() { pending.Wait(); close(results) }()
	return results
}

func mergeSearchRooms(existing, incoming []platforms.Room) []platforms.Room {
	seen := make(map[string]bool, len(existing))
	for _, room := range existing {
		seen[roomKey(room)] = true
	}
	for _, room := range incoming {
		if !seen[roomKey(room)] {
			existing = append(existing, room)
			seen[roomKey(room)] = true
		}
	}
	return existing
}

func (a *app) localSearchRooms(query string, source platforms.Platform) []platforms.Room {
	if a.library == nil {
		return nil
	}
	query = strings.ToLower(strings.TrimSpace(query))
	var matched []platforms.Room
	for _, rooms := range [][]platforms.Room{a.favoriteRooms(), a.library.History} {
		for _, room := range rooms {
			if source != "" && room.Platform != source {
				continue
			}
			if strings.Contains(strings.ToLower(room.Anchor+" "+room.Title+" "+room.ID+" "+a.library.Details(room).Alias), query) {
				matched = mergeSearchRooms(matched, []platforms.Room{room})
			}
		}
	}
	return matched[:min(8, len(matched))]
}

func (a *app) openQuickSearch() {
	if !a.quick.open {
		a.quick.platform = allSearchSources
		a.quick.query = ""
		a.quick.rooms = nil
		a.quick.err, a.quick.submitted = "", ""
	}
	a.closePlayerMenus()
	a.chatSettingsOpen = false
	a.quick.open, a.quick.focus = true, true
}

func (a *app) closeQuickSearch() {
	a.quick.open = false
	a.quick.loading = false
	a.quick.generation++
	if a.quick.cancel != nil {
		a.quick.cancel()
		a.quick.cancel = nil
	}
}

func directRoomQuery(query string) bool {
	_, err := strconv.ParseUint(query, 10, 64)
	return err == nil || strings.Contains(query, "://")
}

func (a *app) quickRoomPlatform() platforms.Platform {
	p := a.platform()
	if a.page == "player" && a.featuredViewer().targetRoom.Platform != "" {
		p = a.featuredViewer().targetRoom.Platform
	}
	for _, source := range a.sourcePlatforms() {
		if source.Name() == a.quick.platform {
			return source
		}
	}
	return p
}

func (a *app) submitQuickSearch() {
	query := strings.TrimSpace(a.quick.query)
	if query == "" {
		return
	}
	p := a.quickRoomPlatform()
	if directRoomQuery(query) {
		if parsed, _, err := platforms.ParseRoom(p, query); err != nil {
			a.quick.err = err.Error()
			return
		} else {
			p = parsed
		}
		if a.player == nil {
			a.quick.err = "播放器正在初始化"
			return
		}
		a.closeQuickSearch()
		if a.page == "player" {
			a.featuredViewer().resolve(p, query, -1)
		} else {
			a.resolve(p, query, -1)
		}
		return
	}
	if a.quick.cancel != nil {
		a.quick.cancel()
	}
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	a.quick.cancel = cancel
	a.quick.generation++
	gen := a.quick.generation
	a.quick.loading = true
	a.quick.err = ""
	a.quick.submitted = query
	selected := p
	if a.quick.platform == allSearchSources {
		selected = ""
	}
	a.quick.rooms = a.localSearchRooms(query, selected)
	sources := []platforms.Platform{p}
	if selected == "" {
		sources = []platforms.Platform{platforms.Bilibili, platforms.Douyu, platforms.Huya, platforms.Douyin, platforms.Qie}
	}
	go func() {
		defer cancel()
		completed, failures := 0, 0
		for result := range searchAcrossSources(ctx, sources, query, func(ctx context.Context, source platforms.Platform, query string) ([]platforms.Room, error) {
			page, err := a.client.Search(ctx, source, query, 1)
			return page.Rooms[:min(6, len(page.Rooms))], err
		}) {
			completed++
			if result.err != nil && !errors.Is(result.err, context.Canceled) {
				failures++
			}
			done, failed := completed == len(sources), failures
			a.win.Update(func() {
				if a.closing || !a.quick.open || a.quick.generation != gen {
					return
				}
				a.quick.rooms = mergeSearchRooms(a.quick.rooms, result.rooms)
				a.quick.loading = !done
				if done && failed > 0 {
					a.quick.err = "部分来源暂时无法搜索"
					if failed == len(sources) && len(a.quick.rooms) == 0 {
						a.quick.err = "暂时无法搜索，请稍后重试或直接粘贴直播间链接。"
					}
				}
			})
		}
	}()
}

func (a *app) quickSearchView(c *ui.Context) {
	// Overlay 根节点在关闭时仍保留，面板退出不改变底下的页面布局。
	ui.Overlay(c, func() {})
	wasOpen := a.quick.open
	p := colors(c.Theme())
	ui.DialogBase(c.Key("quick-search"), &a.quick.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .26)).Justify(ui.Start).Padding(100, 24, 24, 24).Transition(ui.ElementTransition{Colors: true, Duration: 180 * time.Millisecond, Ease: ui.EaseOut, Enter: &ui.Motion{}, Exit: &ui.Motion{}})
		panel.Width(590).MaxWidthPercent(100).Label("快捷搜索直播")
		panel.TransformOrigin(.5, 0).Transition(ui.ElementTransition{
			Size: true, Duration: 220 * time.Millisecond, Ease: ui.CubicBezier(.18, .9, .3, 1.08),
			Enter: &ui.Motion{Y: -12, Scale: .965}, Exit: &ui.Motion{Y: -8, Scale: .985},
		})
		floatingPanel(panel, p)
		ui.Column(c).Gap(13).Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconSearch).Size(18, 18).TextColor(p.muted)
				input := ui.TextInput(c, &a.quick.query).Bind(&a.quick.handle).Placeholder("搜索主播、房间号或直播间链接").Label("快捷搜索内容").Grow(1).MinWidth(0).Height(34).FontSize(13)
				if a.quick.focus {
					a.quick.handle.Focus(c)
					a.quick.focus = false
				}
				if input.Submitted() {
					a.submitQuickSearch()
				}
				if iconButton(c, iconClose, "关闭快捷搜索 · Esc").Clicked() {
					a.closeQuickSearch()
				}
			})
			ui.Row(c).Gap(9).AlignItems(ui.Center).Children(func() {
				if ui.Select(c, &a.quick.platform, append([]string{allSearchSources}, a.sourceLabels()...)).Width(116).Height(29).FontSize(11.5).Label("快捷搜索平台").Changed() {
					if a.quick.cancel != nil {
						a.quick.cancel()
					}
					a.quick.generation++
					a.quick.loading = false
					a.quick.rooms = nil
					a.quick.submitted = ""
					a.quick.err = ""
				}
				ui.Text(c, shortcutLabel("⌘L / ⌘K")).FontSize(10.5).TextColor(p.faint).Grow(1)
				if a.quick.loading {
					ui.Spinner(c).Size(13, 13)
				}
				label := "搜索"
				if directRoomQuery(strings.TrimSpace(a.quick.query)) {
					label = "打开直播间"
				}
				if ui.Button(c, label).Height(29).FontSize(11.5).Disabled(strings.TrimSpace(a.quick.query) == "" || a.quick.loading).Clicked() {
					a.submitQuickSearch()
				}
			})
			ui.Box(c).Height(1).Background(p.line)
			_, height := c.Size()
			ui.Scroll(c.Key("results")).MaxHeight(max(100, min(360, height-285))).Children(func() {
				ui.Column(c).Gap(4).Children(func() {
					rooms := a.quick.rooms
					if a.quick.submitted == "" && a.quick.err == "" && a.library != nil {
						rooms = a.library.History[:min(4, len(a.library.History))]
						if len(rooms) > 0 {
							ui.Text(c, "最近观看").FontSize(10.5).FontWeight(500).TextColor(p.muted).Padding(2, 8, 5, 8)
						}
					}
					for _, room := range rooms {
						b := ui.ButtonBase(c.Key(roomKey(room))).FillWidth().Height(62).Padding(8).Gap(11).Radius(8).AlignItems(ui.Center).Label("切换到 " + a.displayRoomName(room) + " " + room.Title)
						buttonFeedback(b, p)
						if a.canAddSideRoom(room) {
							b.ContextMenu(func(m *ui.Menu) {
								if m.Item("在副画面播放").Chosen() {
									a.closeQuickSearch()
									a.openSideRoom(c, room)
								}
							})
						}
						b.Children(func() {
							ui.Box(c).Size(64, 40).Radius(6).Clip().Shrink(0).Children(func() { a.coverImage(c, room.Cover, 6); sourceBadge(c, room.Platform, true) })
							ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
								ui.Text(c, a.displayRoomName(room)).FontSize(12).FontWeight(500).TextColor(p.text).SingleLine()
								ui.Text(c, room.Platform.Name()+" · "+room.Title).FontSize(10.5).TextColor(p.muted).SingleLine()
							})
							ui.Icon(c, iconPlay).Size(13, 13).TextColor(p.muted)
						})
						if b.Clicked() {
							a.closeQuickSearch()
							a.playRoom(room)
						}
					}
					message := a.quick.err
					if message == "" && len(rooms) == 0 && !a.quick.loading {
						if a.quick.submitted != "" {
							message = "没有找到直播间，换个主播名或直接输入房间号。"
						} else {
							message = "输入主播名搜索，粘贴链接或房间号直接打开。"
						}
					}
					if message != "" {
						ui.Text(c, message).FontSize(11.5).TextColor(p.muted).MaxLines(3).Padding(10, 8)
					}
				})
			})
		})
	})
	if wasOpen && !a.quick.open {
		a.closeQuickSearch()
	}
}
