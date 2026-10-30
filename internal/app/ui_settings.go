//go:build darwin || windows

package app

import (
	"fmt"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"live-mygo/internal/accounts"
	"live-mygo/internal/platforms"
	"strings"
)

const (
	settingsGeneral = iota
	settingsAccounts
	settingsAbout
	settingsPlayback
	settingsBarrage
	settingsChat
)

type settingsState struct {
	tab                         int
	imported, message, selected string
	manual                      bool
	versionTaps                 int
}

func (a *app) accountsWindow() { a.settingsWindow(settingsAccounts) }

func (a *app) settingsWindow(initialTab int) {
	a.settings.tab = initialTab
	a.settings.message = ""
	a.settings.versionTaps = 0
	if a.accountWin != nil && !a.accountWin.IsDestroyed() {
		a.accountWin.Focus()
		a.accountWin.Invalidate()
		return
	}
	a.settings.selected = "哔哩哔哩"
	a.settings.imported = ""
	a.accountWin = mygo.NewWindow(mygo.WindowOptions{Title: "设置 · Live MyGo", Width: 820, Height: 640, MinWidth: 760, MinHeight: 540, TitleBarStyle: mygo.TitleBarHidden, BackgroundColor: "light-dark(#ffffff,#171719)", Parent: a.win, Content: ui.View(a.settingsView)})
	a.accountWin.OnClose(func(*mygo.CloseEvent) { a.settings.imported = "" })
	a.accountWin.OnClosed(func() {
		if !a.closing && a.win != nil && !a.win.IsDestroyed() {
			a.win.Focus()
		}
	})
}

func (a *app) settingsView(c *ui.Context) {
	a.applyTheme(c)
	c.OnShortcut(ui.Cmd, ui.KeyW, func() {
		if a.accountWin != nil {
			a.accountWin.Close()
		}
	})
	c.OnShortcut(0, ui.KeyEscape, func() {
		if a.accountWin != nil {
			a.accountWin.Close()
		}
	})
	p := colors(c.Theme())
	tabs := []struct {
		id    int
		label string
		icon  *ui.SVG
	}{{settingsGeneral, "通用", iconSettings}, {settingsPlayback, "播放", iconPlay}, {settingsBarrage, "画面弹幕", iconDanmaku}, {settingsChat, "弹幕列表", iconChat}, {settingsAccounts, "账号", iconPerson}, {settingsAbout, "关于", iconTV}}
	title := "通用"
	for _, tab := range tabs {
		if tab.id == a.settings.tab {
			title = tab.label
		}
	}
	ui.Row(c).Fill().Background(p.bg).Children(func() {
		ui.Column(c).Width(174).Shrink(0).FillHeight().Padding(65, 12, 18, 12).Gap(4).Background(p.soft).Children(func() {
			ui.Text(c, "设置").FontSize(11).FontWeight(500).TextColor(p.muted).Padding(0, 10, 10, 10)
			for _, tab := range tabs {
				b := ui.ButtonBase(c.Key(tab.id)).FillWidth().Height(36).Radius(8).PaddingX(10).Gap(10).AlignItems(ui.Center).Justify(ui.Start).Label("设置分类：" + tab.label)
				buttonFeedback(b, p)
				ink := p.muted
				if a.settings.tab == tab.id {
					b.Background(p.selected)
					ink = p.accent
				}
				b.Children(func() {
					ui.Icon(c, tab.icon).Size(16, 16).TextColor(ink)
					ui.Text(c, tab.label).FontSize(12.5).FontWeight(500).TextColor(p.text)
				})
				if b.Clicked() {
					a.settings.tab = tab.id
					a.settings.versionTaps = 0
					a.settings.message = ""
				}
			}
		})
		ui.Box(c).Width(1).FillHeight().Background(p.line)
		ui.Column(c).Grow(1).MinWidth(0).FillHeight().Children(func() {
			for _, tab := range tabs {
				if tab.id == a.settings.tab {
					title = tab.label
				}
			}
			ui.Row(c).Height(76).Padding(20, titlebarRight(c, 26), 12, 26).AlignItems(ui.Center).DragWindow().Children(func() {
				ui.Text(c, title).FontSize(21).FontWeight(650).TextColor(p.text).Grow(1)
				if a.settings.tab == settingsBarrage && ui.Button(c, "推荐样式").Height(28).FontSize(11.5).Clicked() {
					a.restoreCaptionStyle()
				}
			})
			ui.Scroll(c.Key(a.settings.tab)).Grow(1).MinHeight(0).Padding(4, 26, 26, 26).Transition(menuContentTransition).Children(func() {
				if a.library == nil {
					emptyState(c, iconSettings, "偏好设置暂不可用", "无法读取本机配置文件。", nil)
					return
				}
				switch a.settings.tab {
				case settingsGeneral:
					a.generalSettings(c)
				case settingsPlayback:
					a.playbackSettings(c)
				case settingsBarrage:
					ui.Column(c).Gap(14).Children(func() {
						settingsGroup(c, "", func() {
							preferenceSwitch(c, true, "显示画面弹幕", a.library.Preferences.Danmaku, func(v bool) { next := a.library.Preferences; next.Danmaku = v; a.savePreferences(next) })
						})
						a.barrageSettings(c, true)
					})
				case settingsChat:
					a.chatSettingsContent(c, true)
				case settingsAccounts:
					a.accountSettings(c)
				case settingsAbout:
					a.aboutView(c)
				}
			})
		})
	})
}

