//go:build darwin || windows

package app

import (
	"maps"
	"time"

	"github.com/egoist/mygo/ui"
	"live-mygo/internal/platforms"
)

func (a *app) homeRecommendationFilterEnabled() bool {
	return a.library != nil && !a.library.Preferences.DisableHomeRecommendationFilter
}

func (a *app) visibleRecommendations(rooms []platforms.Room) []platforms.Room {
	if a.page != "home" || !a.homeRecommendationFilterEnabled() || len(a.library.Preferences.HiddenHomeRooms) == 0 {
		return rooms
	}
	visible := make([]platforms.Room, 0, len(rooms))
	for _, room := range rooms {
		if !a.library.Preferences.HiddenHomeRooms[roomKey(room)] {
			visible = append(visible, room)
		}
	}
	return visible
}

func (a *app) setRecommendationHidden(key string, hidden bool) bool {
	next := a.library.Preferences
	next.HiddenHomeRooms = maps.Clone(next.HiddenHomeRooms)
	if hidden {
		if next.HiddenHomeRooms == nil {
			next.HiddenHomeRooms = make(map[string]bool)
		}
		next.HiddenHomeRooms[key] = true
	} else {
		delete(next.HiddenHomeRooms, key)
	}
	return a.savePreferences(next)
}

func (a *app) hideRecommendation(c *ui.Context, room platforms.Room) {
	key := roomKey(room)
	if !a.setRecommendationHidden(key, true) {
		return
	}
	c.AddToast(ui.Toast{
		ID: "recommendation:" + key, Title: "已减少推荐 " + a.displayRoomName(room),
		Timeout: 7 * time.Second, Action: "撤销",
		OnAction: func() { a.setRecommendationHidden(key, false) },
	})
}

func (a *app) recommendationSettings(c *ui.Context) {
	if a.page != "home" || !a.homeRecommendationFilterEnabled() || len(a.library.Preferences.HiddenHomeRooms) == 0 {
		return
	}
	iconButton(c.Key("recommendation-settings"), iconMore, "管理首页推荐").Size(27, 25).Menu(func(m *ui.Menu) {
		if m.Item("恢复已隐藏的推荐").Chosen() {
			next := a.library.Preferences
			next.HiddenHomeRooms = nil
			if a.savePreferences(next) {
				a.toast = "已恢复首页推荐"
			}
		}
	})
}
