//go:build darwin || windows

package app

import (
	"live-mygo/internal/library"
	"live-mygo/internal/platforms"
	"testing"
	"time"
)

func TestSubscriptionRequiresObservedTransitionAndRespectsQuietAndRate(t *testing.T) {
	a := polishApp(t)
	room := platforms.Room{Platform: platforms.Douyu, ID: "6657", Live: true}
	a.library.Favorites = []platforms.Room{room}
	a.library.FavoriteDetails = map[string]library.FavoriteDetails{roomKey(room): {Notify: true}}
	a.library.Preferences.NotifyEnabled = true
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.Local)
	fresh := map[string]platforms.Room{roomKey(room): room}
	if len(a.notificationCandidates(fresh, now)) != 0 {
		t.Fatal("首次观察不应弹出开播通知")
	}
	room.Live = false
	fresh[roomKey(room)] = room
	a.notificationCandidates(fresh, now)
	room.Live = true
	fresh[roomKey(room)] = room
	if len(a.notificationCandidates(fresh, now)) != 1 {
		t.Fatal("漏掉开播变化")
	}
	room.Live = false
	fresh[roomKey(room)] = room
	a.notificationCandidates(fresh, now)
	room.Live = true
	fresh[roomKey(room)] = room
	if len(a.notificationCandidates(fresh, now.Add(time.Minute))) != 0 {
		t.Fatal("未限制通知频率")
	}
	a.library.Preferences.NotifyQuiet = true
	room.Live = false
	fresh[roomKey(room)] = room
	a.notificationCandidates(fresh, now)
	room.Live = true
	fresh[roomKey(room)] = room
	if len(a.notificationCandidates(fresh, now.Add(time.Hour))) != 0 {
		t.Fatal("免打扰仍发送通知")
	}
}

func TestNotificationHoursHandleAllDayAndOvernight(t *testing.T) {
	day := time.Date(2026, 11, 4, 12, 0, 0, 0, time.Local)
	for _, test := range []struct {
		start, end, hour int
		want             bool
	}{{0, 0, 3, true}, {9, 23, 12, true}, {9, 23, 23, false}, {22, 7, 23, true}, {22, 7, 6, true}, {22, 7, 12, false}} {
		if got := notificationHoursAllow(day.Add(time.Duration(test.hour-12)*time.Hour), library.Preferences{NotifyStartHour: test.start, NotifyEndHour: test.end}); got != test.want {
			t.Fatal(test, got)
		}
	}
}
