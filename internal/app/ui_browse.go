//go:build darwin

package app

import (
	"fmt"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"live-mygo/internal/platforms"
)

func (a *app) browseView(c *ui.Context) {
	p := colors(c.Theme())
	rooms, title := a.catalog, a.platformLabel
	local := a.page == "favorites" || a.page == "history"
	if local {
		title, rooms = "关注", nil
		if a.library != nil {
			rooms = a.favoriteRooms()
		}
		if a.page == "history" {
			title = "最近观看"
			if a.library != nil {
				rooms = a.library.History
			}
		}
		if a.libraryPlatform != "" {
			filtered := make([]platforms.Room, 0, len(rooms))
			for _, r := range rooms {
				if string(r.Platform) == a.libraryPlatform {
					filtered = append(filtered, r)
				}
			}
			rooms = filtered
		}
	} else if a.catalogQuery != "" {
		title = "“" + a.catalogQuery + "”"
	}
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Height(84).Padding(18, contentInset, 10, contentInset).Gap(12).AlignItems(ui.Center).DragWindow().Children(func() {
			ui.Text(c, title).FontSize(23).FontWeight(650).TextColor(p.text).SingleLine().Grow(1).MinWidth(0)
			if local && len(rooms) > 0 {
				ui.Textf(c, "%d 个直播间", len(rooms)).FontSize(12).TextColor(p.muted)
			}
			if a.page == "favorites" {
				if a.libraryRefreshing {
					ui.Spinner(c).Size(14, 14)
				}
				if iconButton(c.Key("refresh-favorites"), iconRefresh, "刷新开播状态 · ⌘R").Disabled(a.libraryRefreshing || len(rooms) == 0).Clicked() {
					a.refreshFavorites(true)
				}
			}
			if !local {
				if a.catalogQuery != "" {
					if ui.Select(c, &a.platformLabel, []string{"哔哩哔哩", "斗鱼"}).Width(100).Height(28).FontSize(12).Label("搜索平台").Changed() {
						a.catalog = nil
						a.grid = ui.ListState{}
						a.loadCatalog(1)
					}
					if iconButton(c.Key("end-search"), iconClose, "结束搜索").Clicked() {
						a.clearSearch()
					}
				}
				if a.catalogLoading {
					ui.Spinner(c).Size(14, 14)
				}
				if iconButton(c.Key("refresh"), iconRefresh, "刷新 · ⌘R").Disabled(a.catalogLoading).Clicked() {
					a.loadCatalog(1)
				}
			}
		})
		if local {
			ui.Row(c).Height(30).Margin(0, contentInset, 20, contentInset).Gap(5).AlignItems(ui.Center).Children(func() {
				for i, label := range []string{"全部", "哔哩哔哩", "斗鱼"} {
					id := []string{"", "bilibili", "douyu"}[i]
					if filterButton(c.Key(id), label, a.libraryPlatform == id).Clicked() {
						a.libraryPlatform = id
						a.grid = ui.ListState{}
					}
				}
			})
		} else if a.catalogQuery == "" {
			a.categoryBar(c)
		}
		if !local && a.catalogQuery == "" && a.category.ID == "" {
			a.discoveryView(c, rooms)
			return
		}
		if !local && a.catalogLoading && len(rooms) == 0 {
			a.roomGrid(c, nil, true, false)
			return
		}
		if !local && a.catalogError != "" && len(rooms) == 0 {
			emptyState(c, iconTV, "暂时无法加载直播", a.catalogError, func() { a.loadCatalog(1) })
			return
		}
		if len(rooms) == 0 {
			heading, detail, icon := "没有找到直播间", "换一个主播名，或直接输入房间号。", iconSearch
			if a.page == "favorites" {
				heading, detail, icon = "还没有关注直播间", "点亮直播间的爱心，即可在这里找到它。", iconHeart
			} else if a.page == "history" {
				heading, detail, icon = "最近观看的直播会留在这里", "打开一个直播间，开始观看。", iconHistory
			}
			if local && a.libraryPlatform != "" {
				heading, detail = "这个平台暂时没有直播间", "切换到全部，查看其他平台的直播间。"
			}
			if local {
				emptyStateAction(c, icon, heading, detail, "浏览直播", func() { a.pendingPage = string(a.platform()); c.After(time.Nanosecond) })
			} else {
				emptyStateAction(c, icon, heading, detail, "快捷搜索直播", a.openQuickSearch)
			}
			return
		}
		if !local && a.catalogError != "" {
			ui.Text(c, a.catalogError).FontSize(12).TextColor(p.danger).Margin(0, contentInset, 14, contentInset)
		}
		a.roomGrid(c, rooms, false, !local && a.catalogMore)
	})
}