func settingsGroup(c *ui.Context, title string, body func()) {
	p := colors(c.Theme())
	ui.Column(c).Gap(9).Children(func() {
		if title != "" {
			ui.Text(c, title).FontSize(11.5).FontWeight(600).TextColor(p.muted).PaddingX(3)
		}
		ui.Column(c).Radius(10).Background(p.soft).Clip().Children(body)
	})
}
func settingsDivider(c *ui.Context) {
	ui.Box(c).Height(1).MarginX(15).Background(colors(c.Theme()).line)
}

func preferenceSelect(c *ui.Context, full bool, label string, value *string, choices []string) bool {
	if !full {
		return compactSelect(c, label, value, choices)
	}
	changed := false
	settingsRow(c, label, func() {
		changed = ui.Select(c, value, choices).Width(170).Height(30).FontSize(12).Label(label).Changed()
	})
	return changed
}
func preferenceSlider(c *ui.Context, full bool, label string, value, lo, hi float64, display string, save func(float64)) {
	if !full {
		compactSlider(c, label, value, lo, hi, display, save)
		return
	}
	settingsRow(c, label, func() {
		if ui.Slider(c, &value, lo, hi).Width(120).Height(25).Label(label).Changed() {
			save(value)
		}
		ui.Text(c, display).Width(40).FontSize(11.5).TextColor(colors(c.Theme()).muted).TextAlign(ui.End)
	})
}
func preferenceSwitch(c *ui.Context, full bool, label string, value bool, save func(bool)) {
	if !full {
		compactSwitch(c, label, value, save)
		return
	}
	settingsRow(c, label, func() {
		if ui.Switch(c, &value).Label(label).Changed() {
			save(value)
		}
	})
}

func preferenceAction(c *ui.Context, full bool, label string) ui.Element {
	p := colors(c.Theme())
	height, inset, size := float32(35), float32(3), float32(12)
	if full {
		height, inset, size = 50, 15, 12.5
	}
	b := ui.ButtonBase(c).FillWidth().Height(height).PaddingX(inset).Gap(10).Radius(7).AlignItems(ui.Center).Justify(ui.Start).Label("打开" + label)
	buttonFeedback(b, p)
	b.Children(func() {
		ui.Text(c, label).FontSize(size).TextColor(p.text).Grow(1)
		ui.Icon(c, iconChevron).Size(13, 13).TextColor(p.faint)
	})
	return b
}

func settingsRow(c *ui.Context, label string, controls func()) {
	p := colors(c.Theme())
	ui.Row(c).Height(50).PaddingX(15).Gap(10).AlignItems(ui.Center).Children(func() { ui.Text(c, label).FontSize(12.5).TextColor(p.text).Grow(1); controls() })
}

