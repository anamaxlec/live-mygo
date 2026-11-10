//go:build darwin || windows

package app

import (
	"maps"

	"github.com/egoist/mygo/ui"
	"live-mygo/internal/library"
	"live-mygo/internal/platforms"
)

func preferredRoomQuality(stream *platforms.Stream, profiles map[string]library.RoomPlaybackPreferences, fallback string) (int, bool) {
	if profile, ok := profiles[roomKey(stream.Room)]; ok && profile.Quality >= 0 {
		match, found := 0, false
		for _, quality := range stream.Qualities {
			if profile.QualityName != "" && quality.Name != profile.QualityName {
				continue
			}
			if quality.ID == profile.Quality {
				return quality.ID, true
			}
			if profile.QualityName != "" && !found {
				match, found = quality.ID, true
			}
		}
		if found {
			return match, true
		}
	}
	return platforms.PreferredQuality(stream.Qualities, fallback)
}

func (a *app) roomPlaybackPreferences(prefs library.Preferences) library.RoomPlaybackPreferences {
	view, ok := prefs.RoomPlayback[roomKey(a.targetRoom)]
	if !ok || prefs.DisableRoomPlaybackMemory {
		view = library.RoomPlaybackPreferences{Quality: -1, Danmaku: prefs.Danmaku, ChatPanel: prefs.ChatPanel}
	}
	if a.roomDanmaku != nil {
		view.Danmaku = *a.roomDanmaku
	}
	if a.roomChatPanel != nil {
		view.ChatPanel = *a.roomChatPanel
	}
	return view
}

func (a *app) roomPreferencesMenu(m *ui.Menu) {
	if a.library == nil || a.library.Preferences.DisableRoomPlaybackMemory || a.targetRoom.ID == "" {
		return
	}
	if _, saved := a.library.Preferences.RoomPlayback[roomKey(a.targetRoom)]; saved {
		m.Separator()
		if m.Item("清除此直播间的观看设置").Chosen() {
			a.clearRoomPlayback()
		}
	}
}

func (a *app) rememberRoomPlayback(rememberQuality bool) {
	if a.library == nil || a.library.Preferences.DisableRoomPlaybackMemory || a.stream == nil || roomKey(a.stream.Room) != roomKey(a.targetRoom) {
		return
	}
	next := a.library.Preferences
	view := a.roomPlaybackPreferences(next)
	if rememberQuality {
		view.Quality = a.stream.QualityID
		view.QualityName = a.qualityLabel
	}
	next.RoomPlayback = maps.Clone(next.RoomPlayback)
	if next.RoomPlayback == nil {
		next.RoomPlayback = make(map[string]library.RoomPlaybackPreferences)
	}
	next.RoomPlayback[roomKey(a.targetRoom)] = view
	if a.savePreferences(next) {
		a.roomDanmaku, a.roomChatPanel = nil, nil
	}
}

func (a *app) clearRoomPlayback() {
	next := a.library.Preferences
	barrageWasVisible := a.barrageVisible()
	next.RoomPlayback = maps.Clone(next.RoomPlayback)
	delete(next.RoomPlayback, roomKey(a.targetRoom))
	if a.savePreferences(next) {
		a.roomDanmaku, a.roomChatPanel = nil, nil
		if barrageWasVisible && !a.barrageVisible() {
			a.clearFlyingChat()
		}
		a.fitWindowToVideo(false)
		a.toast = "已清除此直播间的观看设置"
	}
}
