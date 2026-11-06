//go:build darwin || windows

package app

import (
	"image"
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"live-mygo/internal/platforms"
)

func TestReturnToCardBendsTowardTheActualCard(t *testing.T) {
	card := ui.Rect{X: 23, Y: 163, W: 91, H: 53}
	var start time.Time
	tester := ui.NewTester(func(c *ui.Context) {
		if start.IsZero() {
			start = c.Now()
		}
		c.Root().Background(ui.RGB(255, 255, 255))
		fx := returnToCardEffect{start: start, duration: returnToCardDuration, card: &card}
		ui.Overlay(c, func() {
			fx.shadowView(c)
			ui.Box(c.Key("test-sheet")).Absolute().Fill().Background(ui.RGB(180, 30, 100)).Effect(fx)
		})
	}, 360, 240)
	tester.Advance(returnToCardDuration * 2 / 5)
	tester.Frame()
	img := tester.Image()
	inkAt := func(y int) (left, width int) {
		left = img.Bounds().Dx()
		for x := range img.Bounds().Dx() {
			px := img.RGBAAt(x, y)
			if int(px.R)-int(px.G) > 12 {
				left = min(left, x)
				width++
			}
		}
		return left, width
	}
	upperLeft, upper := inkAt(104)
	lowerLeft, lower := inkAt(208)
	if lower < 90 || upper <= lower+35 {
		t.Fatalf("靠近卡片的一端没有先变窄，顶部宽 %d，底部宽 %d", upper, lower)
	}
	const middleY = 155
	middleLeft, _ := inkAt(middleY)
	if middleLeft != upperLeft || middleLeft != lowerLeft || middleLeft <= 0 || middleLeft >= int(card.X) {
		t.Fatalf("左沿移动时没有保持笔直，左沿坐标为 %d、%d、%d", upperLeft, middleLeft, lowerLeft)
	}
	near := img.RGBAAt(middleLeft-5, middleY)
	far := img.RGBAAt(max(0, middleLeft-26), middleY)
	if near.R != near.G || near.R >= 246 || int(far.R)-int(near.R) < 8 {
		t.Fatalf("左沿外没有柔和的投影，近侧 %v，远侧 %v", near, far)
	}
	tester.Advance(returnToCardDuration * 55 / 100)
	tester.Frame()
	img = tester.Image()
	_, above := inkAt(162)
	_, below := inkAt(217)
	left, width := inkAt(180)
	if above != 0 || below != 0 || left != 23 || width != 91 || img.RGBAAt(22, 180).G != 255 {
		t.Fatal("交接前没有贴齐卡片的四条边或仍留下投影")
	}
}

func TestReturnUsesSelectedMotionAcrossPlaybackStates(t *testing.T) {
	states := []struct {
		name                         string
		resolved, shown, busy, audio bool
		retryDelay, awaitingFrame    bool
	}{
		{name: "解析房间", busy: true},
		{name: "连接直播流", resolved: true, busy: true},
		{name: "等待首帧", resolved: true, shown: true, awaitingFrame: true},
		{name: "重连倒计时", resolved: true, busy: true, retryDelay: true},
		{name: "连接失败", resolved: true},
		{name: "抓帧不可用", resolved: true, shown: true},
		{name: "只听声音", resolved: true, shown: true, audio: true},
	}
	for _, state := range states {
		for _, classic := range []bool{false, true} {
			mode := "收拢到卡片"
			if classic {
				mode = "展开侧栏"
			}
			t.Run(state.name+"/"+mode, func(t *testing.T) {
				a := polishApp(t)
				a.page, a.returnPage, a.focusMode = "player", "history", true
				a.targetRoom = platforms.Room{Platform: platforms.Douyu, ID: "1", Title: "直播"}
				a.title = a.targetRoom.Title
				if state.resolved {
					a.stream = &platforms.Stream{Room: a.targetRoom}
				}
				a.shown, a.busy = state.shown, state.busy
				if state.awaitingFrame {
					a.playStarted = time.Now()
				}
				a.library.Preferences.AudioOnly = state.audio
				a.library.Preferences.ClassicReturn = classic
				cancelled := false
				if a.busy {
					a.resolveCancel = func() { cancelled = true }
				}
				if state.retryDelay {
					a.reconnectTimer = time.NewTimer(time.Hour)
					defer a.reconnectTimer.Stop()
				}
				view := ui.NewTester(a.view, 1000, 580)
				if err := view.Click("返回列表 · Esc"); err != nil {
					t.Fatal(err)
				}
				view.Frame()
				view.Advance(90 * time.Millisecond)
				if a.page != "history" || a.busy || state.busy && !cancelled || a.reconnectTimer != nil {
					t.Fatalf("返回后仍有房间请求或重连计时：page=%s busy=%v cancelled=%v", a.page, a.busy, cancelled)
				}
				if classic {
					if a.returnShrink || a.navWidth <= 0 || a.navWidth >= sidebarWidth {
						t.Fatalf("侧栏未展开，当前宽度 %.1f", a.navWidth)
					}
				} else if !a.returnShrink || a.nowCardBounds.W < 100 {
					t.Fatal("丢失收拢动画或没有目标卡片")
				}
				view.Advance(returnCardCatchEnd)
				if _, found := view.Find("返回正在播放的直播间"); !found || a.returnShrink {
					t.Fatal("动画后没有留下可打开的卡片")
				}
			})
		}
	}
}

