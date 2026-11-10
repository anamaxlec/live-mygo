//go:build darwin || windows

package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"live-mygo/internal/platforms"
)

func TestSplitViewSwapsThePrimaryPictureAndItsRoom(t *testing.T) {
	a := polishApp(t)
	a.page, a.focusMode, a.shown = "player", true, true
	a.title = "哔哩哔哩测试画面"
	a.targetRoom = platforms.Room{Platform: platforms.Bilibili, ID: "123"}
	a.sideViewer = &app{parent: a, embedded: true, shown: true, page: "player", library: a.library,
		title: "斗鱼测试画面", targetRoom: platforms.Room{Platform: platforms.Douyu, ID: "456"}}
	view := ui.NewTester(a.view, 1180, 780)
	main, ok := view.Find("直播画面，双击切换全屏")
	if !ok {
		t.Fatal("缺少主画面")
	}
	secondary, ok := view.Find("副画面")
	if !ok || secondary.X < main.X+main.W-1 {
		t.Fatalf("副画面没有在主画面右侧：主=%+v 副=%+v", main, secondary)
	}
	if err := view.Click("交换主副画面"); err != nil {
		t.Fatal(err)
	}
	if a.featuredViewer() != a.sideViewer || !view.HasText("斗鱼测试画面") {
		t.Fatal("交换画面后主播放室没有切换")
	}
}
