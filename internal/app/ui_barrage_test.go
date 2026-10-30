//go:build darwin

package app

import (
	"image"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"live-mygo/internal/library"
	"live-mygo/internal/platforms"
)

func paintedLeft(img *image.RGBA) int {
	left := img.Bounds().Max.X
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < left; x++ {
			if img.RGBAAt(x, y).A > 40 {
				left = x
			}
		}
	}
	return left
}
func TestBarrageMovesWithoutVideoFramesAndStopsWhenEmpty(t *testing.T) {
	state, _ := library.Load(filepath.Join(t.TempDir(), "library.json"))
	a := &app{library: state, page: "player", shown: true, pendingCaptions: []platforms.ChatMessage{{ID: 1, Text: "平滑弹幕 Aa", Color: 0xffffff}}}
	tester := ui.NewTester(func(c *ui.Context) { c.Root().Background(ui.Transparent); a.barrageView(c, 1280, 720) }, 1280, 720)
	tester.Advance(400 * time.Millisecond)
	first := paintedLeft(tester.Image())
	tester.Advance(100 * time.Millisecond)
	second := paintedLeft(tester.Image())
	if first >= 1280 || math.Abs(float64(first-second-16)) > 2 {
		t.Fatalf("独立绘制没有按时间平滑移动：%d -> %d", first, second)
	}
	tester.Advance(12 * time.Second)
	if tester.FramePending() {
		t.Fatal("没有可见弹幕后仍请求动画帧")
	}
}
func TestCaptionSettingsExposeFontWeightStrokeAndLayout(t *testing.T) {
	state, _ := library.Load(filepath.Join(t.TempDir(), "library.json"))
	a := &app{library: state, page: "player", focusMode: true, shown: true, title: "测试直播", controlsUntil: time.Now().Add(time.Hour), danmakuOpen: true}
	tester := ui.NewTester(a.view, 1180, 780)
	for _, label := range []string{"字体", "字重", "描边", "文字颜色"} {
		if _, ok := tester.Find(label); !ok {
			t.Fatalf("缺少字体设置 %s", label)
		}
	}
	if err := tester.Click("布局"); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"流畅度", "滚动速度", "显示区域", "轨道行数"} {
		if _, ok := tester.Find(label); !ok {
			t.Fatalf("缺少布局设置 %s", label)
		}
	}
	a.danmakuOpen = false
	tester.Frame()
	if err := tester.Click("关闭画面弹幕 · D"); err != nil {
		t.Fatal(err)
	}
	if state.Preferences.Danmaku {
		t.Fatal("开关未更新偏好")
	}
	if _, ok := tester.Find("开启画面弹幕 · D"); !ok {
		t.Fatal("开关没有更新图标语义")
	}
}

func TestCaptionStyleChangesKeepLiveMessagesAndTheirProgress(t *testing.T) {
	a := polishApp(t)
	a.page, a.shown = "player", true
	a.pendingCaptions = []platforms.ChatMessage{{ID: 42, Text: "字体调整仍在滚动 Aa", Color: 0xffffff}}
	tester := ui.NewTester(func(c *ui.Context) {
		c.Root().Background(ui.Transparent)
		a.barrageView(c, 1280, 720)
	}, 1280, 720)
	tester.Advance(500 * time.Millisecond)
	before := a.captions[0]
	for _, change := range []func(*library.Preferences){
		func(p *library.Preferences) { p.DanmakuSize = 32 },
		func(p *library.Preferences) { p.DanmakuFont = "Songti SC" },
		func(p *library.Preferences) { p.DanmakuWeight = 700 },
		func(p *library.Preferences) { p.DanmakuStroke = 2 },
		func(p *library.Preferences) { p.DanmakuOpacity = 60 },
		func(p *library.Preferences) { p.DanmakuColor = "粉色" },
	} {
		next := a.library.Preferences
		change(&next)
		a.previewPreferences(next)
		tester.Frame()
		if len(a.captions) != 1 || a.captions[0].ID != before.ID || a.captions[0].Started != before.Started || a.captions[0].Lane != before.Lane {
			t.Fatal("样式调整清空或重置了当前弹幕")
		}
		if paintedLeft(tester.Image()) >= 1280 {
			t.Fatal("样式更新时出现空白帧")
		}
		if a.captions[0].NextSprite != nil {
			t.Fatal("连续预览时不应逐帧排队生成字图")
		}
	}
	a.savePreferences(a.library.Preferences)
	tester.Frame()
	sprite := a.captions[0].Sprite
	if a.captions[0].NextSprite != nil {
		sprite = a.captions[0].NextSprite
	}
	if sprite == nil || sprite.style != spriteStyle(a.library.Preferences) || sprite.bitmap.Load() == nil {
		t.Fatal("最终样式未恢复字图复用")
	}
	tester.Advance(100 * time.Millisecond)
	if a.captions[0].Started != before.Started || paintedLeft(tester.Image()) >= 1280 {
		t.Fatal("字图就绪后丢失弹幕或重置滚动进度")
	}
}
