//go:build darwin

package main

import (
	"github.com/egoist/mygo/ui"
	"live-mygo/internal/library"
	"math"
	"path/filepath"
	"testing"
	"time"
)

// 固定文字、轨道与帧进度，仅比较 CPU 绘制路径；不表示真实 GPU 或电池功耗。
func BenchmarkCaptionAnimation(b *testing.B) {
	state, _ := library.Load(filepath.Join(b.TempDir(), "library.json"))
	prefs := state.Preferences
	text := "一条用于比较弹幕描边绘制开销的固定消息 Aa"
	for _, cached := range []bool{false, true} {
		name := "outline"
		if cached {
			name = "sprite"
		}
		b.Run(name, func(b *testing.B) {
			sprite := renderCaptionSprite(text, 700, 45, ui.Hex("#ffffff"), prefs)
			start := time.Now()
			tester := ui.NewTester(func(c *ui.Context) {
				c.Root().Background(ui.Transparent)
				ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
					for i := range 24 {
						x := float32(math.Mod(p.Now().Sub(start).Seconds()*160+float64(i)*63, 1800)) - 700
						y := float32(i%6) * 54
						if cached {
							p.Image(sprite, ui.Rect{X: x - 4, Y: y - 4, W: 708, H: 53}, ui.Contain)
						} else {
							paintCaption(p, x, y, text, ui.Hex("#ffffff"), prefs)
						}
					}
					p.AnimationFrameRate(60)
				})
			}, 1280, 720)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				tester.Advance(time.Second / 60)
			}
		})
	}
}
