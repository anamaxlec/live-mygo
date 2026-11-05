//go:build darwin || windows

package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"live-mygo/internal/library"
	"live-mygo/internal/platforms"
)

func notificationHoursAllow(now time.Time, prefs library.Preferences) bool {
	start, end := prefs.NotifyStartHour, prefs.NotifyEndHour
	if start == end {
		return true
	}
	hour := now.Hour()
	if start < end {
		return hour >= start && hour < end
	}
	return hour >= start || hour < end
}

func (a *app) armNotifications() {
	if a.parent != nil {
		a.rootApp().armNotifications()
		return
	}
	if a.notificationTimer != nil {
		a.notificationTimer.Stop()
		a.notificationTimer = nil
	}
	if a.closing || a.win == nil || a.library == nil || !a.library.Preferences.NotifyEnabled {
		return
	}
	any := false
	for _, room := range a.library.Favorites {
		if a.library.Details(room).Notify {
			any = true
			break
		}
	}
	if !any {
		return
	}
	a.notificationTimer = time.AfterFunc(2*time.Minute, func() {
		a.win.Update(func() {
			if a.closing {
				return
			}
			a.notificationTimer = nil
			a.refreshFavoriteRooms(true, true)
			a.armNotifications()
		})
	})
}

func (a *app) notifyLiveRooms(fresh map[string]platforms.Room, now time.Time) {
	if a.parent != nil {
		a.rootApp().notifyLiveRooms(fresh, now)
		return
	}
	if a.library == nil || !a.library.Preferences.NotifyEnabled {
		return
	}
	started := a.notificationCandidates(fresh, now)
	if len(started) == 0 {
		return
	}
	var names []string
	for _, room := range started[:min(2, len(started))] {
		names = append(names, a.displayRoomName(room))
	}
	body := strings.Join(names, "、") + " 开始直播了"
	if len(started) > 2 {
		body = fmt.Sprintf("%s 等 %d 个关注开始直播了", strings.Join(names, "、"), len(started))
	}
	n := mygo.NewNotification(mygo.NotificationOptions{Title: "关注开播", Body: body})
	room := started[0]
	n.OnClick(func() {
		a.win.Update(func() {
			if !a.closing {
				a.win.Focus()
				a.playRoom(room)
			}
		})
	})
	go func() {
		if err := n.Show(); err != nil {
			a.win.Update(func() {
				if !a.closing {
					a.toast = "开播通知暂不可用，请检查系统通知设置"
				}
			})
		}
	}()
}

func (a *app) notificationCandidates(fresh map[string]platforms.Room, now time.Time) []platforms.Room {
	if a.notificationKnown == nil {
		a.notificationKnown = map[string]bool{}
	}
	var started []platforms.Room
	for _, room := range a.library.Favorites {
		key := roomKey(room)
		if !a.library.Details(room).Notify {
			delete(a.notificationKnown, key)
			continue
		}
		current, ok := fresh[key]
		if !ok {
			continue
		}
		previous, known := a.notificationKnown[key]
		a.notificationKnown[key] = current.Live
		if known && !previous && current.Live {
			started = append(started, current)
		}
	}
	prefs := a.library.Preferences
	interval := prefs.NotifyInterval
	if interval < 5 {
		interval = 10
	}
	if len(started) == 0 || prefs.NotifyQuiet || !notificationHoursAllow(now, prefs) || now.Sub(a.notificationLast) < time.Duration(interval)*time.Minute {
		return nil
	}
	a.notificationLast = now
	return started
}

func (a *app) saveFavoriteDetails(room platforms.Room, details library.FavoriteDetails) error {
	previous := a.library.Details(room)
	if err := a.library.SetFavoriteDetails(room, details); err != nil {
		return err
	}
	if previous.Notify != details.Notify {
		delete(a.rootApp().notificationKnown, roomKey(room))
		a.armNotifications()
		a.refreshFavoriteRooms(true, true)
	}
	return nil
}

func (a *app) notificationSettings(c *ui.Context) {
	prefs := a.library.Preferences
	settingsGroup(c, "开播提醒", func() {
		preferenceSwitch(c, true, "开播提醒", prefs.NotifyEnabled, func(value bool) { next := a.library.Preferences; next.NotifyEnabled = value; a.savePreferences(next) })
		if !prefs.NotifyEnabled {
			return
		}
		preferenceSwitch(c, true, "免打扰", prefs.NotifyQuiet, func(value bool) { next := a.library.Preferences; next.NotifyQuiet = value; a.savePreferences(next) })
		settingsRow(c, "允许通知时段", func() {
			hours := make([]string, 24)
			for i := range hours {
				hours[i] = fmt.Sprintf("%02d:00", i)
			}
			start, end := hours[max(0, min(23, prefs.NotifyStartHour))], hours[max(0, min(23, prefs.NotifyEndHour))]
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				changed := ui.Select(c, &start, hours).Width(85).Height(28).Label("通知开始时间").Changed()
				ui.Text(c, "至").FontSize(11)
				changed = ui.Select(c, &end, hours).Width(85).Height(28).Label("通知结束时间").Changed() || changed
				if changed {
					next := a.library.Preferences
					next.NotifyStartHour, _ = strconv.Atoi(start[:2])
					next.NotifyEndHour, _ = strconv.Atoi(end[:2])
					a.savePreferences(next)
				}
			})
		})
		settingsRow(c, "通知间隔", func() {
			interval := prefs.NotifyInterval
			if interval < 5 {
				interval = 10
			}
			label := fmt.Sprintf("%d 分钟", interval)
			if ui.Select(c, &label, []string{"5 分钟", "10 分钟", "15 分钟", "30 分钟", "60 分钟"}).Width(116).Height(28).Label("开播通知最短间隔").Changed() {
				next := a.library.Preferences
				next.NotifyInterval, _ = strconv.Atoi(strings.TrimSuffix(label, " 分钟"))
				a.savePreferences(next)
			}
		})
		ui.Text(c, "在关注的编辑菜单里订阅。开始与结束时间相同表示全天。").FontSize(11).TextColor(colors(c.Theme()).muted).MaxLines(2).Padding(0, 15, 12, 15)
	})
}
