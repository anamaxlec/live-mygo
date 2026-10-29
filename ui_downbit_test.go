//go:build darwin

package main

import (
	"github.com/egoist/mygo/ui"
	"live-mygo/internal/platforms"
	"testing"
	"time"
)

func TestQuickSearchKeepsPlaybackAndDismissesWithoutLeavingRoom(t *testing.T) {
	a := polishApp(t)
	a.page, a.focusMode, a.shown = "player", true, true
	a.platformLabel = "哔哩哔哩"
	a.targetRoom = platforms.Room{Platform: platforms.Bilibili, ID: "6", Title: "当前直播", Anchor: "当前主播"}
	a.stream = &platforms.Stream{Room: a.targetRoom}
	a.stats.Paused = true
	tester := ui.NewTester(a.view, 1180, 780)
	if err := tester.Click("直播画面，双击切换全屏"); err != nil {
		t.Fatal(err)
	}
	tester.Key(ui.Cmd, ui.KeyL)
	if _, ok := tester.Find("快捷搜索内容"); !ok {
		t.Fatal("没有唤出快捷搜索")
	}
	if !tester.Focused("快捷搜索内容") {
		t.Fatal("搜索未取得键盘焦点")
	}
	tester.Type("其他主播")
	tester.Key(0, ui.KeySpace)
	if !a.stats.Paused || a.page != "player" || !a.focusMode {
		t.Fatal("输入搜索改变了播放或专注布局")
	}
	cancelled := false
	a.quick.cancel = func() { cancelled = true }
	a.quick.loading = true
	tester.Key(0, ui.KeyEscape)
	if a.quick.open || !cancelled || a.page != "player" || a.stream.Room.ID != "6" {
		t.Fatal("取消搜索未保持原直播或未取消请求")
	}
}

func TestConnectionStatesSeparateVideoAndChatAndIgnoreOldRoom(t *testing.T) {
	a := polishApp(t)
	a.busy = true
	a.targetRoom = platforms.Room{Platform: platforms.Douyu, ID: "24422", Title: "新直播"}
	steps := a.connectionSteps()
	if steps[0].state != "active" || steps[1].state != "waiting" {
		t.Fatal(steps)
	}
	a.stream = &platforms.Stream{Room: a.targetRoom}
	steps = a.connectionSteps()
	if steps[0].state != "done" || steps[1].state != "active" {
		t.Fatal(steps)
	}
	a.busy, a.shown = false, true
	a.playStarted = time.Now()
	a.chatRoomKey = "bilibili:6"
	a.chatStatus = "已连接"
	if a.connectionSteps()[2].state == "done" {
		t.Fatal("旧房间的弹幕状态污染当前房间")
	}
	if !a.waitingForVideo() {
		t.Fatal("已接受加载命令但未解码时应保留连接状态")
	}
	a.decodedMS = 100
	a.chatRoomKey = roomKey(a.targetRoom)
	a.chatStatus = "弹幕服务连接失败"
	steps = a.connectionSteps()
	if steps[1].state != "done" || steps[2].state != "failed" || a.waitingForVideo() {
		t.Fatal("弹幕失败不应遮挡正常视频", steps)
	}
	a.shown = false
	a.status = "视频源连接失败"
	if a.connectionSteps()[1].state != "failed" {
		t.Fatal("视频连接失败未单独标出")
	}
}

func TestCategorizedSettingsExposeExistingPreferences(t *testing.T) {
	a := polishApp(t)
	a.accountStatus = map[platforms.Platform]string{platforms.Bilibili: "未登录", platforms.Douyu: "未登录"}
	tester := ui.NewTester(a.settingsView, 820, 640)
	for _, label := range []string{"通用", "播放", "画面弹幕", "弹幕列表", "账号", "关于"} {
		if _, ok := tester.Find("设置分类：" + label); !ok {
			t.Fatal("缺少分类", label)
		}
	}
	if err := tester.Click("设置分类：播放"); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"窗口跟随画面比例", "斗鱼弹幕兼容连接", "离开直播间时暂停"} {
		if _, ok := tester.Find(label); !ok {
			t.Fatal("缺少播放设置", label)
		}
	}
	if err := tester.Click("设置分类：画面弹幕"); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"字体", "字重", "字号", "描边", "推荐样式"} {
		if _, ok := tester.Find(label); !ok {
			t.Fatal("缺少完整弹幕设置", label)
		}
	}
	if err := tester.Click("设置分类：弹幕列表"); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"列表样式", "显示弹幕列表", "显示连接面板", "显示官网互动入口"} {
		if _, ok := tester.Find(label); !ok {
			t.Fatal("缺少列表设置", label)
		}
	}
	before := a.library.Preferences.ChatConnection
	// Find 返回左侧文本；点击它右侧的实际 Switch。
	row, _ := tester.Find("显示连接面板")
	tester.ClickAt(row.X+row.W+25, row.Y+row.H/2)
	if a.library.Preferences.ChatConnection == before {
		t.Fatal("完整设置页未写入原有偏好")
	}
}

