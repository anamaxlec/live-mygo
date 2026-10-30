//go:build darwin

package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"live-mygo/internal/platforms"
)

func TestPopoverFadesInOutAndCanReverseWithoutJump(t *testing.T) {
	open := false
	tester := ui.NewTester(func(c *ui.Context) {
		c.Root().Background(ui.Hex("#ffffff"))
		anchor := ui.Button(c, "锚点").Width(80).Height(30)
		animatedPopover(c, anchor, &open, false, func(panel ui.Element) {
			panel.Width(180).Height(100).Background(ui.Hex("#000000")).Label("测试浮层")
		})
	}, 400, 300)
	open = true
	tester.Frame()
	tester.Advance(80 * time.Millisecond)
	r, ok := tester.Find("测试浮层")
	if !ok {
		t.Fatal("打开时未保留浮层")
	}
	x, y := int(r.X+r.W/2), int(r.Y+r.H/2)
	half := tester.Image().RGBAAt(x, y).R
	if half == 0 || half == 255 {
		t.Fatal("打开直接跳到终态，没有淡入", half)
	}
	tester.Advance(200 * time.Millisecond)
	if tester.Image().RGBAAt(x, y).R != 0 {
		t.Fatal("浮层没有完成淡入")
	}
	open = false
	tester.Frame()
	tester.Advance(60 * time.Millisecond)
	// Advance 的最后一帧可能早于时钟终点；在同一时刻比较反向前后。
	tester.Frame()
	fading := tester.Image().RGBAAt(x, y).R
	if fading == 0 || fading == 255 {
		t.Fatal("关闭没有保留中间帧", fading)
	}
	open = true
	tester.Frame()
	if now := tester.Image().RGBAAt(x, y).R; now != fading {
		t.Fatalf("反向打断时透明度跳变：%d -> %d", fading, now)
	}
	tester.Advance(200 * time.Millisecond)
	open = false
	tester.Frame()
	tester.Advance(250 * time.Millisecond)
	if _, ok := tester.Find("测试浮层"); ok || tester.FramePending() {
		t.Fatal("浮层退出后仍占位或仍要求动画帧")
	}
}

func TestQuickSearchAnimatesAndReleasesFocusAfterExit(t *testing.T) {
	a := polishApp(t)
	a.page, a.focusMode, a.shown = "player", true, true
	a.targetRoom = platforms.Room{Platform: platforms.Bilibili, ID: "6"}
	a.stream = &platforms.Stream{Room: a.targetRoom}
	a.stats.Paused = true
	tester := ui.NewTester(func(c *ui.Context) { a.view(c); c.Root().Background(ui.Hex("#000000")) }, 1180, 780)
	tester.Click("直播画面，双击切换全屏")
	tester.Key(ui.Cmd, ui.KeyL)
	tester.Advance(80 * time.Millisecond)
	if _, ok := tester.Find("快捷搜索内容"); !ok {
		t.Fatal("搜索打开过渡丢失输入框")
	}
	tester.Advance(200 * time.Millisecond)
	r, _ := tester.Find("快捷搜索直播")
	x, y := int(r.X+r.W-20), int(r.Y+r.H-8)
	tester.Type("当前草稿")
	tester.Key(0, ui.KeyEscape)
	tester.Advance(60 * time.Millisecond)
	if fade := tester.Image().RGBAAt(x, y).R; fade == 0 || fade == 255 {
		t.Fatal("搜索关闭没有退出过渡", fade)
	}
	tester.Advance(250 * time.Millisecond)
	if _, ok := tester.Find("快捷搜索直播"); ok || !tester.Focused("直播画面，双击切换全屏") || !a.stats.Paused {
		t.Fatal("退出后未还原焦点或改变了播放状态")
	}
}

func TestConnectionInformationOnlyAppearsInsidePanel(t *testing.T) {
	a := polishApp(t)
	a.page, a.focusMode = "player", true
	a.targetRoom = platforms.Room{Platform: platforms.Douyu, ID: "1"}
	a.status = "房间未开播"
	a.chatStatus = "等待直播间连接"
	tester := ui.NewTester(a.view, 1180, 780)
	if _, ok := tester.Find("等待直播间连接"); ok {
		t.Fatal("底部或空状态仍重复显示连接提示")
	}
	if _, ok := tester.Find("解析房间：房间未开播"); !ok {
		t.Fatal("连接面板未保留失败原因")
	}
	a.shown = true
	a.chatRoomKey = roomKey(a.targetRoom)
	a.chatStatus = "弹幕连接失败"
	tester.Frame()
	if _, ok := tester.Find("连接弹幕：弹幕连接失败"); !ok {
		t.Fatal("视频已播放后，弹幕连接失败没有留在面板中")
	}
	if _, ok := tester.Find("重新连接弹幕"); !ok {
		t.Fatal("弹幕连接失败缺少面板内重试")
	}
	a.chatStatus = "已连接"
	tester.Frame()
	tester.Advance(250 * time.Millisecond)
	if _, ok := tester.Find("直播间连接状态"); ok {
		t.Fatal("全部连接完成后没有收起面板")
	}
}

func TestReducedMotionDoesNotKeepClosingPopover(t *testing.T) {
	open := false
	tester := ui.NewTester(func(c *ui.Context) {
		anchor := ui.Button(c, "锚点")
		animatedPopover(c, anchor, &open, false, func(panel ui.Element) { panel.Size(100, 60).Label("测试浮层") })
	}, 300, 240)
	tester.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
	open = true
	tester.Frame()
	open = false
	tester.Frame()
	if _, ok := tester.Find("测试浮层"); ok {
		t.Fatal("减少动态效果设置仍等待浮层移动")
	}
}