func (a *app) generalSettings(c *ui.Context) {
	p := colors(c.Theme())
	ui.Column(c).Gap(24).Children(func() {
		settingsGroup(c, "启动", func() {
			settingsRow(c, "启动页", func() {
				label := startupLabels[startupPage(a.library.Preferences.StartupPage)]
				if ui.Select(c, &label, []string{"首页", "关注", "最近观看", "哔哩哔哩", "斗鱼", "虎牙", "抖音", "企鹅体育"}).Width(170).Height(30).FontSize(12).Label("启动页").Changed() {
					next := a.library.Preferences
					for key, value := range startupLabels {
						if value == label {
							next.StartupPage = key
						}
					}
					a.savePreferences(next)
				}
			})
		})
		settingsGroup(c, "外观", func() {
			settingsRow(c, "应用外观", func() {
				label := map[string]string{"system": "跟随系统", "light": "浅色", "dark": "深色"}[a.library.Preferences.Appearance]
				if label == "" {
					label = "跟随系统"
				}
				if ui.Select(c, &label, []string{"跟随系统", "浅色", "深色"}).Width(170).Height(30).FontSize(12).Label("外观").Changed() {
					next := a.library.Preferences
					next.Appearance = map[string]string{"跟随系统": "system", "浅色": "light", "深色": "dark"}[label]
					a.savePreferences(next)
				}
			})
		})
		ui.Column(c).Gap(13).PaddingX(3).Children(func() {
			ui.Text(c, "键盘快捷键").FontSize(11.5).FontWeight(600).TextColor(p.muted)
			for _, shortcut := range [][2]string{{"快捷搜索直播", shortcutLabel("⌘L / ⌘K")}, {"搜索当前页面 / 观看时快捷搜索", shortcutLabel("⌘F")}, {"暂停 / 继续", "Space"}, {"唤出播放器控制条", "K / Tab"}, {"静音 / 取消静音", "M"}, {"切换画面弹幕", "D"}, {"刷新 / 重新连接", shortcutLabel("⌘R")}, {"全屏", shortcutLabel("⌃⌘F")}, {"切换页面", shortcutLabel("⌘1 — ⌘4")}, {"返回 / 退出全屏", "Esc"}} {
				ui.Row(c).Gap(12).Children(func() {
					ui.Text(c, shortcut[0]).FontSize(12).TextColor(p.muted).Grow(1)
					ui.Text(c, shortcut[1]).FontSize(11.5).TextColor(p.muted)
				})
			}
		})
	})
}

func (a *app) playbackSettings(c *ui.Context) {
	p := colors(c.Theme())
	ui.Column(c).Gap(24).Children(func() {
		settingsGroup(c, "观看时", func() {
			settingsRow(c, "播放音量", func() {
				if ui.Slider(c, &a.volume, 0, 100).Width(120).Label("播放音量").Changed() {
					a.setVolume()
				}
				ui.Text(c, fmt.Sprintf("%.0f%%", a.volume)).Width(40).FontSize(11.5).TextAlign(ui.End).TextColor(p.muted)
			})
			settingsDivider(c)
			settingsRow(c, "离开直播间时暂停", func() {
				v := a.library.Preferences.PauseWhenBrowsing
				if ui.Switch(c, &v).Label("离开直播间时暂停").Changed() {
					next := a.library.Preferences
					next.PauseWhenBrowsing = v
					a.savePreferences(next)
				}
			})
			settingsDivider(c)
			settingsRow(c, "窗口跟随画面比例", func() {
				v := a.library.Preferences.FollowVideo
				if ui.Switch(c, &v).Label("窗口跟随画面比例").Changed() {
					a.toggleFollowVideo()
				}
			})
		})
		settingsGroup(c, "连接", func() {
			settingsRow(c, "斗鱼弹幕兼容连接", func() {
				v := a.library.Preferences.DouyuLegacyTLS
				if ui.Switch(c, &v).Label("斗鱼弹幕兼容连接").Changed() {
					next := a.library.Preferences
					next.DouyuLegacyTLS = v
					a.savePreferences(next)
				}
			})
			ui.Text(c, "斗鱼公开弹幕使用旧式 RSA AES-GCM，缺少前向保密。仅用于公开弹幕，不发送账号 Cookie，仍校验证书与主机名。").FontSize(10.5).TextColor(p.muted).MaxLines(4).Padding(0, 15, 14, 15)
		})
	})
}

