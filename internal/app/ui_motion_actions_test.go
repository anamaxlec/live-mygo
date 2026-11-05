//go:build darwin || windows

package app

import (
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"live-mygo/internal/platforms"
)

func TestBackupActionsUseSettingsInsetsAndEqualButtonGeometry(t *testing.T) {
	for _, width := range []float32{460, 560} {
		a := polishApp(t)
		tester := ui.NewTester(func(c *ui.Context) { c.Root().Padding(24).Background(ui.Hex("#ffffff")); a.backupSettings(c) }, int(width), 180)
		in, ok := tester.Find("导入并合并备份")
		out, ok2 := tester.Find("导出备份")
		title, ok3 := tester.Find("关注与偏好")
		if !ok || !ok2 || !ok3 || in.W != out.W || in.H != out.H || math.Abs(float64(in.Y-out.Y)) > .1 || in.X < title.X+title.W || out.X+out.W > width-38 {
			t.Fatal("数据操作未按设置行对齐", width, in, out, title)
		}
		if _, ok := tester.Find("关注、分组、偏好与屏蔽规则；不包含账号或观看历史。"); ok {
			t.Fatal("多余说明仍占据分组底部")
		}
	}
}

func TestChatEnvelopeKeepsNewMessagesInteractiveAndStopsAnimating(t *testing.T) {
	for _, reduced := range []bool{false, true} {
		a := polishApp(t)
		a.targetRoom = platforms.Room{Platform: platforms.Douyu, ID: "1"}
		a.chatMessages = []platforms.ChatMessage{{ID: 1, User: "原消息", Text: "准备收好"}}
		tester := ui.NewTester(a.chatView, 320, 540)
		tester.SetPreferences(ui.Preferences{ReduceMotion: reduced, TextScale: 1})
		a.clearChatWithMotion()
		tester.Frame()
		tester.Advance(180 * time.Millisecond)
		a.chatMessages = append(a.chatMessages, platforms.ChatMessage{ID: 2, User: "新消息", Text: "继续接收"})
		tester.Frame()
		if _, ok := tester.Find("新消息：继续接收"); !ok {
			t.Fatal("收纳动画阻碍了新弹幕")
		}
		tester.Move(0, 0)
		tester.Advance(1400 * time.Millisecond)
		if tester.FramePending() {
			t.Fatal("收纳完成后仍在请求帧", reduced)
		}
	}
}

func TestClearingFromChatMenuKeepsCurrentFrameIndicesValid(t *testing.T) {
	a := polishApp(t)
	for i := 0; i < 60; i++ {
		a.chatMessages = append(a.chatMessages, platforms.ChatMessage{ID: uint64(i + 1), User: "观众", Text: "准备清空的消息"})
	}
	tester := ui.NewTester(a.chatView, 320, 540)
	if err := tester.Click("弹幕列表操作"); err != nil {
		t.Fatal(err)
	}
	if err := tester.ChooseMenuItem("清空当前弹幕"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	tester.Advance(100 * time.Millisecond)
	if len(a.chatMessages) != 0 {
		t.Fatal("菜单没有清空数据")
	}
	tester.Advance(time.Second)
	if _, ok := tester.Find("等一条新弹幕"); !ok {
		t.Fatal("清空后没有恢复空状态")
	}
}

func TestChannelTuningIsBoundedAndDoesNotChangeRoomState(t *testing.T) {
	a := polishApp(t)
	room := platforms.Room{Platform: platforms.Douyu, ID: "1", Anchor: "第一频道", Title: "原房间"}
	tester := ui.NewTester(func(c *ui.Context) { ui.Box(c).Width(210).Children(func() { a.channelCardIdentity(c, room) }) }, 240, 100)
	room = platforms.Room{Platform: platforms.Douyu, ID: "2", Anchor: "第二频道", Title: "新房间"}
	tester.Frame()
	tester.Advance(120 * time.Millisecond)
	if room.ID != "2" {
		t.Fatal("快门动画改变了目标房间")
	}
	tester.Advance(time.Second)
	if _, ok := tester.Find("第二频道"); !ok || tester.FramePending() {
		t.Fatal("换台没有落定")
	}
}

func TestCompanionVideoFillsLayoutAfterNativeWindowSizeChanges(t *testing.T) {
	a := polishApp(t)
	a.companion = true
	tester := ui.NewTester(func(c *ui.Context) { a.videoView(c, ui.Rect{W: 320, H: 360}) }, 640, 360)
	r, ok := tester.Find("直播画面，双击切换全屏")
	if !ok || r.W != 640 {
		t.Fatal("小窗沿用了旧宽度，没有铺满新窗口", r)
	}
}
