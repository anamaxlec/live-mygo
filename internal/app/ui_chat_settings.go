//go:build darwin

package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"live-mygo/internal/library"
	"live-mygo/internal/platforms"
)

func compactSwitch(c *ui.Context, label string, value bool, save func(bool)) {
	p := colors(c.Theme())
	ui.Row(c).Height(35).PaddingX(3).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(p.text).Grow(1)
		if ui.Switch(c, &value).Label(label).Changed() {
			save(value)
		}
	})
}
func compactSlider(c *ui.Context, label string, value, lo, hi float64, display string, save func(float64)) {
	p := colors(c.Theme())
	ui.Column(c).Gap(6).Padding(8, 3).Children(func() {
		ui.Row(c).Children(func() {
			ui.Text(c, label).FontSize(11.5).TextColor(p.text).Grow(1)
			ui.Text(c, display).FontSize(10.5).TextColor(p.muted)
		})
		if ui.Slider(c, &value, lo, hi).FillWidth().Height(20).Label(label).Changed() {
			save(value)
		}
	})
}

func (a *app) chatSettingsPopover(c *ui.Context, anchor ui.Element) {
	p := colors(c.Theme())
	animatedPopover(c, anchor, &a.chatSettingsOpen, false, func(panel ui.Element) {
		panel.AttachTo(anchor, ui.AnchorBottomEnd, ui.AnchorTopEnd).Width(260).Margin(8, 0, 0, 0)
		panel.Label("弹幕列表选项面板")
		floatingPanel(panel, p)
		_, height := c.Size()
		ui.Scroll(c.Key("chat-preferences")).MaxHeight(max(160, height-170)).Children(func() { a.chatSettingsContent(c, false) })
	})
}

func (a *app) chatSettingsContent(c *ui.Context, full bool) {
	if a.library == nil {
		return
	}
	p := colors(c.Theme())
	prefs := a.library.Preferences
	surface := ui.Column(c).Gap(3)
	if full {
		surface.Radius(10).Background(p.soft).Clip()
	}
	surface.Children(func() {
		if full {
			preferenceSwitch(c, full, "显示弹幕列表", prefs.ChatPanel, func(v bool) {
				next := a.library.Preferences
				next.ChatPanel = v
				a.savePreferences(next)
				a.fitWindowToVideo(false)
			})
			ui.Box(c).Height(1).MarginY(7).Background(p.line)
		} else {
			ui.Text(c, "弹幕列表").FontSize(13).FontWeight(600).TextColor(p.text).Padding(0, 3, 10, 3)
		}
		style := "气泡"
		if prefs.ChatCompact {
			style = "紧凑"
		}
		if preferenceSelect(c, full, "列表样式", &style, []string{"气泡", "紧凑"}) {
			next := a.library.Preferences
			next.ChatCompact = style == "紧凑"
			a.savePreferences(next)
		}
		preferenceSlider(c, full, "文字大小", prefs.ChatSize, 11, 16, fmt.Sprintf("%.0f", prefs.ChatSize), func(v float64) { next := a.library.Preferences; next.ChatSize = v; a.previewPreferences(next) })
		preferenceSwitch(c, full, "显示接收时间", prefs.ChatTime, func(v bool) { next := a.library.Preferences; next.ChatTime = v; a.savePreferences(next) })
		preferenceSwitch(c, full, "显示粉丝牌", prefs.ChatBadges, func(v bool) { next := a.library.Preferences; next.ChatBadges = v; a.savePreferences(next) })
		ui.Box(c).Height(1).MarginY(7).Background(p.line)
		preferenceSwitch(c, full, "显示连接面板", prefs.ChatConnection, func(v bool) { next := a.library.Preferences; next.ChatConnection = v; a.savePreferences(next) })
		preferenceSwitch(c, full, "显示跟随状态", prefs.ChatFollow, func(v bool) { next := a.library.Preferences; next.ChatFollow = v; a.savePreferences(next) })
		preferenceSwitch(c, full, "显示官网互动入口", prefs.ChatOfficial, func(v bool) { next := a.library.Preferences; next.ChatOfficial = v; a.savePreferences(next) })
		ui.Box(c).Height(1).MarginY(7).Background(p.line)
		preferenceSwitch(c, full, "屏蔽规则", prefs.FilterEnabled, func(v bool) {
			next := a.library.Preferences
			next.FilterEnabled = v
			a.savePreferences(next)
			a.clearFlyingChat()
		})
		preferenceSwitch(c, full, "过滤重复消息", prefs.FilterDuplicates, func(v bool) { next := a.library.Preferences; next.FilterDuplicates = v; a.savePreferences(next) })
		if preferenceAction(c, full, "关键词与用户屏蔽").Clicked() {
			a.chatSettingsOpen = false
			a.filterWindow()
		}
		if !full && ui.Button(c, "全部弹幕列表设置").FillWidth().Height(29).FontSize(11.5).Margin(8, 0, 0, 0).Clicked() {
			a.chatSettingsOpen = false
			a.settingsWindow(settingsChat)
		}
	})
}

func (a *app) blockChatUser(msg platforms.ChatMessage) {
	if a.library == nil {
		return
	}
	next := a.library.Preferences
	entry := library.BlockedUser{Platform: a.targetRoom.Platform, ID: msg.UserID, Name: msg.User}
	for _, user := range next.BlockedUsers {
		if user.Platform == entry.Platform && ((entry.ID != "" && user.ID == entry.ID) || (entry.ID == "" && strings.EqualFold(user.Name, entry.Name))) {
			return
		}
	}
	next.BlockedUsers = append(append([]library.BlockedUser(nil), next.BlockedUsers...), entry)
	next.FilterEnabled = true
	a.savePreferences(next)
	a.clearFlyingChat()
	a.toast = "已在本机屏蔽 " + msg.User
}

