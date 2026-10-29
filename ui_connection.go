//go:build darwin

package main

import (
	"github.com/egoist/mygo/ui"
	"strings"
)

type connectionStep struct{ title, detail, state string }

func (a *app) connectionSteps() []connectionStep {
	room := connectionStep{"解析房间", "正在获取房间信息", "active"}
	video := connectionStep{"连接视频", "等待房间解析", "waiting"}
	chat := connectionStep{"连接弹幕", "等待视频连接", "waiting"}
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
	return a.waitingForVideo() || a.chatRoomKey == roomKey(a.targetRoom) && a.chatStatus != "已连接"
}

func connectionRows(c *ui.Context, steps []connectionStep) {
	p := colors(c.Theme())
	for _, step := range steps {
		ui.Row(c.Key(step.title)).Gap(9).PaddingY(4).AlignItems(ui.Center).Label(step.title + "：" + step.detail).Children(func() {
			ui.Box(c).Size(18, 18).Center().Shrink(0).Children(func() {
				switch step.state {
				case "active":
					ui.Spinner(c).Size(12, 12)
				case "done":
					ui.Icon(c, iconCheck).Size(14, 14).TextColor(p.accent)
				case "failed":
					ui.Icon(c, iconClose).Size(13, 13).TextColor(p.danger)
				default:
					ui.Box(c).Size(5, 5).Radius(3).Background(p.faint.Alpha(.5))
				}
			})
			ui.Text(c, step.title).FontSize(11.5).FontWeight(500).TextColor(p.text).Width(66)
			ink := p.muted
			if step.state == "failed" {
				ink = p.danger
			}
			ui.Text(c, step.detail).FontSize(10.5).TextColor(ink).MaxLines(3).Grow(1).MinWidth(0)
		})
	}
}

// 连接进度只在弹幕栏内展示，不占用或遮挡视频区域。
func (a *app) connectionStatus(c *ui.Context) {
	ui.Column(c.Key("room-connection")).FillWidth().Shrink(0).Padding(6, 11, 9, 11).Label("直播间连接状态").Children(func() {
		connectionRows(c, a.connectionSteps())
		if !a.busy && !a.shown {
			ui.Row(c).Gap(8).Padding(6, 0, 0, 0).Children(func() {
				if a.targetRoom.ID != "" && ui.Button(c, "重新连接").Height(29).FontSize(11.5).Clicked() {
					a.reloadStream()
				}
				if ui.Button(c, "返回直播列表").Height(29).FontSize(11.5).Clicked() {
					a.backToBrowse()
				}
			})
		} else if a.shown && a.chatRoomKey == roomKey(a.targetRoom) && a.connectionSteps()[2].state == "failed" {
			if ui.Button(c, "重新连接弹幕").Height(29).FontSize(11.5).Margin(6, 0, 0, 0).Clicked() {
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
