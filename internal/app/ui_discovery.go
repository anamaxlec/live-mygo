//go:build darwin || windows

package app

import (
	"github.com/egoist/mygo/ui"
	"live-mygo/internal/platforms"
	"time"
)

func (a *app) coverImage(c *ui.Context, raw string, radius ...float32) {
	p := colors(c.Theme())
	image := a.cover(raw)
	target := float32(0)
	if image != nil {
		target = 1
	}
	box := ui.Box(c.Key(raw)).Fill().Radius(radius...).Clip().Background(p.soft)
	opacity := box.Animate("image", target, 180*time.Millisecond)
	box.Children(func() {
		if image == nil {
			ui.Box(c).Fill().Center().Children(func() { ui.Icon(c, iconTV).Size(24, 24).TextColor(p.faint) })
		} else {
			ui.Image(c, image).Fill().Fit(ui.Cover).Radius(radius...).Opacity(opacity)
		}
	})
}

type discoveryRow struct {
	key   string
	build func(*ui.Context)
}

func (a *app) discoveryView(c *ui.Context, rooms []platforms.Room) {
	p := colors(c.Theme())
	width, _ := c.Size()
	available := width - a.contentNavWidth() - a.browseLeftInset(c) - contentInset - 1
	// 列数按侧栏展开时的宽度计算，收起过程中卡片平滑变宽而不跳列。
	columnWidth := width - sidebarWidth - 2*contentInset - 1
	columns := max(1, int((columnWidth+20)/256))
	coverHeight := (available - float32(columns-1)*20) / float32(columns) * 9 / 16
	rows := []discoveryRow{}
	seen := map[string]bool{}
	heading := func(key, title, page string) {
		rows = append(rows, discoveryRow{key, func(c *ui.Context) {
			ui.Row(c).Height(42).AlignItems(ui.Center).Children(func() {
				ui.Text(c, title).FontSize(15).FontWeight(600).TextColor(p.text).Grow(1)
				if page != "" && ui.Button(c, "全部").Label("查看全部"+title).FontSize(10.5).Height(25).Clicked() {
					a.pendingPage = page
					c.After(time.Nanosecond)
				}
			})
		}})
	}
	personal := func(key, title, page string, list []platforms.Room, large bool) {
		if len(list) == 0 {
			return
		}
		heading(key+"-heading", title, page)
		limit := max(1, min(3, int((columnWidth+14)/260)))
		gap, bottom := float32(14), float32(13)
		if large {
			gap, bottom = 20, 22
		}
		list = list[:min(limit, len(list))]
		for _, r := range list {
			seen[roomKey(r)] = true
		}
		rows = append(rows, discoveryRow{key, func(c *ui.Context) {
			ui.Row(c).Gap(gap).Padding(0, 0, bottom, 0).Children(func() {
				for _, r := range list {
					cardWidth := (available - float32(limit-1)*gap) / float32(limit)
					ui.Column(c.Key(roomKey(r))).Width(cardWidth).MinWidth(0).Children(func() {
						if large {
							// 关注中且正在直播的内容与推荐卡片同级，首页第一眼就能看到。
							a.roomCard(c, r, cardWidth*9/16)
							return
						}
						a.personalRoom(c, r, false)
					})
				}
			})
		}})
	}
	live := []platforms.Room{}
	if a.page == "home" {
		// favoriteRooms 会用本次刷新的状态覆盖收藏；刷新尚未完成时仍可用上次保存的开播状态。
		for _, r := range a.favoriteRooms() {
			if r.Live {
				live = append(live, r)
			}
		}
		personal("live", "关注正在直播", "favorites", live, true)
		if a.library != nil {
			recent := []platforms.Room{}
			for _, r := range a.library.History {
				if !seen[roomKey(r)] {
					recent = append(recent, r)
				}
			}
			personal("recent", "最近观看", "history", recent, false)
		}
	}
	recommended := rooms
	recommendationTitle := a.platform().Name() + "推荐"
	if a.page == "home" {
		recommendationTitle = "为你推荐"
	}
	heading("recommend-heading", recommendationTitle, "")
	if len(recommended) == 0 {
		rows = append(rows, discoveryRow{"catalog-state", func(c *ui.Context) {
			if a.catalogLoading {
				ui.Row(c).Height(190).Gap(20).Padding(0, 0, 18, 0).Children(func() {
					for range min(2, columns) {
						ui.Box(c).Grow(1).Height(170).Radius(11).Background(p.soft)
					}
				})
				return
			}
			title := "暂时没有直播间"
			if a.catalogError != "" {
				title = "暂时无法加载推荐"
			}
			ui.Column(c).Height(170).Center().Gap(10).Children(func() {
				ui.Text(c, title).FontSize(13).FontWeight(500).TextColor(p.muted)
				if a.catalogError != "" {
					ui.Text(c, a.catalogError).FontSize(11).TextColor(p.faint).MaxWidth(400).MaxLines(2).TextAlign(ui.Center)
				}
				if ui.Button(c, "重新加载推荐").Height(28).FontSize(11.5).Clicked() {
					a.loadCatalog(1)
				}
			})
		}})
	} else {
		if len(live) == 0 && (a.library == nil || len(a.library.History) == 0) {
			featured := recommended[0]
			recommended = recommended[1:]
			rows = append(rows, discoveryRow{"featured:" + roomKey(featured), func(c *ui.Context) { a.featuredRoom(c, featured, available) }})
		}
		for start := 0; start < len(recommended); start += columns {
			index := start
			end := min(len(recommended), start+columns)
			rows = append(rows, discoveryRow{"recommend:" + roomKey(recommended[start]), func(c *ui.Context) {
				ui.Row(c).Gap(20).Padding(0, 0, 24, 0).AlignItems(ui.Stretch).Children(func() {
					for col := 0; col < columns; col++ {
						ui.Column(c.Key(col)).Grow(1).Basis(0).MinWidth(0).Children(func() {
							if index+col < end {
								a.roomCard(c, recommended[index+col], coverHeight)
							}
						})
					}
				})
			}})
		}
	}
	if a.catalogMore {
		rows = append(rows, discoveryRow{"load-more", func(c *ui.Context) {
			ui.Box(c).Height(60).Center().Children(func() {
				if ui.Button(c, "加载更多直播").FontSize(11.5).Height(29).Disabled(a.catalogLoading).Clicked() {
					a.loadCatalog(a.catalogPage + 1)
				}
			})
		}})
	}
	a.grid.Key = func(i int) any { return rows[i].key }
	// 元信息晚于首页到达时，保持滚动位置，不能把新插入的关注分组藏在视口上方。
	a.grid.KeepOffset = true
	ui.List(c.Key("discovery"), &a.grid, len(rows), func(i int) { rows[i].build(c) }).Grow(1).MinHeight(0).Padding(0, contentInset, 20, a.browseLeftInset(c))
}

