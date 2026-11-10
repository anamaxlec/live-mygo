//go:build darwin || windows

package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"live-mygo/internal/platforms"
)

func TestHideHomeRecommendationCanBeUndoneWithoutReload(t *testing.T) {
	a := polishApp(t)
	a.page = "home"
	room := platforms.Room{Platform: platforms.Douyu, ID: "1", Title: "赛事", Anchor: "测试主播", Live: true}
	a.catalog = []platforms.Room{room}
	a.homeCatalog.rooms = a.catalog
	tester := ui.NewTester(a.view, 1180, 780)
	if err := tester.Click("推荐操作 · 测试主播"); err != nil {
		t.Fatal(err)
	}
	if err := tester.ChooseMenuItem("少推荐这个主播"); err != nil {
		t.Fatal(err)
	}
	if _, found := tester.Find("精选直播 测试主播 赛事"); found {
		t.Fatal("推荐卡片没有消失")
	}
	if len(a.catalog) != 1 || len(a.homeCatalog.rooms) != 1 {
		t.Fatal("筛选破坏了原始推荐或首页缓存")
	}
	room.Title = "新一场赛事"
	a.catalog = []platforms.Room{room}
	tester.Frame()
	if _, found := tester.Find("精选直播 测试主播 新一场赛事"); found {
		t.Fatal("更新推荐后隐藏的房间重新出现")
	}
	a.page = "douyu"
	if got := a.visibleRecommendations(a.catalog); len(got) != 1 {
		t.Fatal("首页筛选影响了平台页")
	}
	a.page = "home"
	settings := ui.NewTester(a.generalSettings, 590, 600)
	clickPreferenceSwitch(t, settings, "首页推荐筛选")
	tester.Frame()
	if _, found := tester.Find("精选直播 测试主播 新一场赛事"); !found {
		t.Fatal("关闭首页推荐筛选后没有显示原推荐")
	}
	if _, found := tester.Find("推荐操作 · 测试主播"); found {
		t.Fatal("关闭筛选后仍显示推荐操作")
	}
	clickPreferenceSwitch(t, settings, "首页推荐筛选")
	a.library.Favorites = []platforms.Room{room}
	tester.Frame()
	if _, found := tester.Find("播放 测试主播 新一场赛事"); !found {
		t.Fatal("首页筛选影响了正在直播的关注")
	}
	a.library.Favorites = nil
	tester.Frame()
	if err := tester.Click("撤销"); err != nil {
		t.Fatal(err)
	}
	if _, found := tester.Find("精选直播 测试主播 新一场赛事"); !found {
		t.Fatal("撤销没有立即恢复推荐卡片")
	}
	a.library.Favorites = []platforms.Room{{Platform: platforms.Huya, ID: "2", Title: "另一场直播", Live: true}}
	tester.Frame()
	if err := tester.Click("推荐操作 · 测试主播"); err != nil {
		t.Fatal(err)
	}
	if err := tester.ChooseMenuItem("少推荐这个主播"); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click("管理首页推荐"); err != nil {
		t.Fatal(err)
	}
	if err := tester.ChooseMenuItem("恢复已隐藏的推荐"); err != nil {
		t.Fatal(err)
	}
	if _, found := tester.Find("播放 测试主播 新一场赛事"); !found {
		t.Fatal("推荐管理没有恢复普通推荐卡片")
	}
}
