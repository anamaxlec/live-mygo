//go:build darwin || windows

package app

import (
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"
)

func compactSelect(c *ui.Context, label string, value *string, choices []string) bool {
	p := colors(c.Theme())
	changed := false
	ui.Row(c).Height(37).PaddingX(3).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(p.text).Grow(1)
		changed = ui.Select(c, value, choices).Width(140).Height(27).FontSize(11.5).Label(label).Changed()
	})
	return changed
}

func (a *app) barrageSettings(c *ui.Context, full ...bool) {
	expanded := len(full) > 0 && full[0]
	p := colors(c.Theme())
	if !expanded {
		ui.Row(c).Padding(4, 3, 10, 3).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "弹幕与画面").FontSize(13).FontWeight(600).TextColor(p.text).Grow(1)
			a.recommendedStyleButton(c)
		})
	}

	if a.library == nil {
		return
	}
	prefs := a.library.Preferences
	ui.Row(c).Gap(5).Padding(0, 0, 8, 0).AlignItems(ui.Center).Children(func() {
		for i, label := range []string{"样式", "布局", "屏蔽"} {
			if filterButton(c.Key(i), label, a.danmakuTab == i).Clicked() {
				a.danmakuTab = i
			}
		}
		if expanded {
			ui.Box(c).Grow(1)
			a.recommendedStyleButton(c)
		}
	})
	ui.Column(c.Key("barrage-tab-content")).FillWidth().Clip().Children(func() {
		ui.Column(c.Key(a.danmakuTab)).FillWidth().Transition(menuContentTransition).Children(func() {
			if a.danmakuTab == 0 {
				ui.Box(c.Key("caption-style-preview")).Height(62).FillWidth().Radius(9).Background(ui.Hex("#24262c")).Margin(0, 0, 9, 0).Clip().Label("弹幕效果预览").Draw(func(p *ui.Painter, r ui.Rect) {
					w, h := p.MeasureText(0, captionSpan("弹幕预览 Aa", prefs, ui.Hex("#ffffff")))
					paintCaption(p, r.X+max(8, (r.W-w)/2), r.Y+(r.H-h)/2, "弹幕预览 Aa", captionColor(0xffffff, prefs.DanmakuColor), prefs)
				})
			}
			_, height := c.Size()
			content := func() {
				ui.Column(c).Gap(2).Children(func() {
					switch a.danmakuTab {
					case 0:
						font := "系统默认"
						labels, families := captionFontOptions()
						for i, family := range families {
							if family == prefs.DanmakuFont {
								font = labels[i]
							}
						}
						if preferenceSelect(c, expanded, "字体", &font, labels) {
							for i, label := range labels {
								if font == label {
									next := a.library.Preferences
									next.DanmakuFont = families[i]
									a.savePreferences(next)
								}
							}
						}
						weight := map[int]string{400: "常规", 500: "中等", 600: "加粗", 700: "粗体"}[prefs.DanmakuWeight]
						if weight == "" {
							weight = "加粗"
						}
						if preferenceSelect(c, expanded, "字重", &weight, []string{"常规", "中等", "加粗", "粗体"}) {
							next := a.library.Preferences
							next.DanmakuWeight = map[string]int{"常规": 400, "中等": 500, "加粗": 600, "粗体": 700}[weight]
							a.savePreferences(next)
						}
						preferenceSlider(c, expanded, "字号", prefs.DanmakuSize, 16, 48, fmt.Sprintf("%.0f", prefs.DanmakuSize), func(v float64) { next := a.library.Preferences; next.DanmakuSize = v; a.previewPreferences(next) })
						color := prefs.DanmakuColor
						if color == "" {
							color = "平台颜色"
						}
						if preferenceSelect(c, expanded, "文字颜色", &color, []string{"平台颜色", "白色", "暖黄", "浅蓝", "粉色"}) {
							next := a.library.Preferences
							next.DanmakuColor = color
							a.savePreferences(next)
						}
						preferenceSlider(c, expanded, "描边", prefs.DanmakuStroke, 0, 3, fmt.Sprintf("%.1f px", prefs.DanmakuStroke), func(v float64) { next := a.library.Preferences; next.DanmakuStroke = v; a.previewPreferences(next) })
						preferenceSlider(c, expanded, "不透明度", prefs.DanmakuOpacity, 20, 100, fmt.Sprintf("%.0f%%", prefs.DanmakuOpacity), func(v float64) { next := a.library.Preferences; next.DanmakuOpacity = v; a.previewPreferences(next) })
					case 1:
						fps := "流畅 · 60 fps"
						if prefs.DanmakuFPS <= 30 {
							fps = "省电 · 30 fps"
						} else if prefs.DanmakuFPS >= 120 {
							fps = "跟随屏幕"
						}
						if preferenceSelect(c, expanded, "流畅度", &fps, []string{"省电 · 30 fps", "流畅 · 60 fps", "跟随屏幕"}) {
							next := a.library.Preferences
							next.DanmakuFPS = map[string]int{"省电 · 30 fps": 30, "流畅 · 60 fps": 60, "跟随屏幕": 120}[fps]
							a.savePreferences(next)
						}
						preferenceSlider(c, expanded, "滚动速度", prefs.DanmakuSpeed, .5, 2, fmt.Sprintf("%.1f×", prefs.DanmakuSpeed), func(v float64) { next := a.library.Preferences; next.DanmakuSpeed = v; a.previewPreferences(next) })
						preferenceSlider(c, expanded, "显示区域", prefs.DanmakuArea, .25, 1, fmt.Sprintf("%.0f%%", prefs.DanmakuArea*100), func(v float64) { next := a.library.Preferences; next.DanmakuArea = v; a.previewPreferences(next) })
						preferenceSlider(c, expanded, "轨道行数", float64(prefs.DanmakuLines), 1, 16, fmt.Sprint(prefs.DanmakuLines), func(v float64) {
							next := a.library.Preferences
							next.DanmakuLines = int(math.Round(v))
							a.previewPreferences(next)
						})
						preferenceSlider(c, expanded, "行距", prefs.DanmakuGap, 0, 24, fmt.Sprintf("%.0f px", prefs.DanmakuGap), func(v float64) { next := a.library.Preferences; next.DanmakuGap = v; a.previewPreferences(next) })
						preferenceSlider(c, expanded, "上边距", prefs.DanmakuTop, 0, 100, fmt.Sprintf("%.0f px", prefs.DanmakuTop), func(v float64) { next := a.library.Preferences; next.DanmakuTop = v; a.previewPreferences(next) })
						preferenceSlider(c, expanded, "下边距", prefs.DanmakuBottom, 0, 100, fmt.Sprintf("%.0f px", prefs.DanmakuBottom), func(v float64) { next := a.library.Preferences; next.DanmakuBottom = v; a.previewPreferences(next) })
						ui.Box(c).Height(1).MarginY(9).Background(p.line)
						preferenceSwitch(c, expanded, "窗口跟随画面比例", prefs.FollowVideo, func(bool) { a.toggleFollowVideo() })
						mode := a.fitMode
						if mode == "" {
							mode = "适应"
						}
						if preferenceSelect(c, expanded, "画面比例", &mode, []string{"适应", "填满", "拉伸"}) {
							a.setFit(mode)
						}
					case 2:
						preferenceSwitch(c, expanded, "过滤画面重复弹幕", prefs.DanmakuMerge, func(v bool) { next := a.library.Preferences; next.DanmakuMerge = v; a.savePreferences(next) })
						preferenceSwitch(c, expanded, "启用屏蔽规则", prefs.FilterEnabled, func(v bool) {
							next := a.library.Preferences
							next.FilterEnabled = v
							a.savePreferences(next)
							a.clearFlyingChat()
						})
						preferenceSwitch(c, expanded, "过滤重复消息", prefs.FilterDuplicates, func(v bool) { next := a.library.Preferences; next.FilterDuplicates = v; a.savePreferences(next) })
						if preferenceAction(c, expanded, "关键词与用户屏蔽").Clicked() {
							a.danmakuOpen = false
							a.filterWindow()
						}
					}
				})
			}
			if expanded {
				ui.Column(c).Radius(10).Background(p.soft).Clip().Children(content)
			} else {
				ui.Scroll(c.Key(a.danmakuTab)).MaxHeight(max(100, min(390, height-240))).Children(content)
				if ui.Button(c, "全部画面弹幕设置").FillWidth().Height(29).FontSize(11.5).Margin(8, 0, 0, 0).Clicked() {
					a.danmakuOpen = false
					a.settingsWindow(settingsBarrage)
				}
			}
		})
	})
}

