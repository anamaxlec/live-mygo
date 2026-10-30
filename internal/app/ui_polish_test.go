//go:build darwin || windows

package app

import (
	"github.com/egoist/mygo/ui"
	"live-mygo/internal/library"
	"live-mygo/internal/platforms"
	"live-mygo/internal/player"
	"path/filepath"
	"testing"
	"time"
)

func polishApp(t *testing.T) *app {
	s, _ := library.Load(filepath.Join(t.TempDir(), "library.json"))
	return &app{library: s, page: "bilibili", player: &player.Player{}, playerOps: make(chan func(*player.Player), 20), covers: map[string]*ui.Bitmap{}, coverLoading: map[string]bool{}, categories: map[platforms.Platform][]platforms.Category{}, categoryLoading: map[platforms.Platform]bool{}}
}
func TestDiscoverySeparatesFamiliarRoomsAndRecommendations(t *testing.T) {
	a := polishApp(t)
	a.page = "home"
	fav := platforms.Room{Platform: platforms.Bilibili, ID: "1", Title: "关注直播", Anchor: "关注主播", Live: true}
	old := platforms.Room{Platform: platforms.Douyu, ID: "2", Title: "历史直播", Anchor: "历史主播"}
	a.library.Favorites = []platforms.Room{fav}
	a.library.History = []platforms.Room{old}
	// 刷新尚未完成时也要按收藏里保存的开播状态优先展示。
	a.catalog = []platforms.Room{{Platform: platforms.Bilibili, ID: "3", Title: "推荐直播", Anchor: "推荐主播", Live: true}}
	tester := ui.NewTester(a.view, 1180, 920)
	for _, label := range []string{"关注正在直播", "最近观看", "为你推荐", "播放 关注主播 关注直播", "打开 历史主播 历史直播", "播放 推荐主播 推荐直播"} {
		if _, ok := tester.Find(label); !ok {
			t.Fatalf("缺少分组或操作 %q", label)
		}
	}
	liveHeading, _ := tester.Find("关注正在直播")
	recommendHeading, _ := tester.Find("为你推荐")
	if liveHeading.Y >= recommendHeading.Y {
		t.Fatal("关注正在直播没有排在推荐之前")
	}
	if err := tester.Click("查看全部最近观看"); err != nil {
		t.Fatal(err)
	}
	tester.Advance(time.Millisecond)
	if a.page != "history" {
		t.Fatal("分组入口未导航")
	}
}
func TestSidebarQuickControlsKeepBrowsingPage(t *testing.T) {
	a := polishApp(t)
	r := platforms.Room{Platform: platforms.Bilibili, ID: "1", Title: "测试标题", Anchor: "测试主播"}
	a.shown = true
	a.stream = &platforms.Stream{Room: r}
	a.volume = 60
	tester := ui.NewTester(a.view, 1180, 780)
	if err := tester.Click("暂停当前直播"); err != nil {
		t.Fatal(err)
	}
	if !a.stats.Paused || a.page != "bilibili" {
		t.Fatal("暂停改变了浏览页")
	}
	if err := tester.Click("静音当前直播"); err != nil {
		t.Fatal(err)
	}
	if !a.stats.Muted || a.page != "bilibili" {
		t.Fatal("静音改变了浏览页")
	}
}

