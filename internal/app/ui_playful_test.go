//go:build darwin || windows

package app

import (
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestNavigationSelectionCanReverseAndDoesNotMoveHitTargets(t *testing.T) {
	selected := 0
	tester := ui.NewTester(func(c *ui.Context) {
		c.Root().Background(ui.Hex("#ffffff"))
		ui.Column(c).Width(220).Padding(20).Gap(3).Children(func() {
			for i, label := range []string{"第一项", "第二项", "第三项"} {
				b := ui.ButtonBase(c.Key(i)).Height(35).FillWidth().Label(label)
				b.Children(func() { navigationSelection(c, "test", selected == i, colors(c.Theme())) })
				if b.Clicked() {
					selected = i
				}
			}
		})
	}, 300, 240)
	first, _ := tester.Find("第一项")
	third, _ := tester.Find("第三项")
	centroid := func() float64 {
		img := tester.Image()
		var sum, weight float64
		for y := 15; y < 145; y++ {
			v := float64(255 - img.RGBAAt(180, y).R)
			sum += float64(y) * v
			weight += v
		}
		if weight == 0 {
			t.Fatal("移动中的选中底色消失")
		}
		return sum / weight
	}
	if err := tester.Click("第三项"); err != nil {
		t.Fatal(err)
	}
	tester.Move(280, 200)
	tester.Advance(70 * time.Millisecond)
	tester.Frame()
	before := centroid()
	if before <= float64(first.Y+first.H/2) || before >= float64(third.Y+third.H/2) {
		t.Fatal("选中底色没有经过条目之间", before)
	}
	selected = 0
	tester.Frame()
	if after := centroid(); math.Abs(after-before) > .6 {
		t.Fatalf("快速反向时选中底色跳跃：%.2f -> %.2f", before, after)
	}
	if err := tester.Click("第二项"); err != nil || selected != 1 {
		t.Fatal("移动底色影响了固定的点击目标", err, selected)
	}
	tester.Move(280, 200)
	tester.Advance(time.Second)
	if tester.FramePending() {
		t.Fatal("导航已经落定，仍在请求动画帧")
	}
}

func TestFavoriteReactionDoesNotReplayOnLoadAndSurvivesRepeatedClicks(t *testing.T) {
	selected := true
	tester := ui.NewTester(func(c *ui.Context) {
		c.Root().Center().Background(ui.Hex("#ffffff"))
		icon := iconHeart
		if selected {
			icon = iconHeartFilled
		}
		if iconButton(c.Key("follow"), icon, "关注按钮").Clicked() {
			selected = !selected
		}
	}, 160, 120)
	if tester.FramePending() {
		t.Fatal("读取已有关注时不应该播放庆祝动画")
	}
	button, _ := tester.Find("关注按钮")
	for range 6 {
		if err := tester.Click("关注按钮"); err != nil {
			t.Fatal(err)
		}
		tester.Advance(70 * time.Millisecond)
		if current, _ := tester.Find("关注按钮"); current != button {
			t.Fatal("关注动画改变了按钮点击区域", current, button)
		}
	}
	if !selected {
		t.Fatal("快速连点丢失了关注状态")
	}
	tester.Move(0, 0)
	tester.Advance(time.Second)
	if tester.FramePending() {
		t.Fatal("关注反馈结束后仍在请求动画帧")
	}
}

func TestMascotSettlesAndReducedMotionKeepsItStill(t *testing.T) {
	for _, reduced := range []bool{false, true} {
		tester := ui.NewTester(func(c *ui.Context) {
			c.Root().Center().Background(ui.Hex("#ffffff"))
			playfulMascot(c)
		}, 180, 160)
		tester.SetPreferences(ui.Preferences{ReduceMotion: reduced, TextScale: 1})
		tester.Advance(time.Second)
		bounds, _ := tester.Find("和高松灯打个招呼")
		colored := 0
		for y := int(bounds.Y); y < int(bounds.Y+bounds.H); y++ {
			for x := int(bounds.X); x < int(bounds.X+bounds.W); x++ {
				pixel := tester.Image().RGBAAt(x, y)
				if pixel.R < 190 && pixel.G < 190 && pixel.B < 205 {
					colored++
				}
			}
		}
		if colored < 100 {
			t.Fatal("角色 SVG 图层没有显示", colored)
		}
		if err := tester.Click("和高松灯打个招呼"); err != nil {
			t.Fatal(err)
		}
		tester.Move(0, 0)
		tester.Advance(210 * time.Millisecond)
		if current, _ := tester.Find("和高松灯打个招呼"); current != bounds {
			t.Fatal("角色动作移动了点击区域")
		}
		if reduced && tester.FramePending() {
			t.Fatal("减少动态效果开启后仍在蹦跳")
		}
		tester.Advance(time.Second)
		if tester.FramePending() {
			t.Fatal("角色在没有操作时继续请求动画帧")
		}
	}
}
