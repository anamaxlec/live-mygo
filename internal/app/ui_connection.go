//go:build darwin || windows

package app

import (
	"github.com/egoist/mygo/ui"
	"live-mygo/internal/platforms"
	"strings"
)

type connectionStep struct{ title, detail, state string }

func (a *app) connectionOverlay(c *ui.Context, opacity float32) {
	if opacity <= .005 {
		return
	}
	p := colors(c.Theme())
	// 叠层的空白区域穿透点击，只有卡片本身接收操作。
	ui.Box(c.Key("connection-layer")).Absolute().Left(10).Right(10).Top(10).Bottom(10).Justify(ui.End).PassThrough().Opacity(opacity).Children(func() {
		ui.Scroll(c).MaxHeightPercent(100).Radius(10).Background(p.bg).Border(1, p.line).Shadow(0, 4, 18, 0, ui.RGBA(0, 0, 0, .12)).Children(func() { a.connectionStatus(c) })
	})
}

func (a *app) connectionSteps() []connectionStep {
	room := connectionStep{"解析房间", "正在获取房间信息", "active"}
	video := connectionStep{"连接视频", "等待房间解析", "waiting"}
	chat := connectionStep{"连接弹幕", "等待视频连接", "waiting"}
	if a.targetRoom.Platform == platforms.FLive {
		room.title = "解析频道"
		video.title = "连接信号"
	}
	resolved := a.stream != nil && roomKey(a.stream.Room) == roomKey(a.targetRoom)
	if resolved {
		room.detail, room.state = "房间已确认", "done"
		video.detail, video.state = "正在等待视频画面", "active"
	}
	if a.shown && (a.decodedMS > 0 || a.playStarted.IsZero()) {
		video.detail, video.state = "播放中", "done"
		if a.stats.Paused {
			video.detail = "已暂停"
		}
	}
	if !a.busy && !a.shown {
		if resolved {
			video.detail, video.state = a.status, "failed"
		} else {
			room.detail, room.state = a.status, "failed"
		}
	}
	if a.targetRoom.Platform == platforms.FLive {
		return []connectionStep{room, video}
	}
	if a.chatRoomKey == roomKey(a.targetRoom) {
		chat.detail = a.chatStatus
		if a.chatStatus == "已连接" {
			chat.state = "done"
		} else if a.chatStatus == "正在连接" || strings.Contains(a.chatStatus, "秒后重连") {
			chat.state = "active"
		} else {
			chat.state = "failed"
		}
	}
	return []connectionStep{room, video, chat}
}

func (a *app) waitingForVideo() bool {
	return !a.shown || !a.playStarted.IsZero() && a.decodedMS == 0
}

func (a *app) connectionPending() bool {
	return a.waitingForVideo() || a.targetRoom.Platform != platforms.FLive && a.chatRoomKey == roomKey(a.targetRoom) && a.chatStatus != "已连接"
}

func connectionRows(c *ui.Context, steps []connectionStep) {
	p := colors(c.Theme())
	ui.Row(c).FillWidth().Padding(9, 2).Radius(8).Background(p.soft).Children(func() {
		for i, step := range steps {
			ui.Column(c.Key(step.title)).Grow(1).Basis(0).MinWidth(0).Gap(5).AlignItems(ui.Center).Label(step.title + "：" + step.detail).Children(func() {
				ink := p.faint
				if step.state == "active" || step.state == "done" {
					ink = p.accent
				} else if step.state == "failed" {
					ink = p.danger
				}
				ui.Box(c).Size(18, 18).Radius(9).Background(ink.Alpha(.09)).Center().Children(func() {
					switch step.state {
					case "active":
						ui.Spinner(c).Size(11, 11)
					case "done":
						ui.Icon(c, iconCheck).Size(11, 11).TextColor(ink)
					case "failed":
						ui.Icon(c, iconClose).Size(10, 10).TextColor(ink)
					default:
						ui.Text(c, string(rune('1'+i))).FontSize(9).FontWeight(600).TextColor(ink)
					}
				})
				ui.Text(c, step.title).FontSize(10).FontWeight(500).TextColor(p.muted).SingleLine()
			})
		}
	})
}

func connectionSummary(steps []connectionStep) (title, detail string, failed bool) {
	for i, step := range steps {
		if step.state == "failed" {
			if strings.Contains(step.detail, "未开播") {
				return "直播间未开播", "", true
			}
			title = "暂时无法连接"
			if i == 2 {
				title = "弹幕暂未连接"
			}
			return title, step.detail, true
		}
	}
	for _, step := range steps {
		if step.state == "active" {
			return "正在" + step.title, "", false
		}
	}
	return "连接完成", "", false
}

// 连接进度只在弹幕栏内展示，不占用或遮挡视频区域。
func (a *app) connectionStatus(c *ui.Context) {
	p := colors(c.Theme())
	steps := a.connectionSteps()
	title, detail, failed := connectionSummary(steps)
	ui.Column(c.Key("room-connection")).FillWidth().Shrink(0).Padding(14).Gap(12).Label("直播间连接状态").Children(func() {
		ui.Row(c).Gap(9).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(28, 28).Radius(8).Background(p.soft).Center().Children(func() {
				if title == "连接完成" {
					ui.Icon(c, iconCheck).Size(15, 15).TextColor(p.accent)
				} else if failed {
					ui.Icon(c, iconTV).Size(16, 16).TextColor(p.muted)
				} else {
					ui.Spinner(c).Size(13, 13)
				}
			})
			ui.Text(c, title).FontSize(13).FontWeight(600).TextColor(p.text).Grow(1).MinWidth(0).MaxLines(2)
		})
		if detail != "" {
			ui.Text(c, detail).FontSize(11).TextColor(p.muted).MaxLines(3)
		}
		connectionRows(c, steps)
		if !a.busy && !a.shown {
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				if a.targetRoom.ID != "" && ui.PrimaryButton(c, "重新连接").Label("重新连接").Grow(1).Basis(0).MinWidth(0).Padding(0).Height(30).Radius(7).FontSize(11.5).Clicked() {
					a.reloadStream()
				}
				back := ui.Button(c, "返回列表").Label("返回直播列表").Grow(1).Basis(0).MinWidth(0).Padding(0).Height(30).Radius(7).Border(0, ui.Transparent).FontSize(11.5).TextColor(p.muted)
				if back.Clicked() {
					a.backToBrowse()
				}
			})
		} else if a.shown && a.chatRoomKey == roomKey(a.targetRoom) && len(steps) > 2 && steps[2].state == "failed" {
			if ui.PrimaryButton(c, "重新连接弹幕").FillWidth().Height(30).FontSize(11.5).Clicked() {
				a.startChat(true)
			}
		}
		if a.targetRoom.Platform == "douyu" && a.library != nil && !a.library.Preferences.DouyuLegacyTLS && strings.Contains(a.chatStatus, "加密连接失败") {
			if ui.Button(c, "斗鱼连接兼容设置").FontSize(11).Height(28).Margin(6, 0, 0, 0).Clicked() {
				a.settingsWindow(settingsPlayback)
			}
		}
	})
}
