//go:build darwin

package main

import (
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"live-mygo/internal/platforms"
)

func (a *app) categoryWindow(parent string) {
	if a.categoryWin != nil && !a.categoryWin.IsDestroyed() {
		a.categoryWin.Focus()
		return
	}
	source := a.platform()
	groups := a.categories[source]
	if len(groups) == 0 {
		return
	}
	if parent == "" {
		parent = groups[0].ID
	}
	var query string
	var win *mygo.Window
	win = mygo.NewWindow(mygo.WindowOptions{Title: "选择分区 · Live MyGo", Width: 760, Height: 560, MinWidth: 640, MinHeight: 420, TitleBarStyle: mygo.TitleBarHidden, BackgroundColor: "light-dark(#ffffff,#171719)", Parent: a.win, Content: ui.View(func(c *ui.Context) {
		a.applyTheme(c)
		c.OnShortcut(ui.Cmd, ui.KeyW, func() { win.Close() })
		c.OnShortcut(0, ui.KeyEscape, func() {
			if query != "" {
				query = ""
			} else {
				win.Close()
			}
		})
		p := colors(c.Theme())
		ui.Column(c).Fill().Background(p.bg).Children(func() {
			ui.Row(c).Height(48).DragWindow().Center().Children(func() { ui.Text(c, "选择分区").FontSize(13).FontWeight(600).TextColor(p.text) })
			ui.Row(c).Padding(5, 22, 18, 22).Children(func() {
				ui.Row(c).Grow(1).Height(32).Gap(7).PaddingX(10).Radius(8).Background(p.soft).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, iconSearch).Size(15, 15).TextColor(p.muted)
					ui.TextInput(c, &query).Placeholder("搜索分区").Label("搜索所有分区").FontSize(12.5).Grow(1).Height(30).BorderWidth(0, 0, 0, 0).Background(ui.Transparent)
				})
			})
			ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
				ui.Scroll(c).Width(153).Background(p.soft).Padding(10).Children(func() {
					ui.Column(c).Gap(3).Children(func() {
						for _, group := range groups {
							b := ui.ButtonBase(c.Key(group.ID)).Height(33).Justify(ui.Start).PaddingX(10).Radius(7).Label(group.Name)
							ink := p.muted
							if parent == group.ID && query == "" {
								b.Background(p.selected)
								ink = p.text
							} else if b.Hovered() {
								b.Background(p.hover)
							}
							b.Children(func() { ui.Text(c, group.Name).FontSize(12.5).FontWeight(500).TextColor(ink) })
							if b.Clicked() {
								parent = group.ID
								query = ""
							}
						}
					})
				})
				ui.Scroll(c).Grow(1).MinWidth(0).Padding(14, 22, 24, 22).Children(func() {
					ui.Column(c).Gap(15).Children(func() {
						var choices []platforms.Category
						title := "搜索结果"
						for _, group := range groups {
							if query == "" {
								if group.ID == parent {
									title = group.Name
									choices = group.Children
								}
								continue
							}
							for _, sub := range group.Children {
								if strings.Contains(strings.ToLower(sub.Name), strings.ToLower(strings.TrimSpace(query))) {
									choices = append(choices, sub)
								}
							}
						}
						ui.Text(c, title).FontSize(17).FontWeight(600).TextColor(p.text)
						if len(choices) == 0 {
							ui.Text(c, "没有找到这个分区").FontSize(12.5).TextColor(p.muted)
						}
						ui.Row(c).Wrap().Gap(8).Children(func() {
							for _, sub := range choices {
								b := ui.ButtonBase(c.Key(sub.ParentID+":"+sub.ID)).Padding(8, 11).MinWidth(68).Radius(7).Border(1, p.line).Label(sub.Name)
								ink := p.text
								if a.category.ID == sub.ID && a.category.ParentID == sub.ParentID {
									b.Background(p.accent.Alpha(.09))
									ink = p.accent
								} else if b.Hovered() {
									b.Background(p.hover)
								}
								b.Children(func() { ui.Text(c, sub.Name).FontSize(12).TextColor(ink) })
								if b.Clicked() {
									a.selectCategory(sub)
									a.win.Invalidate()
									win.Close()
								}
							}
						})
					})
				})
			})
		})
	})})
	a.categoryWin = win
}
