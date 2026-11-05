//go:build darwin || windows

package app

import (
	"github.com/egoist/mygo/ui"
	"live-mygo/internal/library"
	"live-mygo/internal/platforms"
)

type favoriteEditorState struct {
	open, focus bool
	room        platforms.Room
	details     library.FavoriteDetails
	err         string
	handle      ui.Handle
}

func (a *app) editFavorite(room platforms.Room) {
	if a.library == nil || !a.library.Has(room) {
		return
	}
	a.favoriteEditor = favoriteEditorState{open: true, focus: true, room: room, details: a.library.Details(room)}
}

func (a *app) displayRoomName(room platforms.Room) string {
	if a.library != nil {
		if alias := a.library.Details(room).Alias; alias != "" {
			return alias
		}
	}
	return roomLabel(room)
}

func (a *app) favoriteEditorView(c *ui.Context) {
	ui.Overlay(c, func() {})
	state := &a.favoriteEditor
	p := colors(c.Theme())
	ui.DialogBase(c.Key("favorite-editor"), &state.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .26)).Padding(24)
		floatingPanel(panel, p).Width(420).MaxWidthPercent(100).Gap(14).Transition(menuContentTransition)
		ui.Text(c, "编辑关注").FontSize(16).FontWeight(600).TextColor(p.text)
		ui.Text(c, roomLabel(state.room)).FontSize(12).TextColor(p.muted).SingleLine()
		settingsRow(c, "别名", func() {
			ui.TextInput(c, &state.details.Alias).Bind(&state.handle).Width(240).Height(30).Placeholder("使用主播名").Label("关注别名")
			if state.focus {
				state.handle.Focus(c)
				state.focus = false
			}
		})
		settingsRow(c, "分组", func() {
			ui.TextInput(c, &state.details.Group).Width(240).Height(30).Placeholder("未分组").Label("关注分组名称")
		})
		settingsRow(c, "置顶", func() { ui.Switch(c, &state.details.Pinned).Label("置顶这个关注") })
		settingsRow(c, "开播通知", func() { ui.Switch(c, &state.details.Notify).Label("订阅这个关注的开播通知") })
		if state.err != "" {
			ui.Text(c, state.err).FontSize(11.5).TextColor(p.danger).MaxLines(2)
		}
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			if ui.Button(c, "取消").Grow(1).Basis(0).Height(30).Clicked() {
				state.open = false
			}
			if ui.PrimaryButton(c, "保存").Grow(1).Basis(0).Height(30).Clicked() {
				if err := a.saveFavoriteDetails(state.room, state.details); err != nil {
					state.err = err.Error()
				} else {
					state.open = false
					a.toast = "已保存关注设置"
				}
			}
		})
	})
}
