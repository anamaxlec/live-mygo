//go:build darwin || windows

package app

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestWatchFrameEnclosesVideoAndChatWithEqualInsets(t *testing.T) {
	for _, width := range []int{1000, 1180, 1480} {
		for _, dark := range []bool{false, true} {
			a := polishApp(t)
			a.page, a.focusMode = "player", true
			tester := ui.NewTester(a.view, width, 720)
			tester.SetDark(dark)
			frame, ok := tester.Find("直播间内容容器")
			if !ok || frame.X != watchFrameInset || frame.Y != watchHeaderHeight+watchFrameInset || frame.X+frame.W != float32(width)-watchFrameInset || frame.Y+frame.H != 720-watchFrameInset {
				t.Fatalf("容器四周留白不一致：width=%d frame=%+v", width, frame)
			}
			video, _ := tester.Find("直播画面，双击切换全屏")
			if video.X != frame.X+watchFrameBorder || video.Y+video.H != frame.Y+frame.H-watchFrameBorder {
				t.Fatalf("视频没有放入容器：%+v %+v", frame, video)
			}
			img := tester.Image()
			bottom := img.RGBAAt(width/2, 719)
			left := img.RGBAAt(2, 400)
			header := img.RGBAAt(width/2, 24)
			// 取消阴影后，四周底板保持标题栏的颜色。
			if bottom.A != 255 || bottom.R != header.R || left.R != header.R {
				t.Fatal("底板未沿用标题栏颜色", bottom, left, header)
			}
		}
	}
}

func TestWatchFrameDoesNotCastShadowAcrossVideoAndComments(t *testing.T) {
	a := polishApp(t)
	a.page, a.focusMode, a.shown = "player", true, true
	tester := ui.NewTester(a.view, 1180, 780)
	tester.SetVibrancy(true)
	frame, _ := tester.Find("直播间内容容器")
	img := tester.Image()
	// 取边线外侧 1 DIP；移除阴影后这里必须保持透明，露出系统底板。
	video := img.RGBAAt(400, int(frame.Y)-1)
	chat := img.RGBAAt(1000, int(frame.Y)-1)
	if video.A != 0 || chat.A != 0 {
		t.Fatal("用户关闭阴影后，内容容器外仍存在阴影", video, chat)
	}
}

func TestWatchFrameGeometryMatchesAspectFollowingAndFullscreen(t *testing.T) {
	for _, sidebar := range []float32{0, sidebarWidth} {
		for _, chat := range []float32{0, chatPanelWidth} {
			for _, aspect := range []float64{16.0 / 9, 9.0 / 16} {
				side := int(sidebar + chat + 2*(watchFrameInset+watchFrameBorder))
				header := int(watchHeaderHeight + 2*(watchFrameInset+watchFrameBorder))
				w, h := fittedWindow(1180, 780, aspect, side, header, 1496, 900, true)
				layout := watchGeometry(float32(w), float32(h), watchHeaderHeight, sidebar, chat)
				if math.Abs(float64(layout.Video.W/layout.Video.H)-aspect) > .006 {
					t.Fatalf("新增容器占位未纳入比例：%+v aspect=%v", layout, aspect)
				}
			}
		}
	}
	full := watchGeometry(1440, 900, 0, 0, 0)
	if full.Frame != (ui.Rect{W: 1440, H: 900}) || full.Video != full.Frame {
		t.Fatal("全屏仍保留窗口边框占位", full)
	}
}

func TestWatchFrameCornersDoNotBlurIntoVideo(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		img := ui.Render(func(c *ui.Context) {
			c.Root().Background(ui.Hex("#000000"))
			ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) { paintWatchFrame(p, r, colors(ui.LightTheme()), watchFrameRadius) })
		}, 100, 100, scale)
		for _, x := range []int{6, 93} {
			for _, y := range []int{6, 93} {
				if pixel := img.RGBAAt(int(float32(x)*scale), int(float32(y)*scale)); pixel.R > 4 || pixel.G > 4 || pixel.B > 4 {
					t.Fatal("圆角遮罩模糊到了画面内", scale, x, y, pixel)
				}
			}
		}
	}
}

func TestWatchChromeRevealsSystemMaterialWhileChatStaysOpaque(t *testing.T) {
	a := polishApp(t)
	a.page, a.focusMode = "player", true
	tester := ui.NewTester(a.view, 1180, 780)
	tester.SetVibrancy(true)
	img := tester.Image()
	if header := img.RGBAAt(590, 24); header.A != 0 {
		t.Fatal("标题栏遮住了系统磨砂材质", header)
	}
	if chat := img.RGBAAt(1000, 600); chat.A != 255 || chat.R != 255 || chat.G != 255 || chat.B != 255 {
		t.Fatal("评论区没有保持不透明纯白", chat)
	}
	frame, _ := tester.Find("直播间内容容器")
	for _, y := range []int{int(frame.Y + 1), int(frame.Y + frame.H - 2)} {
		if corner := img.RGBAAt(int(frame.X+frame.W-2), y); corner.A > 32 {
			t.Fatal("评论区圆角外侧有不透明直角露边", corner, y)
		}
	}
}

func TestChatBackgroundRoundsWithoutOuterWatchClip(t *testing.T) {
	for _, legacy := range []string{`{}`, `{"chat_follow":true,"chat_official":true}`} {
		a := polishApp(t)
		if err := json.Unmarshal([]byte(legacy), &a.library.Preferences); err != nil {
			t.Fatal(err)
		}
		tester := ui.NewTester(func(c *ui.Context) {
			c.Root().Background(ui.Transparent)
			a.chatView(c)
		}, 286, 700)
		img := tester.Image()
		for _, y := range []int{0, 699} {
			if pixel := img.RGBAAt(285, y); pixel.A > 16 {
				t.Fatalf("评论白底依赖外层才能收圆角：legacy=%v y=%d color=%v", legacy, y, pixel)
			}
		}
		if pixel := img.RGBAAt(270, 350); pixel.A != 255 || pixel.R != 255 {
			t.Fatal("圆角处理改变了评论区内部白底", pixel)
		}
	}
}
