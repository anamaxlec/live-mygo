//go:build darwin

package app

import (
	"fmt"
	"math"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"live-mygo/internal/library"
	"live-mygo/internal/platforms"
)

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