func TestDiscoveryShowsLiveGroupWhenMetadataArrives(t *testing.T) {
	a := polishApp(t)
	a.page = "home"
	fav := platforms.Room{Platform: platforms.Bilibili, ID: "1", Title: "关注直播", Anchor: "关注主播", Live: true}
	a.library.Favorites = []platforms.Room{fav}
	a.library.History = []platforms.Room{{Platform: platforms.Douyu, ID: "2", Title: "最近直播", Anchor: "最近主播"}}
	tester := ui.NewTester(a.view, 1180, 780)
	a.libraryRooms = map[string]platforms.Room{roomKey(fav): fav}
	tester.Frame()
	tester.Frame()
	live, ok := tester.Find("播放 关注主播 关注直播")
	heading, headingOK := tester.Find("关注正在直播")
	if !ok || !headingOK || live.Y < heading.Y+heading.H || live.W > 310 || live.H < 150 {
		t.Fatalf("异步关注分组不应藏到视口上方，卡片应与推荐卡同级且不撑满整行：%+v", live)
	}
}
func TestCompactChatUsesLessSpaceAndKeepsSuperChatEmphasis(t *testing.T) {
	a := polishApp(t)
	a.chatMessages = []platforms.ChatMessage{{ID: 1, User: "观众", Text: "紧凑消息"}, {ID: 2, User: "支持者", Text: "支持主播", Kind: "superchat", Price: 30}}
	tester := ui.NewTester(func(c *ui.Context) { a.chatView(c) }, 286, 700)
	bubble, ok := tester.Find("观众：紧凑消息")
	if !ok {
		t.Fatal("气泡消息未显示")
	}
	a.library.Preferences.ChatCompact = true
	tester.Frame()
	compact, ok := tester.Find("观众：紧凑消息")
	if !ok || compact.H >= bubble.H-8 {
		t.Fatalf("紧凑列表未减少占位：%+v %+v", bubble, compact)
	}
	if _, ok := tester.Find("醒目留言  ¥30"); !ok {
		t.Fatal("醒目留言丢失强调")
	}
}
func TestHiddenControlsCanBeFocusedByKeyboardAndReleased(t *testing.T) {
	a := polishApp(t)
	a.page = "player"
	a.focusMode = true
	a.shown = true
	a.controlsUntil = time.Now().Add(-time.Second)
	tester := ui.NewTester(a.view, 1180, 780)
	tester.Key(0, ui.KeyK)
	tester.Advance(220 * time.Millisecond)
	tester.Advance(4 * time.Second)
	if _, ok := tester.Find("暂停 · Space"); !ok {
		t.Fatal("键盘唤起或聚焦保持失败")
	}
	if err := tester.Click("直播画面，双击切换全屏"); err != nil {
		t.Fatal(err)
	}
	tester.Advance(3500 * time.Millisecond)
	if _, ok := tester.Find("暂停 · Space"); ok {
		t.Fatal("退出控制区后未恢复自动隐藏")
	}
}
func TestRapidSidebarReversalFinishesAtRequestedGeometry(t *testing.T) {
	a := polishApp(t)
	a.page = "player"
	a.focusMode = true
	a.shown = true
	tester := ui.NewTester(a.view, 1180, 780)
	tester.SetTitleBar(ui.TitleBar{Left: 96, Height: 58})
	if err := tester.Click("显示 / 隐藏导航侧栏"); err != nil {
		t.Fatal(err)
	}
	tester.Advance(90 * time.Millisecond)
	if a.navWidth <= 0 || a.navWidth >= sidebarWidth {
		t.Fatal("侧栏没有短促宽度过渡")
	}
	if err := tester.Click("显示 / 隐藏导航侧栏"); err != nil {
		t.Fatal(err)
	}
	tester.Advance(350 * time.Millisecond)
	if a.navWidth > .1 {
		t.Fatal("反向打断后侧栏尺寸未收敛")
	}
	back, _ := tester.Find("返回列表 · Esc")
	if back.X < 108 {
		t.Fatal("窗口控件安全区被占用")
	}
}

func TestTabEntersControlsThenMovesToNextButton(t *testing.T) {
	a := polishApp(t)
	a.page, a.focusMode, a.shown = "player", true, true
	a.targetRoom = platforms.Room{Platform: platforms.Bilibili, ID: "1"}
	tester := ui.NewTester(a.view, 1180, 780)
	if err := tester.Click("直播画面，双击切换全屏"); err != nil {
		t.Fatal(err)
	}
	tester.Key(0, ui.KeyTab)
	tester.Advance(220 * time.Millisecond)
	if !tester.Focused("暂停 · Space") {
		t.Fatal("Tab 未进入控制条")
	}
	tester.Key(0, ui.KeyTab)
	if !tester.Focused(shortcutLabel("重新连接 · ⌘R")) {
		t.Fatal("Tab 被画面容器截获，无法前往下一个按钮")
	}
}
func TestRecommendedCaptionStyleDoesNotResetOtherPreferences(t *testing.T) {
	a := polishApp(t)
	a.library.Preferences.ChatCompact = true
	a.library.Preferences.DanmakuSize = 42
	a.restoreCaptionStyle()
	p := a.library.Preferences
	if p.DanmakuSize != 24 || p.DanmakuWeight != 500 || p.DanmakuLines != 4 || !p.DanmakuMerge || !p.ChatCompact {
		t.Fatal("推荐样式没有正确应用或覆盖了无关偏好")
	}
}

func TestRoomCardsDoNotShowDuplicateTitleTooltip(t *testing.T) {
	a := polishApp(t)
	a.page = "home"
	featured := platforms.Room{Platform: platforms.Bilibili, ID: "7", Title: "精选直播测试标题", Anchor: "精选主播", Live: true}
	card := platforms.Room{Platform: platforms.Bilibili, ID: "8", Title: "推荐卡片测试标题", Anchor: "推荐主播", Live: true}
	a.catalog = []platforms.Room{featured, card}
	tester := ui.NewTester(a.view, 1180, 920)
	for _, target := range []struct{ label, title string }{
		{"精选直播 " + featured.Anchor + " " + featured.Title, featured.Title},
		{"播放 " + card.Anchor + " " + card.Title, card.Title},
	} {
		rect, ok := tester.Find(target.label)
		if !ok {
			t.Fatalf("缺少卡片 %q：%v", target.label, tester.Texts())
		}
		tester.Move(rect.X+rect.W/2, rect.Y+rect.H/2)
		tester.Advance(800 * time.Millisecond)
		count := 0
		for _, text := range tester.Texts() {
			if text == target.title {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("卡片标题在悬停时出现了 %d 次，可能仍有重复标题提示：%v", count, tester.Texts())
		}
	}
}
