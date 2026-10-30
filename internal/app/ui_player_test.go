//go:build darwin || windows

package app

import (
	"fmt"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"live-mygo/internal/library"
	"live-mygo/internal/platforms"
	"live-mygo/internal/player"
)

func TestWatchHeaderReservesRightSystemButtons(t *testing.T) {
	a := polishApp(t)
	a.page, a.focusMode = "player", true
	tester := ui.NewTester(a.view, 1000, 580)
	tester.SetTitleBar(ui.TitleBar{Right: 138, Height: 58})
	for _, label := range []string{shortcutLabel("快捷搜索直播 · ⌘L"), "直播间选项", shortcutLabel("设置 · ⌘,")} {
		r, ok := tester.Find(label)
		if !ok || r.X+r.W > 1000-138-12+.1 {
			t.Fatalf("%s 进入右侧系统按钮区：%+v", label, r)
		}
	}
}

func TestWatchTitleAlignsWithVideoAndNavigationStaysToItsLeft(t *testing.T) {
	for _, width := range []int{1000, 1180, 1480} {
		for _, metadata := range []bool{false, true} {
			a := polishApp(t)
			a.page, a.focusMode, a.title = "player", false, "对齐检查标题"
			a.targetRoom = platforms.Room{}
			if metadata {
				a.targetRoom = platforms.Room{Platform: platforms.Qie, Anchor: "测试主播"}
			}
			tester := ui.NewTester(a.view, width, 720)
			tester.SetTitleBar(ui.TitleBar{Left: 96, Height: 58})
			title, ok := tester.Find(a.title)
			video, _ := tester.Find("直播画面，双击切换全屏")
			if !ok || math.Abs(float64(title.X-video.X)) > .01 {
				t.Fatalf("标题未与播放器左边线对齐：width=%d metadata=%v title=%+v video=%+v", width, metadata, title, video)
			}
			top, bottom := title.Y, title.Y+title.H
			if metadata {
				meta, _ := tester.Find("企鹅体育 · 测试主播")
				bottom = meta.Y + meta.H
			}
			for _, label := range []string{"返回列表 · Esc", "显示 / 隐藏导航侧栏"} {
				button, ok := tester.Find(label)
				if !ok || button.X < 110 || button.X+button.W > title.X-8 {
					t.Fatalf("导航按钮未放在标题左侧或侵犯系统按钮区域：%s %+v title=%+v", label, button, title)
				}
				if math.Abs(float64((top+bottom)/2-button.Y-button.H/2)) > .5 {
					t.Fatalf("标题整体与导航按钮未垂直居中：metadata=%v title=%+v button=%+v", metadata, title, button)
				}
			}
		}
	}
}

func TestWatchLayoutKeepsControlsInsideVideoAndChatOnRight(t *testing.T) {
	for _, width := range []int{1000, 1180, 1480} {
		for _, sidebar := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/sidebar=%v", width, sidebar), func(t *testing.T) {
				state, _ := library.Load(filepath.Join(t.TempDir(), "library.json"))
				room := platforms.Room{Platform: platforms.Bilibili, ID: "6", Title: "直播间标题", Anchor: "测试主播"}
				a := &app{library: state, page: "player", focusMode: !sidebar, title: room.Title, targetRoom: room, shown: true, stats: player.Stats{Paused: true}, controlsUntil: time.Now().Add(time.Hour), chatList: ui.ListState{FollowEnd: true}, chatStatus: "已连接", stream: &platforms.Stream{Room: room, QualityID: 250, Qualities: []platforms.Quality{{ID: 250, Name: "超清"}}, Sources: []platforms.Source{{ID: "one", Name: "线路 1", Format: "FLV"}, {ID: "two", Name: "线路 2", Format: "HLS"}}, SourceID: "one"}, qualityLabel: "超清", chatMessages: []platforms.ChatMessage{{ID: 1, User: "观众", Text: "较长的中文消息应该自然换行，不挤占右侧边界，也不会横向撑开列表。"}}}
				tester := ui.NewTester(a.view, width, 720)
				tester.SetTitleBar(ui.TitleBar{Left: 96, Height: 58})
				back, _ := tester.Find("返回列表 · Esc")
				if !sidebar && back.X < 108 {
					t.Fatalf("按钮进入系统控件占位：%+v", back)
				}
				if _, ok := tester.Find("前往官网发送弹幕"); ok {
					t.Fatal("已删除的底栏仍存在")
				}
				video, ok := tester.Find("直播画面，双击切换全屏")
				if !ok {
					t.Fatal("缺少视频区域")
				}
				for _, label := range []string{"继续 · Space", "切换清晰度", "切换播放线路", "收起弹幕列表", "工作陪伴小窗", "睡眠定时", shortcutLabel("全屏 · ⌃⌘F")} {
					r, ok := tester.Find(label)
					if !ok || r.X < video.X || r.Y < video.Y || r.X+r.W > video.X+video.W+1 || r.Y+r.H > video.Y+video.H+1 {
						t.Fatalf("控件 %q 未嵌入画面：video=%+v control=%+v found=%v", label, video, r, ok)
					}
				}
				chat, ok := tester.Find("弹幕列表")
				if !ok || math.Abs(float64(chat.X-video.X-video.W)) > .01 || chat.Y != video.Y || chat.H != video.H {
					t.Fatalf("弹幕列表和视频未共享顶边、底边：video=%+v chat=%+v", video, chat)
				}
				if err := tester.Click("切换播放线路"); err != nil {
					t.Fatal(err)
				}
				if _, ok := tester.Find("线路 2"); !ok {
					t.Fatal("没有显示真实备用线路")
				}
				a.sourceOpen = false
				tester.Frame()
				if err := tester.Click("弹幕列表设置"); err != nil {
					t.Fatal(err)
				}
				if _, ok := tester.Find("过滤重复消息"); !ok {
					t.Fatal("缺少弹幕过滤选项")
				}
			})
		}
	}
}