func TestFailedConnectionKeepsIdentityAndRecoveryActions(t *testing.T) {
	a := polishApp(t)
	a.page, a.focusMode = "player", true
	a.targetRoom = platforms.Room{Platform: platforms.Douyu, ID: "24422", Title: "同一个房间", Anchor: "同一个主播"}
	a.title = a.targetRoom.Title
	a.status = "房间未开播"
	tester := ui.NewTester(a.view, 1000, 580)
	for _, label := range []string{"同一个房间", "解析房间：房间未开播", "重新连接", "返回直播列表"} {
		if _, ok := tester.Find(label); !ok {
			t.Fatal("错误状态缺少身份或恢复操作", label)
		}
	}
	video, _ := tester.Find("直播画面，双击切换全屏")
	status, _ := tester.Find("直播间连接状态")
	if status.X < video.X+video.W || status.X+status.W > 1001 {
		t.Fatalf("连接信息必须完全位于弹幕区：video=%+v status=%+v", video, status)
	}
}

func TestConnectionStatusStaysOutOfVideoWhenChatIsHidden(t *testing.T) {
	a := polishApp(t)
	a.page, a.focusMode = "player", true
	a.targetRoom = platforms.Room{Platform: platforms.Douyu, ID: "24422"}
	a.status = "连接失败"
	a.library.Preferences.ChatPanel = false
	tester := ui.NewTester(a.view, 1000, 580)
	if _, ok := tester.Find("直播间连接状态"); ok {
		t.Fatal("弹幕区收起时仍显示连接面板")
	}
	if _, ok := tester.Find("重新连接 · ⌘R"); !ok {
		t.Fatal("弹幕区收起后丢失播放器重试入口")
	}
	a.library.Preferences.ChatPanel = true
	a.library.Preferences.ChatConnection = false
	tester.Frame()
	tester.Advance(300 * time.Millisecond)
	if _, ok := tester.Find("直播间连接状态"); ok {
		t.Fatal("隐藏连接状态的偏好未生效")
	}
}

func TestEmptyLibraryProvidesBrowseAction(t *testing.T) {
	a := polishApp(t)
	a.page = "favorites"
	tester := ui.NewTester(a.view, 1000, 580)
	if _, ok := tester.Find("浏览直播"); !ok {
		t.Fatal("空关注缺少可执行下一步")
	}
}

func TestConnectionLayerFadesWithoutMovingEmptyChat(t *testing.T) {
	a := polishApp(t)
	a.page, a.focusMode, a.shown = "player", true, true
	a.targetRoom = platforms.Room{Platform: platforms.Douyu, ID: "24422"}
	a.stream = &platforms.Stream{Room: a.targetRoom}
	a.chatRoomKey, a.chatStatus = roomKey(a.targetRoom), "已连接"
	a.playStarted = time.Now()
	a.decodedMS = 100
	tester := ui.NewTester(a.view, 1000, 580)
	before, ok := tester.Find("等一条新弹幕")
	if !ok {
		t.Fatal("缺少弹幕空状态")
	}
	a.shown = false
	a.busy = true
	tester.Frame()
	tester.Advance(90 * time.Millisecond)
	during, _ := tester.Find("等一条新弹幕")
	if during != before {
		t.Fatalf("连接叠层移动了空状态：%+v -> %+v", before, during)
	}
	if _, ok := tester.Find("直播间连接状态"); !ok {
		t.Fatal("连接叠层未显示")
	}
	a.shown = true
	a.busy = false
	tester.Frame()
	tester.Advance(60 * time.Millisecond)
	if _, ok := tester.Find("直播间连接状态"); !ok {
		t.Fatal("连接状态直接移除，没有保留淡出")
	}
	tester.Advance(250 * time.Millisecond)
	if _, ok := tester.Find("直播间连接状态"); ok {
		t.Fatal("淡出后连接叠层仍未移除")
	}
	after, _ := tester.Find("等一条新弹幕")
	if after != before {
		t.Fatalf("淡出改变了空状态布局：%+v -> %+v", before, after)
	}
}

func TestQuickSearchDefaultsToCurrentRoomPlatform(t *testing.T) {
	a := polishApp(t)
	a.page = "player"
	a.platformLabel = "哔哩哔哩"
	a.targetRoom = platforms.Room{Platform: platforms.Douyu, ID: "24422"}
	a.openQuickSearch()
	if a.quick.platform != "斗鱼" {
		t.Fatal("直接跨平台开房后，房间号搜索仍使用旧平台")
	}
}