func filterButton(c *ui.Context, label string, selected bool, dropdown ...bool) ui.Element {
	p := colors(c.Theme())
	b := ui.ButtonBase(c).Height(28).PaddingX(11).Radius(7).Gap(5).Label(label)
	bg, ink := ui.Transparent, p.muted
	if selected {
		bg, ink = p.selected, p.text
	} else if b.Hovered() || b.FocusVisible() {
		bg = p.hover
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 110 * time.Millisecond}))
	b.Children(func() {
		ui.Text(c, label).FontSize(12).FontWeight(500).TextColor(ink).SingleLine().MaxWidth(160)
		if len(dropdown) > 0 && dropdown[0] {
			ui.Icon(c, iconChevronDown).Size(10, 10).TextColor(p.muted)
		}
	})
	return b
}

func (a *app) categoryBar(c *ui.Context) {
	p := colors(c.Theme())
	groups := a.categories[a.platform()]
	ui.Row(c).Height(30).Margin(0, contentInset, 20, contentInset).Gap(4).AlignItems(ui.Center).Children(func() {
		if filterButton(c.Key("all"), "全部直播", a.category.ID == "").Clicked() {
			a.selectCategory(platforms.Category{})
		}
		if len(groups) == 0 {
			if a.categoryLoading[a.platform()] {
				ui.Spinner(c).Size(12, 12).Margin(0, 7)
			} else if filterButton(c, "加载分区", false).Clicked() {
				a.loadCategories(a.platform())
			}
			return
		}
		for _, group := range groups[:min(5, len(groups))] {
			selected := a.category.ParentID == group.ID
			label := group.Name
			if selected {
				label = a.category.Name
			}
			b := filterButton(c.Key(group.ID), label, selected, true)
			b.Menu(func(m *ui.Menu) {
				for _, sub := range group.Children[:min(12, len(group.Children))] {
					if m.Item(sub.Name).Checked(a.category.ID == sub.ID && a.category.ParentID == sub.ParentID).Chosen() {
						a.selectCategory(sub)
					}
				}
				if len(group.Children) > 12 {
					m.Separator()
					if m.Item("查看全部分区…").Chosen() {
						a.categoryWindow(group.ID)
					}
				}
			})
		}
		if len(groups) > 5 {
			b := ui.ButtonBase(c.Key("more-categories")).Height(28).PaddingX(10).Radius(7).Gap(4).Label("更多分区")
			if b.Hovered() {
				b.Background(p.hover)
			}
			b.Children(func() {
				ui.Text(c, "更多").FontSize(12).TextColor(p.muted)
				ui.Icon(c, iconChevronDown).Size(11, 11).TextColor(p.muted)
			})
			if b.Clicked() {
				a.categoryWindow("")
			}
		}
	})
}

func (a *app) selectCategory(category platforms.Category) {
	if category.ID == a.category.ID && category.ParentID == a.category.ParentID {
		return
	}
	a.category = category
	a.catalog = nil
	a.grid = ui.ListState{}
	a.loadCatalog(1)
}

func (a *app) clearSearch() {
	a.catalogQuery = ""
	a.roomInput = ""
	a.catalog = nil
	a.grid = ui.ListState{}
	a.loadCatalog(1)
}

func (a *app) loadCategories(source platforms.Platform) {
	if a.categoryLoading[source] || len(a.categories[source]) > 0 {
		return
	}
	a.categoryLoading[source] = true
	go func() {
		groups, err := a.client.Categories(a.ctx, source)
		a.win.Update(func() {
			if a.closing {
				return
			}
			a.categoryLoading[source] = false
			if err == nil {
				a.categories[source] = groups
			}
		})
	}()
}

func (a *app) roomGrid(c *ui.Context, rooms []platforms.Room, skeleton, more bool) {
	p := colors(c.Theme())
	w, _ := c.Size()
	available := w - sidebarWidth - contentInset*2 - 1
	const gap float32 = 20
	columns := max(1, int((available+gap)/(236+gap)))
	coverHeight := (available - float32(columns-1)*gap) / float32(columns) * 9 / 16
	count := len(rooms)
	if skeleton {
		count = columns * 2
	}
	rows := (count + columns - 1) / columns
	total := rows
	if more {
		total++
	}
	ui.List(c.Key("room-grid"), &a.grid, total, func(row int) {
		if row == rows {
			ui.Row(c).Height(62).Center().Children(func() {
				label := "加载更多"
				if a.catalogLoading {
					label = "正在加载"
					ui.Spinner(c).Size(12, 12).Margin(0, 8)
				}
				if ui.Button(c, label).Height(29).FontSize(12).Disabled(a.catalogLoading).Clicked() {
					a.loadCatalog(a.catalogPage + 1)
				}
			})
			return
		}
		ui.Row(c).Gap(gap).Padding(0, 0, 25, 0).AlignItems(ui.Stretch).Children(func() {
			for column := range columns {
				i := row*columns + column
				ui.Column(c.Key(column)).Grow(1).Basis(0).MinWidth(0).Children(func() {
					if i >= count {
						return
					}
					if skeleton {
						ui.Column(c).Gap(11).Children(func() {
							ui.Box(c).Height(coverHeight).Radius(9).Background(p.soft)
							ui.Box(c).Height(12).WidthPercent(86).Radius(4).Background(p.soft)
							ui.Box(c).Height(10).WidthPercent(48).Radius(4).Background(p.soft)
							ui.Box(c).Height(10)
						})
					} else {
						a.roomCard(c, rooms[i], coverHeight)
					}
				})
			}
		})
	}).Grow(1).Padding(0, contentInset, 20, contentInset)
}

