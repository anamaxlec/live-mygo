//go:build darwin || windows

package app

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"live-mygo/internal/library"
	"live-mygo/internal/platforms"
)

func TestRoomPlaybackPreferencesStayWithTheirRoom(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	a := polishApp(t)
	a.library, _ = library.Load(path)
	first := platforms.Room{Platform: platforms.Douyu, ID: "7", Title: "赛事直播"}
	second := platforms.Room{Platform: platforms.Huya, ID: "7", Title: "聊天直播"}
	a.targetRoom, a.title = first, first.Title
	a.stream = &platforms.Stream{Room: first, QualityID: 2, Qualities: []platforms.Quality{{ID: 0, Name: "原画"}, {ID: 2, Name: "高清"}}}
	a.page, a.shown, a.focusMode = "player", true, true
	a.controlsUntil = time.Now().Add(time.Hour)
	tester := ui.NewTester(a.view, 1180, 780)
	for _, label := range []string{"关闭画面弹幕 · D", "收起弹幕列表"} {
		if err := tester.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	afterToggle, err := library.Load(path)
	if err != nil || afterToggle.Preferences.RoomPlayback[roomKey(first)].Danmaku ||
		afterToggle.Preferences.RoomPlayback[roomKey(first)].ChatPanel || afterToggle.Preferences.RoomPlayback[roomKey(first)].Quality != -1 {
		t.Fatal("弹幕开关没有自动保存房间偏好", err)
	}
	a.qualityLabel = "高清"
	a.rememberRoomPlayback(true)
	reloaded, err := library.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	a.library = reloaded
	if a.chatVisible() || a.barrageVisible() {
		t.Fatal("重载后没有恢复该房间的弹幕设置")
	}
	if id, ok := preferredRoomQuality(a.stream, reloaded.Preferences.RoomPlayback, platforms.QualityHighest); !ok || id != 2 {
		t.Fatalf("未优先选择保存的画质，得到 %d", id)
	}
	a.stream.Qualities = append(a.stream.Qualities[:1], platforms.Quality{ID: 2, Name: "标清"}, platforms.Quality{ID: 3, Name: "高清"})
	if id, ok := preferredRoomQuality(a.stream, reloaded.Preferences.RoomPlayback, platforms.QualityHighest); !ok || id != 3 {
		t.Fatalf("档位编号变化后没有按原画质名称选择，得到 %d", id)
	}
	a.stream.Qualities = a.stream.Qualities[:1]
	if id, ok := preferredRoomQuality(a.stream, reloaded.Preferences.RoomPlayback, platforms.QualityHighest); !ok || id != 0 {
		t.Fatalf("原画质不可用时没有回退，得到 %d", id)
	}
	a.selectTargetRoom(second)
	if !a.chatVisible() || !a.barrageVisible() {
		t.Fatal("房间偏好影响了另一个平台的同号房间")
	}
	a.selectTargetRoom(first)
	want := a.library.Preferences.RoomPlayback[roomKey(first)]
	settings := ui.NewTester(a.playbackSettings, 590, 600)
	clickPreferenceSwitch(t, settings, "记住每个直播间的观看设置")
	if !a.library.Preferences.DisableRoomPlaybackMemory || !a.chatVisible() || !a.barrageVisible() {
		t.Fatal("关闭记忆后没有使用全局设置")
	}
	a.toggleBarrage()
	a.toggleChat()
	if a.library.Preferences.RoomPlayback[roomKey(first)] != want {
		t.Fatal("关闭记忆后仍写入房间偏好")
	}
	clickPreferenceSwitch(t, settings, "记住每个直播间的观看设置")
	a.clearRoomPlayback()
	if !a.chatVisible() || !a.barrageVisible() || len(a.library.Preferences.RoomPlayback) != 0 {
		t.Fatal("清除房间偏好后没有恢复全局设置")
	}
}
