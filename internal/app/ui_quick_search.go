//go:build darwin || windows

package app

import (
	"context"
	"github.com/egoist/mygo/ui"
	"live-mygo/internal/platforms"
	"strconv"
	"strings"
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

func (a *app) openQuickSearch() {
	if !a.quick.open {
		a.quick.platform = a.platform().Name()
		if a.page == "player" && a.targetRoom.Platform != "" {
			a.quick.platform = a.targetRoom.Platform.Name()
		}
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

func (a *app) submitQuickSearch() {
	query := strings.TrimSpace(a.quick.query)
	if query == "" {
		return
	}
	p := platforms.Bilibili
	if a.quick.platform == platforms.FLive.Name() {
		p = platforms.FLive
	}
	if a.quick.platform == platforms.Douyin.Name() {
		p = platforms.Douyin
	}
	if a.quick.platform == platforms.Qie.Name() {
		p = platforms.Qie
	}
	if a.quick.platform == platforms.Huya.Name() {
		p = platforms.Huya
	}
	if a.quick.platform == platforms.Douyu.Name() {
		p = platforms.Douyu
	}
	if directRoomQuery(query) {
		if _, _, err := platforms.ParseRoom(p, query); err != nil {
			a.quick.err = err.Error()
			return
		}
		if a.player == nil {
			a.quick.err = "播放器正在初始化"
			return
		}
		a.closeQuickSearch()
		a.resolve(p, query, -1)
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
	a.quick.rooms = nil
	go func() {
		defer cancel()
		result, err := a.client.Search(ctx, p, query, 1)
		a.win.Update(func() {
			if a.closing || !a.quick.open || a.quick.generation != gen {
				return
			}
			a.quick.loading = false
			if err != nil {
				a.quick.err = err.Error()
				return
			}
			a.quick.rooms = result.Rooms[:min(8, len(result.Rooms))]
		})
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
		panel.Transition(ui.ElementTransition{Size: true, Duration: 180 * time.Millisecond, Ease: ui.EaseOut})
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
				if ui.Select(c, &a.quick.platform, a.sourceLabels()).Width(116).Height(29).FontSize(11.5).Label("快捷搜索平台").Changed() {
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
						b := ui.ButtonBase(c.Key(roomKey(room))).FillWidth().Height(62).Padding(8).Gap(11).Radius(8).AlignItems(ui.Center).Label("切换到 " + room.Anchor + " " + room.Title)
						buttonFeedback(b, p)
						b.Children(func() {
							ui.Box(c).Size(64, 40).Radius(6).Clip().Shrink(0).Children(func() { a.coverImage(c, room.Cover, 6) })
							ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
								ui.Text(c, roomLabel(room)).FontSize(12).FontWeight(500).TextColor(p.text).SingleLine()
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