func (a *app) filterWindow() {
	if a.filterWin != nil && !a.filterWin.IsDestroyed() {
		a.filterWin.Focus()
		a.filterWin.Invalidate()
		return
	}
	tab := 0
	a.filterWin = mygo.NewWindow(mygo.WindowOptions{Title: "弹幕屏蔽 · Live MyGo", Width: 580, Height: 520, MinWidth: 500, MinHeight: 420, TitleBarStyle: mygo.TitleBarHidden, Parent: a.win, BackgroundColor: "light-dark(#ffffff,#171719)", Content: ui.View(func(c *ui.Context) {
		a.applyTheme(c)
		c.OnShortcut(ui.Cmd, ui.KeyW, func() { a.filterWin.Close() })
		c.OnShortcut(0, ui.KeyEscape, func() { a.filterWin.Close() })
		p := colors(c.Theme())
		ui.Column(c).Fill().Background(p.bg).Children(func() {
			ui.Row(c).Height(48).Center().DragWindow().Children(func() { ui.Text(c, "弹幕屏蔽").FontSize(13).FontWeight(600).TextColor(p.text) })
			if a.library == nil {
				return
			}
			ui.Row(c).Padding(8, 24, 15, 24).Gap(8).AlignItems(ui.Center).Children(func() {
				for i, label := range []string{"关键词", "用户"} {
					if filterButton(c.Key(i), label, tab == i).Clicked() {
						tab = i
					}
				}
				ui.Box(c).Grow(1)
				value := a.library.Preferences.FilterEnabled
				ui.Text(c, "启用屏蔽").FontSize(11.5).TextColor(p.muted)
				if ui.Switch(c, &value).Label("启用屏蔽").Changed() {
					next := a.library.Preferences
					next.FilterEnabled = value
					a.savePreferences(next)
					a.clearFlyingChat()
				}
			})
			ui.Box(c).Height(1).Background(p.line)
			if tab == 0 {
				ui.Row(c).Padding(18, 24, 12, 24).Gap(9).Children(func() {
					field := ui.TextInput(c, &a.filterDraft).Placeholder("输入要屏蔽的关键词").Label("屏蔽关键词").FontSize(12.5).Height(32).Grow(1)
					add := ui.Button(c, "添加").Height(32).Disabled(strings.TrimSpace(a.filterDraft) == "")
					if field.Submitted() || add.Clicked() {
						a.addBlockedKeyword()
					}
				})
			}
			ui.Scroll(c.Key(tab)).Grow(1).MinHeight(0).Padding(12, 24, 20, 24).Children(func() {
				ui.Column(c).Gap(7).Children(func() {
					if tab == 0 {
						keywords := a.library.Preferences.BlockedKeywords
						if len(keywords) == 0 {
							ui.Text(c, "暂无屏蔽关键词").FontSize(12).TextColor(p.faint).PaddingY(22)
						}
						for i, word := range keywords {
							ui.Row(c.Key(word)).Height(39).PaddingX(11).Radius(8).Background(p.soft).Gap(10).AlignItems(ui.Center).Children(func() {
								ui.Text(c, word).FontSize(12).TextColor(p.text).Grow(1).MinWidth(0).SingleLine().Tooltip(word)
								if iconButton(c, iconClose, "移除关键词 "+word).Size(24, 24).Clicked() {
									next := a.library.Preferences
									next.BlockedKeywords = append(append([]string(nil), keywords[:i]...), keywords[i+1:]...)
									a.savePreferences(next)
								}
							})
						}
					} else {
						users := a.library.Preferences.BlockedUsers
						if len(users) == 0 {
							ui.Text(c, "在弹幕上点右键可屏蔽用户").FontSize(12).TextColor(p.faint).PaddingY(22)
						}
						for i, user := range users {
							ui.Row(c.Key(string(user.Platform) + user.ID + user.Name)).Height(47).PaddingX(11).Radius(8).Background(p.soft).Gap(10).AlignItems(ui.Center).Children(func() {
								ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
									ui.Text(c, user.Name).FontSize(12).TextColor(p.text).SingleLine()
									ui.Text(c, user.Platform.Name()).FontSize(10).TextColor(p.faint)
								})
								if iconButton(c, iconClose, "取消屏蔽 "+user.Name).Size(24, 24).Clicked() {
									next := a.library.Preferences
									next.BlockedUsers = append(append([]library.BlockedUser(nil), users[:i]...), users[i+1:]...)
									a.savePreferences(next)
								}
							})
						}
					}
				})
			})
			ui.Text(c, "规则同时应用于列表和画面弹幕，仅在此设备生效。").FontSize(10.5).TextColor(p.faint).Padding(13, 24).BorderWidth(1, 0, 0, 0).BorderColor(p.line)
		})
	})})
}

func (a *app) addBlockedKeyword() {
	word := strings.TrimSpace(a.filterDraft)
	if a.library == nil || word == "" {
		return
	}
	if len([]rune(word)) > 80 {
		a.toast = "关键词最多 80 个字"
		return
	}
	next := a.library.Preferences
	for _, old := range next.BlockedKeywords {
		if strings.EqualFold(old, word) {
			a.filterDraft = ""
			return
		}
	}
	next.BlockedKeywords = append(append([]string(nil), next.BlockedKeywords...), word)
	next.FilterEnabled = true
	a.savePreferences(next)
	a.filterDraft = ""
	a.clearFlyingChat()
}
