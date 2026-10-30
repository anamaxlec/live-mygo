//go:build darwin || windows

package app

import (
	"fmt"
	"math"
	"path/filepath"
	"runtime"
	"testing"

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
