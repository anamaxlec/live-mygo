//go:build darwin || windows

package app

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"live-mygo/internal/library"
	"live-mygo/internal/platforms"
)

func TestFiveVersionClicksUnlockAndPersistRaceMenu(t *testing.T) {
	a := polishApp(t)
	path := filepath.Join(t.TempDir(), "library.json")
	state, err := library.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	a.library = state
	tester := ui.NewTester(a.aboutView, 650, 600)
	for i := 0; i < 4; i++ {
		if err := tester.Click("版本号"); err != nil {
			t.Fatal(err)
		}
	}
	if a.raceMenuUnlocked() {
		t.Fatal("不足五次就开启彩蛋")
	}
	if err := tester.Click("版本号"); err != nil {
		t.Fatal(err)
	}
	if !a.raceMenuUnlocked() {
		t.Fatal("五次点击未开启")
	}
	loaded, err := library.Load(path)
	if err != nil || !loaded.Preferences.RaceMenu {
		t.Fatal("彩蛋入口未保存", err)
	}
	for _, p := range a.sourcePlatforms() {
		if p == platforms.FLive {
			return
		}
	}
	t.Fatal("开启后没有加入来源入口")
}

func TestRaceSourceHasNoChatControls(t *testing.T) {
	a := polishApp(t)
	a.page, a.shown, a.focusMode = "player", true, true
	a.targetRoom = platforms.Room{Platform: platforms.FLive, ID: "1", Title: "F1 TV"}
	a.stream = &platforms.Stream{Room: a.targetRoom, Sources: []platforms.Source{{ID: "1", Name: "线路 1"}}, SourceID: "1"}
	a.revealControls()
	tester := ui.NewTester(a.view, 1180, 780)
	for _, label := range []string{"弹幕列表设置", "弹幕列表操作", "关闭画面弹幕 · D", "连接弹幕：等待视频连接"} {
		if _, ok := tester.Find(label); ok {
			t.Fatal("无弹幕来源仍显示弹幕控件", label)
		}
	}
	if _, ok := tester.Find("播放线路列表"); !ok {
		t.Fatal("没有显示原生线路列表")
	}
	if _, ok := tester.Find("收起线路列表"); !ok {
		t.Fatal("没有切换线路列表的入口")
	}
}

func TestRaceRoutesReceiveClicksUnderConnectionOverlay(t *testing.T) {
	for _, busy := range []bool{false, true} {
		a := polishApp(t)
		a.targetRoom = platforms.Room{Platform: platforms.FLive, ID: "1", Title: "Sky Sports F1"}
		a.stream = &platforms.Stream{Room: a.targetRoom, SourceID: "1", Sources: []platforms.Source{{ID: "1", Name: "线路 1"}, {ID: "2", Name: "线路 2"}}}
		a.busy, a.status = busy, "频道信号连接失败，请更换线路"
		chosen := ""
		tester := ui.NewTester(func(c *ui.Context) {
			ui.Box(c).Fill().Children(func() {
				if id := raceSourceList(c, a.stream, busy); id != "" {
					chosen = id
				}
				a.connectionOverlay(c, 1)
			})
		}, 286, 700)
		if err := tester.Click("切换赛事线路 线路 2"); err != nil {
			t.Fatal(err)
		}
		if chosen != "2" {
			t.Fatalf("连接叠层遮挡线路或连接中无法换线：busy=%v selected=%q", busy, chosen)
		}
	}
}
