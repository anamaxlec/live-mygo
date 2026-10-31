//go:build darwin || windows

package app

import (
	"fmt"
	"math"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"live-mygo/internal/library"
	"live-mygo/internal/platforms"
)

func TestSearchResultsExposeBackButton(t *testing.T) {
	rooms := []platforms.Room{{Platform: platforms.Douyu, ID: "6657", Title: "小宝宝藏结晶直播间", Anchor: "玩机器·Machine"}}
	s, _ := library.Load(filepath.Join(t.TempDir(), "library.json"))
	a := &app{
		page: "douyu", platformLabel: "斗鱼", catalogQuery: "玩机器", catalog: rooms, library: s,
		focusMode: true, navMeasured: true, navWidth: 0,
		covers: map[string]*ui.Bitmap{}, coverLoading: map[string]bool{},
	}
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
			ui.Box(c.Key("content")).Grow(1).MinWidth(0).Children(func() { a.browseView(c) })
		})
	}, 1180, 800)
	if _, ok := tester.Find("返回"); !ok {
		t.Fatal("搜索结果缺少返回按钮")
	}
	if !tester.HasText("“玩机器”") {
		t.Fatal("搜索结果标题错误")
	}
}

func TestBrowsePagesShareOneLeftEdgeWhileSidebarMoves(t *testing.T) {
	rooms := []platforms.Room{
		{Platform: platforms.Huya, ID: "1", Title: "这是第一个直播间的完整标题", Anchor: "主播一"},
		{Platform: platforms.Huya, ID: "2", Title: "这是第二个直播间的完整标题", Anchor: "主播二"},
		{Platform: platforms.Huya, ID: "3", Title: "这是第三个直播间的完整标题", Anchor: "主播三"},
	}
	for _, navWidth := range []float32{0, 20, sidebarWidth} {
		t.Run(fmt.Sprintf("nav-%v", navWidth), func(t *testing.T) {
			s, _ := library.Load(filepath.Join(t.TempDir(), "library.json"))
			s.History = []platforms.Room{{Platform: platforms.Bilibili, ID: "99", Title: "最近看过"}}
			a := &app{
				page: "huya", platformLabel: "虎牙", catalog: rooms, library: s,
				focusMode: navWidth == 0, navMeasured: true, navWidth: navWidth,
				covers: map[string]*ui.Bitmap{}, coverLoading: map[string]bool{},
			}
			tester := ui.NewTester(func(c *ui.Context) {
				ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
					if navWidth > 0 {
						ui.Box(c.Key("nav")).Width(navWidth).Shrink(0).FillHeight()
					}
					ui.Box(c.Key("content")).Grow(1).MinWidth(0).Children(func() { a.browseView(c) })
				})
			}, 1180, 800)
			inset := contentInset
			if runtime.GOOS == "windows" {
				inset = max(contentInset, 54-navWidth)
			} else {
				inset = max(contentInset, 137-navWidth)
			}
			want := navWidth + inset
			for _, label := range []string{"虎牙", "虎牙推荐", "播放 主播一 这是第一个直播间的完整标题"} {
				rect, ok := tester.Find(label)
				if !ok {
					t.Fatalf("缺少 %q", label)
				}
				if math.Abs(float64(rect.X-want)) > .5 {
					t.Fatalf("%q 左边线为 %v，应为 %v", label, rect.X, want)
				}
			}
			// 分区胶囊向左挂出内边距，使标签文字与标题落在同一条线。
			if rect, ok := tester.Find("全部直播"); !ok {
				t.Fatal("缺少分区标签")
			} else if math.Abs(float64(rect.X-(want-filterTabPad))) > .5 {
				t.Fatalf("分区背景左边线为 %v，文字应为 %v", rect.X, want)
			}
		})
	}
}

func TestBrowseSidebarRespectsSystemButtonsAndReversesSmoothly(t *testing.T) {
	for _, nativeLeft := range []float32{0, 96, 146} {
		a := polishApp(t)
		a.platformLabel = "哔哩哔哩"
		a.catalog = []platforms.Room{{Platform: platforms.Bilibili, ID: "1", Title: "侧栏动画测试", Anchor: "主播"}}
		tester := ui.NewTester(a.view, 1110, 720)
		tester.SetTitleBar(ui.TitleBar{Left: nativeLeft, Height: 58})
		button, ok := tester.Find("显示 / 隐藏导航侧栏")
		if !ok {
			t.Fatal("浏览页缺少侧栏开关")
		}
		if runtime.GOOS == "darwin" && button.X < max(100, nativeLeft+14) {
			t.Fatalf("侧栏开关进入红绿灯安全区：%+v nativeLeft=%v", button, nativeLeft)
		}
		if math.Abs(float64(button.Y+button.H/2-watchHeaderHeight/2)) > .1 {
			t.Fatal("侧栏开关没有与标题栏居中", button)
		}
		if err := tester.Click("显示 / 隐藏导航侧栏"); err != nil {
			t.Fatal(err)
		}
		tester.Advance(80 * time.Millisecond)
		if a.navWidth <= 0 || a.navWidth >= sidebarWidth {
			t.Fatal("浏览页侧栏没有连续收起过渡", a.navWidth)
		}
		if err := tester.Click("显示 / 隐藏导航侧栏"); err != nil {
			t.Fatal(err)
		}
		tester.Advance(250 * time.Millisecond)
		if math.Abs(float64(a.navWidth-sidebarWidth)) > .1 {
			t.Fatal("快速反向操作未恢复展开状态", a.navWidth)
		}
		if err := tester.Click("显示 / 隐藏导航侧栏"); err != nil {
			t.Fatal(err)
		}
		tester.Advance(250 * time.Millisecond)
		title, ok := tester.Find("哔哩哔哩")
		if !ok || a.navWidth > .1 || title.X < button.X+button.W+7-.1 {
			t.Fatalf("收起后标题被开关挡住：nav=%v title=%+v button=%+v", a.navWidth, title, button)
		}
	}
}