func (a *app) roomCard(c *ui.Context, r platforms.Room, coverHeight float32) {
	p := colors(c.Theme())
	ui.Column(c.Key(string(r.Platform) + ":" + r.ID)).Gap(6).Children(func() {
		b := ui.ButtonBase(c.Key("open")).Column().AlignItems(ui.Stretch).Justify(ui.Start).WidthPercent(100).MinWidth(0).Gap(9).Radius(9).Label("播放 " + r.Anchor + " " + r.Title).Tooltip(r.Title).Disabled(a.player == nil)
		b.Children(func() {
			ui.Box(c).Height(coverHeight).Radius(9).Background(p.soft).Children(func() {
				a.coverImage(c, r.Cover, 9)
				shown := float32(0)
				if b.Hovered() || b.FocusVisible() {
					shown = 1
				}
				ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Center().Radius(9).Background(ui.RGBA(0, 0, 0, 0.1)).Opacity(b.AnimateTo("play", shown, ui.Animation{Duration: 140 * time.Millisecond})).Children(func() {
					ui.Box(c).Size(36, 36).Radius(18).Background(ui.RGBA(0, 0, 0, 0.48)).Center().Children(func() { ui.Icon(c, iconPlay).Size(17, 17).TextColor(ui.Hex("#ffffff")) })
				})
				if a.page == "favorites" {
					state, ink := "待刷新", p.muted
					if fresh, ok := a.libraryRooms[roomKey(r)]; ok {
						state = "未开播"
						if fresh.Live {
							state = "直播中"
							ink = ui.Hex("#5ba789")
						}
					}
					ui.Text(c, state).FontSize(10).TextColor(ink).Padding(4, 7).Background(p.bg).Radius(5).Absolute().Top(8).Left(8)
				}
				if r.Heat > 0 && (a.page == "bilibili" || a.page == "douyu") {
					ui.Row(c).Absolute().Bottom(8).Right(8).Padding(3, 6).Radius(5).Gap(4).AlignItems(ui.Center).Background(ui.RGBA(0, 0, 0, 0.42)).Tooltip("平台热度").Children(func() {
						ui.Icon(c, iconSignal).Size(11, 11).TextColor(ui.Hex("#ffffff"))
						ui.Text(c, heatLabel(r.Heat)).FontSize(10).TextColor(ui.Hex("#ffffff"))
					})
				}
			})
			ui.Text(c, r.Title).FontSize(13).FontWeight(500).MaxLines(2).Height(34).WidthPercent(100).MinWidth(0).TextColor(p.text)
		})
		if b.Clicked() {
			a.playRoom(r)
		}
		b.ContextMenu(func(m *ui.Menu) { a.roomMenu(c, m, r) })
		ui.Row(c).Height(22).Gap(5).AlignItems(ui.Center).Children(func() {
			if a.page == "favorites" || a.page == "history" {
				icon := iconTV
				if r.Platform == platforms.Douyu {
					icon = iconFish
				}
				ui.Icon(c, icon).Size(12, 12).TextColor(p.faint)
			}
			ui.Text(c, roomLabel(r)).FontSize(11.5).TextColor(p.muted).SingleLine().Grow(1).MinWidth(0)
			if a.library != nil {
				icon, label := iconHeart, "关注 "+roomLabel(r)
				if a.library.Has(r) {
					icon, label = iconHeartFilled, "取消关注 "+roomLabel(r)
				}
				if iconButton(c.Key("follow"), icon, label).Size(25, 22).Clicked() {
					a.toggleFavorite(r)
				}
			}
		})
	})
}

func heatLabel(n int64) string {
	if n >= 10000 {
		return fmt.Sprintf("%.1f万", float64(n)/10000)
	}
	return fmt.Sprint(n)
}

func roomURL(r platforms.Room) string {
	if r.Platform == platforms.Douyu {
		return "https://www.douyu.com/" + r.ID
	}
	return "https://live.bilibili.com/" + r.ID
}

func (a *app) roomMenu(c *ui.Context, m *ui.Menu, r platforms.Room) {
	if m.Item("播放直播间").Disabled(a.player == nil).Chosen() {
		a.playRoom(r)
	}
	if a.library != nil {
		label := "关注直播间"
		if a.library.Has(r) {
			label = "取消关注"
		}
		if m.Item(label).Chosen() {
			a.toggleFavorite(r)
		}
	}
	if !a.coverRetry[r.Cover].IsZero() {
		if m.Item("重新加载封面").Chosen() {
			delete(a.coverRetry, r.Cover)
		}
	}
	m.Separator()
	if m.Item("复制直播间链接").Chosen() {
		mygo.Clipboard.WriteText(roomURL(r))
		a.toast = "已复制直播间链接"
	}
	if m.Item("在官网打开").Chosen() {
		c.OpenURL(roomURL(r))
	}
}