func (a *app) personalRoom(c *ui.Context, r platforms.Room, live bool) {
	p := colors(c.Theme())
	b := ui.ButtonBase(c).FillWidth().Height(86).Padding(10).Gap(10).AlignItems(ui.Center).Radius(10).Border(1, p.line).Background(p.bg).Label("打开 " + r.Anchor + " " + r.Title).Disabled(a.player == nil)
	if b.Hovered() {
		b.Background(p.hover)
	}
	b.Children(func() {
		ui.Box(c).Size(80, 50).Radius(7).Clip().Shrink(0).Children(func() { a.coverImage(c, r.Cover, 7); sourceBadge(c, r.Platform, true) })
		ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
			ui.Text(c, roomLabel(r)).FontSize(12).FontWeight(600).TextColor(p.text).SingleLine()
			ui.Text(c, r.Title).FontSize(10.5).TextColor(p.muted).SingleLine()
			if live {
				ui.Row(c).Gap(5).Children(func() {
					ui.Box(c).Size(4, 4).Radius(2).Margin(3, 0, 0, 0).Background(ui.Hex("#5ba789"))
					ui.Text(c, "直播中").FontSize(9.5).TextColor(p.faint)
				})
			}
		})
	})
	if b.Clicked() {
		a.playRoom(r)
	}
	b.ContextMenu(func(m *ui.Menu) { a.roomMenu(c, m, r) })
}
func (a *app) featuredRoom(c *ui.Context, r platforms.Room, width float32) {
	p := colors(c.Theme())
	coverWidth := min(float32(330), width*.47)
	height := coverWidth * 9 / 16
	ui.Box(c).Padding(0, 0, 22, 0).Children(func() {
		b := ui.ButtonBase(c).FillWidth().Height(height).Gap(22).AlignItems(ui.Center).Radius(12).Background(p.soft).Clip().Label("精选直播 " + r.Anchor + " " + r.Title).Disabled(a.player == nil)
		b.Children(func() {
			ui.Box(c).Width(coverWidth).Height(height).Shrink(0).Clip().Children(func() { a.coverImage(c, r.Cover, 12, 0, 0, 12); sourceBadge(c, r.Platform, false) })
			ui.Column(c).Grow(1).MinWidth(0).Padding(16, 20, 16, 0).Gap(10).Children(func() {
				ui.Text(c, r.Title).FontSize(18).FontWeight(600).LineHeight(1.35).MaxLines(3).TextColor(p.text)
				ui.Text(c, roomLabel(r)).FontSize(12).TextColor(p.muted).SingleLine()
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, iconPlay).Size(12, 12).TextColor(p.accent)
					ui.Text(c, "进入直播间").FontSize(11).FontWeight(500).TextColor(p.accent)
				})
			})
		})
		if b.Clicked() {
			a.playRoom(r)
		}
		b.ContextMenu(func(m *ui.Menu) { a.roomMenu(c, m, r) })
	})
}