func TestBrowseCardsKeepColumnsThroughSidebarAnimation(t *testing.T) {
	for _, discovery := range []bool{false, true} {
		for _, width := range []int{1000, 1110, 1360} {
			var firstRow int
			for _, nav := range []float32{sidebarWidth, 120, 20, 0} {
				a := polishApp(t)
				a.navMeasured, a.navWidth = true, nav
				var rooms []platforms.Room
				for i := 0; i < 9; i++ {
					rooms = append(rooms, platforms.Room{Platform: platforms.Bilibili, ID: fmt.Sprint(i), Title: fmt.Sprint(i), Anchor: "主播"})
				}
				tester := ui.NewTester(func(c *ui.Context) {
					ui.Row(c).Fill().Children(func() {
						ui.Box(c).Width(nav).Shrink(0)
						ui.Box(c).Grow(1).MinWidth(0).Children(func() {
							if discovery {
								a.discoveryView(c, rooms)
							} else {
								a.roomGrid(c, rooms, false, false)
							}
						})
					})
				}, width, 900)
				first, _ := tester.Find("播放 主播 0")
				count := 0
				for i := range rooms {
					if r, ok := tester.Find("播放 主播 " + fmt.Sprint(i)); ok && r.Y == first.Y {
						count++
					}
				}
				if nav == sidebarWidth {
					firstRow = count
				} else if count != firstRow {
					t.Fatalf("侧栏动画导致跳列：discovery=%v width=%d nav=%v columns=%d want=%d", discovery, width, nav, count, firstRow)
				}
			}
		}
	}
}

func TestLiveCardsKeepTitleBelowWidescreenCover(t *testing.T) {
	rooms := []platforms.Room{{Platform: platforms.Bilibili, ID: "1", Title: "这是第一个直播间的完整标题", Anchor: "主播一"}, {Platform: platforms.Douyu, ID: "2", Title: "这是第二个直播间的完整标题", Anchor: "主播二"}, {Platform: platforms.Bilibili, ID: "3", Title: "这是第三个直播间的完整标题", Anchor: "主播三"}}
	for _, width := range []int{1000, 1180, 1480} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			s, _ := library.Load(filepath.Join(t.TempDir(), "library.json"))
			a := &app{page: "bilibili", library: s, covers: map[string]*ui.Bitmap{}, coverLoading: map[string]bool{}}
			tester := ui.NewTester(func(c *ui.Context) {
				ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
					ui.Box(c).Width(sidebarWidth)
					ui.Column(c).Grow(1).MinWidth(0).Children(func() { a.roomGrid(c, rooms, false, false) })
				})
			}, width, 700)
			var previous ui.Rect
			for i, r := range rooms[:2] {
				button, ok := tester.Find("播放 " + r.Anchor + " " + r.Title)
				if !ok {
					t.Fatal("缺少可访问的直播间按钮")
				}
				title, ok := tester.Find(r.Title)
				if !ok {
					t.Fatal("缺少标题")
				}
				coverHeight := title.Y - button.Y - 9
				if math.Abs(float64(button.W/coverHeight-16.0/9)) > .03 {
					t.Fatalf("封面比例错误：button=%+v title=%+v", button, title)
				}
				if title.X < button.X || title.X+title.W > button.X+button.W+1 {
					t.Fatal("标题超出卡片")
				}
				if i > 0 && (button.Y != previous.Y || button.X < previous.X+previous.W+16) {
					t.Fatalf("卡片未按统一间距对齐：%+v %+v", previous, button)
				}
				previous = button
			}
			if err := tester.Click("关注 主播一"); err != nil {
				t.Fatal(err)
			}
			if !s.Has(rooms[0]) {
				t.Fatal("点击关注没有保存")
			}
			if err := tester.Click("取消关注 主播一"); err != nil {
				t.Fatal(err)
			}
			if s.Has(rooms[0]) {
				t.Fatal("再次点击没有取消关注")
			}
		})
	}
}