func TestReturnWhileCardIsOpeningOrReconnecting(t *testing.T) {
	cases := []struct {
		name    string
		elapsed time.Duration
		escape  bool
	}{
		{"展开初段点击", cardOpenDuration / 4, false},
		{"展开中段点击", cardOpenDuration / 2, false},
		{"展开尾段点击", cardOpenDuration * 3 / 4, false},
		{"展开中按 Esc", cardOpenDuration / 2, true},
		{"展开后仍在重连", cardOpenDuration + 20*time.Millisecond, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := polishApp(t)
			a.page = "history"
			a.targetRoom = platforms.Room{Platform: platforms.Douyu, ID: "1", Title: "测试房间"}
			a.stream = &platforms.Stream{Room: a.targetRoom}
			a.title = a.targetRoom.Title
			frame := ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 16, 9)))
			a.returnFrame = frame
			view := ui.NewTester(a.view, 1000, 580)
			if err := view.Click("返回正在播放的直播间"); err != nil {
				t.Fatal(err)
			}
			cancelled := false
			a.busy = true
			a.resolveCancel = func() { cancelled = true }
			midFlight := tc.elapsed < cardOpenDuration
			view.Advance(tc.elapsed)
			if tc.escape {
				view.Key(0, ui.KeyEscape)
			} else if err := view.Click("返回列表 · Esc"); err != nil {
				t.Fatal(err)
			}
			view.Frame()
			if a.page != "history" || !a.returnShrink || a.returnFromOpen != midFlight || !cancelled || a.returnFrame != frame {
				t.Fatalf("返回丢失状态：page=%s shrink=%v reversed=%v cancelled=%v frame=%v", a.page, a.returnShrink, a.returnFromOpen, cancelled, a.returnFrame != nil)
			}
			if midFlight {
				phase := float64(view.Now().Sub(a.returnShrinkAt)) / float64(returnToCardDuration)
				expected := 1 - float64(tc.elapsed)/float64(cardOpenDuration)
				if math.Abs(phase-expected) > .05 {
					t.Fatalf("反向时从错误的轮廓开始：%.3f，预期 %.3f", phase, expected)
				}
			}
			view.Advance(returnCardCatchEnd)
			if err := view.Click("返回正在播放的直播间"); err != nil {
				t.Fatal(err)
			}
			view.Advance(cardOpenDuration + time.Millisecond)
			if a.page != "player" || a.returnShrink {
				t.Fatal("快速返回后卡片无法重新展开")
			}
		})
	}
}

func TestReturnCaptureRespectsSameRoomReconnectAndExplicitNavigation(t *testing.T) {
	for _, navigateAway := range []bool{false, true} {
		a := polishApp(t)
		a.page, a.returnPage = "player", "history"
		a.targetRoom = platforms.Room{Platform: platforms.Douyu, ID: "1"}
		timer := time.NewTimer(time.Hour)
		defer timer.Stop()
		a.returnCapture = timer
		if navigateAway {
			a.openPage("favorites")
		} else {
			a.generation++
		}
		frame := ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 16, 9)))
		a.finishReturnCapture(timer, 0, frame)
		wantPage := "history"
		if navigateAway {
			wantPage = "favorites"
		}
		if a.page != wantPage || a.returnShrink == navigateAway || a.returnCapture != nil || a.returnFrame != nil {
			t.Fatalf("迟到的抓帧结果改变了返回或导航：page=%s shrink=%v", a.page, a.returnShrink)
		}
	}
}
