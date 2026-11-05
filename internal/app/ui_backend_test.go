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
	if _, ok := tester.Find("重启以切换播放内核？"); !ok || !a.settings.backendRestartOpen {
		t.Fatal("切换内核后未弹出重启选择")
	}
	if err := tester.Click("稍后再说"); err != nil {
		t.Fatal(err)
	}
	if a.settings.backendRestartOpen || a.library.Preferences.PlayerBackend != "vt" || a.page != "player" || !a.shown {
		t.Fatal("稍后重启应保留设置与当前播放")
	}
	if _, ok := tester.Find("当前使用：mpv"); !ok {
		t.Fatal("未显示实际运行内核")
	}
	if err := tester.Click("重启生效"); err != nil {
		t.Fatal(err)
	}
	if !a.settings.backendRestartOpen {
		t.Fatal("稍后重启入口不可用")
	}
	if err := tester.Click("稍后再说"); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click("原生硬解（测试）"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if err := tester.Click("mpv"); err != nil {
		t.Fatal(err)
	}
	if a.settings.backendRestartOpen {
		t.Fatal("选择当前运行内核不应要求重启")
	}
}
