package library

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"live-mygo/internal/platforms"
)

func TestFavoriteDetailsSurviveRefreshAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s, _ := Load(path)
	room := platforms.Room{Platform: platforms.Douyu, ID: "1", Anchor: "主播"}
	if err := s.Toggle(room); err != nil {
		t.Fatal(err)
	}
	want := FavoriteDetails{Alias: "我的别名", Group: "赛事", Pinned: true}
	if err := s.SetFavoriteDetails(room, want); err != nil {
		t.Fatal(err)
	}
	room.Title = "新的直播标题"
	if err := s.UpdateFavorites([]platforms.Room{room}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Load(path)
	if err != nil || reloaded.Details(room) != want || reloaded.Favorites[0].Title != room.Title {
		t.Fatal("刷新覆盖了本地关注设置", reloaded, err)
	}
}

func TestBackupMergesFavoritesAndPreservesCredentialsBoundary(t *testing.T) {
	s, _ := Load(filepath.Join(t.TempDir(), "library.json"))
	a := platforms.Room{Platform: platforms.Bilibili, ID: "1"}
	b := platforms.Room{Platform: platforms.Douyu, ID: "2"}
	s.Toggle(a)
	if err := s.ImportBackup([]byte(`{"favorites":[{"platform":"douyu","id":"2"}],"favorite_details":{"douyu:2":{"alias":"赛事","pinned":true}},"preferences":{"volume":25},"cookies":"ignored-secret"}`)); err != nil {
		t.Fatal(err)
	}
	if !s.Has(a) || !s.Has(b) || s.Details(b).Alias != "赛事" || s.Preferences.Volume != 25 || s.Preferences.DanmakuSize != 24 {
		t.Fatal("导入没有合并关注或改变了未指定偏好", s)
	}
	backup, err := s.Backup()
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(backup, &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["cookies"]; ok {
		t.Fatal("备份保存了凭据")
	}
	if _, ok := payload["history"]; ok {
		t.Fatal("备份包含观看历史")
	}
}

func TestBackupMergesRoomPlaybackAndHiddenRecommendations(t *testing.T) {
	source, _ := Load(filepath.Join(t.TempDir(), "library.json"))
	key := "douyu:7"
	view := RoomPlaybackPreferences{Quality: 2, QualityName: "高清", ChatPanel: true}
	source.Preferences.RoomPlayback = map[string]RoomPlaybackPreferences{key: view}
	source.Preferences.HiddenHomeRooms = map[string]bool{key: true}
	backup, err := source.Backup()
	if err != nil {
		t.Fatal(err)
	}

	target, _ := Load(filepath.Join(t.TempDir(), "library.json"))
	other := "huya:7"
	target.Preferences.RoomPlayback = map[string]RoomPlaybackPreferences{other: {Quality: 4}}
	target.Preferences.HiddenHomeRooms = map[string]bool{other: true}
	if err := target.ImportBackup(backup); err != nil {
		t.Fatal(err)
	}
	if target.Preferences.RoomPlayback[key] != view || target.Preferences.RoomPlayback[other].Quality != 4 ||
		!target.Preferences.HiddenHomeRooms[key] || !target.Preferences.HiddenHomeRooms[other] {
		t.Fatal("备份没有合并房间偏好和首页筛选")
	}
	if err := target.ImportBackup([]byte(`{"preferences":{"room_playback":{"douyu:7":{"quality":9}},"hidden_home_rooms":{"douyu:7":false}},"favorites":[{"platform":"unknown","id":"1"}]}`)); err == nil {
		t.Fatal("无效备份未被拒绝")
	}
	if target.Preferences.RoomPlayback[key] != view || !target.Preferences.HiddenHomeRooms[key] {
		t.Fatal("导入失败覆盖了当前偏好")
	}
}
