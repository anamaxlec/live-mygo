//go:build darwin

package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

const sidebarWidth float32 = 232
const contentInset float32 = 26

var menuContentTransition = ui.ElementTransition{
	Duration: 140 * time.Millisecond, Ease: ui.EaseOut, Colors: true,
	Enter: &ui.Motion{Y: 4}, Exit: &ui.Motion{Y: -3},
}

type palette struct{ bg, sidebar, text, muted, faint, line, hover, selected, soft, accent, danger ui.Color }

func floatingPanel(panel ui.Element, p palette) ui.Element {
	return panel.Padding(14).Radius(12).Background(p.bg).Border(1, p.line).Shadow(0, 6, 24, 0, ui.RGBA(0, 0, 0, .18))
}

// 使用框架的退出快照；淡出面板不再接收点击，快速重开沿当前姿态继续。
func animatedPopover(c *ui.Context, anchor ui.Element, open *bool, above bool, content func(ui.Element)) {
	ui.Overlay(c, func() {})
	ui.PopoverBase(c, anchor, open, func(panel ui.Element) {
		content(panel)
		origin, offset := float32(0), float32(-5)
		if above {
			origin, offset = 1, 5
		}
		panel.TransformOrigin(1, origin).Transition(ui.ElementTransition{
			Size: true, Duration: 160 * time.Millisecond, Ease: ui.EaseOut,
			Enter: &ui.Motion{Y: offset, Scale: .985}, Exit: &ui.Motion{Y: offset, Scale: .985},
		})
	})
}

func buttonFeedback(b ui.Element, p palette) {
	bg := ui.Transparent
	if b.Hovered() || b.FocusVisible() {
		bg = p.hover
	}
	if b.Pressed() {
		bg = p.selected
	}
	b.Background(b.AnimateColor("feedback", bg, ui.Animation{Duration: 90 * time.Millisecond})).FocusRing(true)
}

func colors(t *ui.Theme) palette {
	p := palette{bg: ui.Hex("#ffffff"), sidebar: ui.Hex("#f4f4f6"), text: ui.Hex("#242426"), muted: ui.Hex("#737379"), faint: ui.Hex("#8d8d95"), line: ui.RGBA(0, 0, 0, 0.065), hover: ui.RGBA(0, 0, 0, 0.035), selected: ui.RGBA(0, 0, 0, 0.07), soft: ui.Hex("#f3f3f5"), accent: t.Accent, danger: ui.Hex("#d64949")}
	if t.Dark {
		p.bg = ui.Hex("#171719")
		p.sidebar = ui.Hex("#202023")
		p.text = ui.Hex("#ededf0")
		p.muted = ui.Hex("#9a9aa2")
		p.faint = ui.Hex("#696972")
		p.line = ui.RGBA(255, 255, 255, 0.075)
		p.hover = ui.RGBA(255, 255, 255, 0.05)
		p.selected = ui.RGBA(255, 255, 255, 0.09)
		p.soft = ui.Hex("#252528")
		p.danger = ui.Hex("#f17676")
	}
	return p
}

func iconButton(c *ui.Context, icon *ui.SVG, label string, active ...bool) ui.Element {
	p := colors(c.Theme())
	b := ui.ButtonBase(c).Size(30, 30).Radius(7).Center().Label(label).Tooltip(label)
	bg := ui.Transparent
	if b.Hovered() || b.FocusVisible() {
		bg = p.hover
	}
	if b.Pressed() {
		bg = p.selected
	}
	b.FocusRing(true)
	b.Background(b.AnimateColor("hover", bg, ui.Animation{Duration: 110 * time.Millisecond}))
	ink := p.muted
	if len(active) > 0 && active[0] {
		ink = p.accent
		b.Background(p.accent.Alpha(.08))
	}
	if icon == iconHeartFilled {
		ink = p.accent
	}
	b.Children(func() { ui.Icon(c, icon).Size(17, 17).TextColor(ink) })
	return b
}

func emptyState(c *ui.Context, icon *ui.SVG, title, detail string, action func()) {
	emptyStateAction(c, icon, title, detail, "重新加载", action)
}
func emptyStateAction(c *ui.Context, icon *ui.SVG, title, detail, actionLabel string, action func()) {
	p := colors(c.Theme())
	ui.Column(c).Grow(1).Center().Padding(24).Gap(10).Children(func() {
		ui.Box(c).Size(58, 58).Radius(18).Background(p.soft).Center().Margin(0, 0, 7, 0).Children(func() { ui.Icon(c, icon).Size(27, 27).TextColor(p.faint) })
		ui.Text(c, title).FontSize(15).FontWeight(600).TextColor(p.text)
		if detail != "" {
			ui.Text(c, detail).MaxWidth(420).MaxLines(3).FontSize(12.5).TextColor(p.muted)
		}
		if action != nil && ui.Button(c, actionLabel).Height(31).FontSize(12).Margin(4, 0, 0, 0).Clicked() {
			action()
		}
	})
}
