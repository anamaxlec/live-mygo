//go:build darwin || windows

package app

import (
	"github.com/egoist/mygo/ui"
	"live-mygo/internal/platforms"
)

func (a *app) raceMenuUnlocked() bool { return a.library != nil && a.library.Preferences.RaceMenu }

func (a *app) unlockRaceMenu() bool {
	if a.library == nil || a.raceMenuUnlocked() {
		return false
	}
	a.settings.versionTaps++
	if a.settings.versionTaps < 5 {
		return false
	}
	a.settings.versionTaps = 0
	next := a.library.Preferences
	next.RaceMenu = true
	a.savePreferences(next)
	return a.raceMenuUnlocked()
}

func (a *app) sourcePlatforms() []platforms.Platform {
	out := []platforms.Platform{platforms.Bilibili, platforms.Douyu, platforms.Huya, platforms.Douyin, platforms.Qie}
	if a.raceMenuUnlocked() {
		out = append(out, platforms.FLive)
	}
	return out
}

func (a *app) sourceLabels() []string {
	var out []string
	for _, p := range a.sourcePlatforms() {
		out = append(out, p.Name())
	}
	return out
}

func (a *app) raceCatalog(c *ui.Context, rooms []platforms.Room) {
	if len(rooms) == 0 {
		if a.catalogLoading {
			ui.Box(c).Grow(1).FillWidth().Center().Children(func() { ui.Spinner(c).Size(18, 18) })
		} else {
			emptyState(c, logoFLive, "暂时没有可用频道", a.catalogError, func() { a.loadCatalog(1) })
		}
		return
	}
	p := colors(c.Theme())
	a.grid.Key = func(i int) any { return roomKey(rooms[i]) }
	ui.List(c, &a.grid, len(rooms), func(i int) {
		room := rooms[i]
		ui.Box(c).FillWidth().Padding(0, contentInset, 8, a.browseLeftInset(c)).Children(func() {
			button := ui.ButtonBase(c.Key(roomKey(room))).FillWidth().Height(64).PaddingX(14).Gap(12).Radius(10).AlignItems(ui.Center).Label("播放赛事频道 " + room.Title)
			buttonFeedback(button, p)
			button.Children(func() {
				ui.Box(c).Size(36, 36).Shrink(0).Radius(10).Background(p.soft).Center().Children(func() { ui.Icon(c, logoFLive).Size(23, 23).TextColor(p.muted) })
				ui.Text(c, room.Title).FontSize(13).FontWeight(500).TextColor(p.text).MaxLines(2).Grow(1).MinWidth(0)
				status, ink := "离线", p.faint
				if room.Live {
					status, ink = "在线", p.accent
				}
				ui.Box(c).Size(4, 4).Radius(2).Background(ink)
				ui.Text(c, status).FontSize(11).TextColor(p.muted)
				ui.Icon(c, iconChevron).Size(12, 12).TextColor(p.faint)
			})
			if button.Clicked() && a.player != nil {
				a.resolve(platforms.FLive, room.ID, -1)
			}
		})
	}).Grow(1).MinHeight(0)
}

func (a *app) raceSourceList(c *ui.Context) {
	if id := raceSourceList(c, a.stream, a.busy); id != "" {
		// 选中态立即跟随用户操作；resolve 会取消上一条线路的请求。
		room := a.stream.Room
		a.stream.SourceID = id
		a.resolve(room.Platform, room.ID, -1, id)
	}
}

func raceSourceList(c *ui.Context, stream *platforms.Stream, busy bool) string {
	p := colors(c.Theme())
	if stream == nil || len(stream.Sources) == 0 {
		ui.Column(c).Fill().Center().Gap(10).Padding(24).Children(func() {
			ui.Icon(c, logoFLive).Size(25, 25).TextColor(p.faint)
			ui.Text(c, "选择频道后查看线路").FontSize(12).TextColor(p.muted)
		})
		return ""
	}
	chosen := ""
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).Padding(10).Gap(6).Children(func() {
			for _, source := range stream.Sources {
				selected := source.ID == stream.SourceID
				button := ui.ButtonBase(c.Key(source.ID)).FillWidth().MinHeight(44).Padding(10, 12).Radius(8).Gap(10).AlignItems(ui.Center).Label("切换赛事线路 " + source.Name)
				bg := ui.Transparent
				if selected {
					bg = p.soft
				} else if button.Hovered() || button.FocusVisible() {
					bg = p.hover
				}
				button.Background(bg).FocusRing(true).Children(func() {
					ui.Text(c, source.Name).FontSize(11.5).TextColor(p.text).Grow(1).MinWidth(0).MaxLines(2)
					if selected {
						if busy {
							ui.Spinner(c).Size(13, 13)
						} else {
							ui.Icon(c, iconCheck).Size(13, 13).TextColor(p.accent)
						}
					}
				})
				if button.Clicked() && (!selected || !busy) {
					chosen = source.ID
				}
			}
		})
	})
	return chosen
}