func TestWindowFollowingSubtractsChatAndHeader(t *testing.T) {
	for _, tc := range []struct {
		w, h, side  int
		aspect      float64
		preferWidth bool
	}{{1180, 780, 286, 16.0 / 9, true}, {1180, 780, 0, 16.0 / 9, true}, {1100, 720, 519, 16.0 / 9, false}, {1180, 780, 286, 9.0 / 16, true}} {
		w, h := fittedWindow(tc.w, tc.h, tc.aspect, tc.side, 58, 1496, 900, tc.preferWidth)
		if w > 1496 || h > 900 {
			t.Fatalf("窗口超出屏幕：%dx%d", w, h)
		}
		actual := float64(w-tc.side) / float64(h-58)
		if math.Abs(actual-tc.aspect) > .006 {
			t.Fatalf("视频比例错误：%+v -> %dx%d ratio=%f", tc, w, h, actual)
		}
	}
}

func TestEmbeddedControlsHideAfterInactivity(t *testing.T) {
	state, _ := library.Load(filepath.Join(t.TempDir(), "library.json"))
	a := &app{library: state, page: "player", focusMode: true, shown: true, title: "测试直播", controlsUntil: time.Now().Add(time.Second)}
	tester := ui.NewTester(a.view, 1180, 780)
	if _, ok := tester.Find("暂停 · Space"); !ok {
		t.Fatal("初始控制条不可见")
	}
	tester.Advance(1500 * time.Millisecond)
	if _, ok := tester.Find("暂停 · Space"); ok {
		t.Fatal("无操作后控制条仍可见")
	}
	a.qualityOpen = true
	tester.Frame()
	tester.Advance(200 * time.Millisecond)
	if _, ok := tester.Find("暂停 · Space"); !ok {
		t.Fatal("菜单打开时控制条不应消失")
	}
}

func TestSpaceOnVideoPausesInsteadOfOnlyActivatingContainer(t *testing.T) {
	state, _ := library.Load(filepath.Join(t.TempDir(), "library.json"))
	a := &app{library: state, page: "player", focusMode: true, shown: true, title: "测试直播", controlsUntil: time.Now().Add(time.Hour), player: &player.Player{}, playerOps: make(chan func(*player.Player), 2)}
	tester := ui.NewTester(a.view, 1180, 780)
	if err := tester.Click("直播画面，双击切换全屏"); err != nil {
		t.Fatal(err)
	}
	tester.Key(0, ui.KeySpace)
	if !a.stats.Paused || len(a.playerOps) != 1 {
		t.Fatalf("Space 没有准确触发一次暂停：paused=%v commands=%d", a.stats.Paused, len(a.playerOps))
	}
	tester.Key(0, ui.KeySpace)
	if a.stats.Paused || len(a.playerOps) != 2 {
		t.Fatal("第二次 Space 没有恢复播放")
	}
}

func TestQualityMenuListsEveryReturnedQuality(t *testing.T) {
	state, _ := library.Load(filepath.Join(t.TempDir(), "library.json"))
	room := platforms.Room{Platform: platforms.Douyu, ID: "182102", Title: "直播间标题", Anchor: "测试主播"}
	qualities := []platforms.Quality{{ID: 0, Name: "原画1080P30"}, {ID: 4, Name: "蓝光4M"}, {ID: 3, Name: "超清"}, {ID: 2, Name: "高清"}}
	a := &app{library: state, page: "player", title: room.Title, targetRoom: room, shown: true, stats: player.Stats{Paused: true}, controlsUntil: time.Now().Add(time.Hour), stream: &platforms.Stream{Room: room, QualityID: 4, Qualities: qualities, Sources: []platforms.Source{{ID: "line", Name: "线路 1", Format: "FLV"}}, SourceID: "line"}, qualityLabel: "蓝光4M"}
	tester := ui.NewTester(a.view, 1180, 720)
	if err := tester.Click("切换清晰度"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	for _, quality := range qualities {
		if _, ok := tester.Find(quality.Name); !ok {
			t.Fatalf("清晰度菜单缺少平台返回的档位 %q", quality.Name)
		}
	}
}