func (a *app) accountSettings(c *ui.Context) {
	p := colors(c.Theme())
	ui.Column(c).Gap(13).Children(func() {
		for _, source := range []platforms.Platform{platforms.Bilibili, platforms.Douyu, platforms.Douyin, platforms.Qie} {
			ui.Row(c.Key(string(source))).Padding(16).Gap(13).Radius(11).Border(1, p.line).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(38, 38).Radius(10).Background(p.soft).Center().Children(func() { ui.Icon(c, platformLogo(source)).Size(24, 24) })
				status := a.accountStatus[source]
				saved := strings.HasPrefix(status, "已保存")
				if saved {
					status = "Cookie 已保存"
				}
				ui.Column(c).Grow(1).Gap(5).Children(func() {
					ui.Text(c, source.Name()).FontSize(13.5).FontWeight(600).TextColor(p.text)
					ui.Text(c, status).FontSize(11.5).TextColor(p.muted)
				})
				label := "登录"
				if saved {
					label = "重新登录"
				}
				if ui.Button(c, label).Height(30).FontSize(12).Clicked() {
					a.loginWindow(source)
				}
				if a.accountStatus[source] != "未登录" && iconButton(c.Key("remove"), iconClose, "移除本机登录").Size(28, 28).Clicked() {
					if err := a.vault.Remove(source); err != nil {
						a.settings.message = err.Error()
					} else {
						a.refreshAccountStatus()
						a.settings.message = "已移除本机登录"
					}
				}
			})
		}
		ui.Text(c, "Cookie 加密保存在此设备。").FontSize(11.5).TextColor(p.muted).Margin(3, 2, 8, 2)
		b := ui.ButtonBase(c.Key("import")).Padding(4, 0).Gap(6).Justify(ui.Start).Label("导入已有 Cookie")
		b.Children(func() {
			angle := float32(0)
			if a.settings.manual {
				angle = 90
			}
			ui.Icon(c, iconChevron).Size(11, 11).Rotate(angle).TextColor(p.muted)
			ui.Text(c, "导入已有 Cookie").FontSize(12).TextColor(p.muted)
		})
		if b.Clicked() {
			a.settings.manual = !a.settings.manual
			a.settings.message = ""
		}
		if a.settings.manual {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Select(c, &a.settings.selected, []string{"哔哩哔哩", "斗鱼", "抖音", "企鹅体育"}).Width(106).Height(30).FontSize(12).Label("Cookie 所属平台")
				ui.TextInput(c, &a.settings.imported).Password().Placeholder("粘贴 Cookie").Label("手动导入 Cookie").FontSize(12).Grow(1).MinWidth(0).Height(30)
				if ui.Button(c, "保存").Height(30).FontSize(12).Disabled(strings.TrimSpace(a.settings.imported) == "").Clicked() {
					source := platforms.Bilibili
					if a.settings.selected == "企鹅体育" {
						source = platforms.Qie
					}
					if a.settings.selected == "抖音" {
						source = platforms.Douyin
					}
					if a.settings.selected == "斗鱼" {
						source = platforms.Douyu
					}
					var cookies []accounts.Cookie
					for _, part := range strings.Split(a.settings.imported, ";") {
						name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
						if ok {
							cookies = append(cookies, accounts.Cookie{Name: name, Value: value, Domain: "." + source.Domain(), Path: "/", Secure: true})
						}
					}
					if err := a.vault.Save(source, cookies); err != nil {
						a.settings.message = err.Error()
					} else {
						a.settings.imported = ""
						a.settings.message = "已保存本机 Cookie"
						a.refreshAccountStatus()
						a.win.Invalidate()
					}
				}
			})
		}
		if a.settings.message != "" {
			ui.Text(c, a.settings.message).FontSize(11.5).MaxLines(3).TextColor(p.muted)
		}
	})
}

func (a *app) aboutView(c *ui.Context) {
	p := colors(c.Theme())
	ui.Column(c).AlignItems(ui.Center).Gap(9).Padding(10, 0).Children(func() {
		ui.Image(c, appIcon()).Size(82, 82)
		ui.Text(c, "Live MyGo").FontSize(20).FontWeight(650).TextColor(p.text)
		version := ui.ButtonBase(c).Height(28).PaddingX(10).Radius(7).Center().Label("版本号")
		buttonFeedback(version, p)
		version.Children(func() { ui.Text(c, strings.TrimSpace(appVersion)+" · 测试版").FontSize(12).TextColor(p.muted) })
		if version.Clicked() && a.unlockRaceMenu() {
			c.Toast("赛事入口已开启")
		}
		ui.Text(c, "原生播放器练手项目").FontSize(12).TextColor(p.muted).Margin(2, 0, 16, 0)
		ui.Column(c).WidthPercent(100).Gap(10).Children(func() {
			for _, reference := range [][2]string{{"MyGo", "https://github.com/egoist/mygo"}, {"Pure Live", "https://github.com/liuchuancong/pure_live"}, {"Simple Live", "https://github.com/xuanxuan123xuan/dart_simple_live"}, {"Lucide Icons", "https://lucide.dev/"}, {"mpv", "https://mpv.io/"}} {
				ui.Row(c).Gap(10).Children(func() {
					ui.Text(c, reference[0]).FontSize(12).TextColor(p.muted).Grow(1)
					if iconButton(c, iconExternal, "打开 "+reference[0]+" 项目").Size(22, 22).Clicked() {
						c.OpenURL(reference[1])
					}
				})
			}
		})
	})
}