// recommendedStyleButton 是次要操作，跟随内容列右缘而不是占用页面标题栏。
func (a *app) recommendedStyleButton(c *ui.Context) {
	p := colors(c.Theme())
	b := ui.ButtonBase(c.Key("recommended-caption-style")).Height(28).PaddingX(9).Radius(7).Gap(6).AlignItems(ui.Center).Label("推荐样式")
	hovered := b.Hovered() || b.FocusVisible()
	if hovered {
		b.Background(p.hover)
	}
	ink := p.muted
	if hovered {
		ink = p.text
	}
	b.Children(func() {
		ui.Icon(c, iconRefresh).Size(13, 13).TextColor(ink)
		ui.Text(c, "推荐样式").FontSize(11.5).FontWeight(500).TextColor(ink)
	})
	if b.Clicked() {
		a.restoreCaptionStyle()
	}
}

func (a *app) restoreCaptionStyle() {
	if a.library == nil {
		return
	}
	next := a.library.Preferences
	next.DanmakuFont = ""
	next.DanmakuSize = 24
	next.DanmakuWeight = 500
	next.DanmakuOpacity = 80
	next.DanmakuStroke = 1
	next.DanmakuLines = 4
	next.DanmakuArea = .35
	next.DanmakuGap = 10
	next.DanmakuTop = 12
	next.DanmakuBottom = 24
	next.DanmakuSpeed = 1
	next.DanmakuColor = "平台颜色"
	next.DanmakuMerge = true
	a.savePreferences(next)
}
