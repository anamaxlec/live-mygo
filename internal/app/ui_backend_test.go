//go:build darwin || windows

package app

import (
	"github.com/egoist/mygo/ui"
	"live-mygo/internal/library"
	"live-mygo/internal/player"
	"path/filepath"
	"testing"
)

func TestPlaybackBackendSelectorPersistsWithoutInterruptingRoom(t *testing.T) {
	a := polishApp(t)
	var err error
	a.library, err = library.Load(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	a.settings.tab = settingsPlayback
	a.page, a.title, a.shown = "player", "正在观看的房间", true
	tester := ui.NewTester(a.settingsView, 820, 1000)
	if !player.NativeBackendAvailable() {
		if _, ok := tester.Find("原生硬解（测试）"); ok {
			t.Fatal("不支持的平台不应显示原生内核")
		}
		return
	}
	if err := tester.Click("mpv"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if err := tester.Click("原生硬解（测试）"); err != nil {
		t.Fatal(err)
	}
	if a.library.Preferences.PlayerBackend != "vt" || a.page != "player" || !a.shown || a.title != "正在观看的房间" {
		t.Fatal("切换设置不应停止当前房间")
	}
	if _, ok := tester.Find("重新打开应用后生效。当前使用：mpv"); !ok {
		t.Fatal("未说明当前内核与生效时机")
	}
}
