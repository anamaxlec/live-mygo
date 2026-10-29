// 为拖拽安装窗口生成 1x / 2x 背景，图标本身由 Finder 绘制。
package main

import (
	"image/png"
	"os"
	"path/filepath"

	"github.com/egoist/mygo/ui"
)

func main() {
	output := "build/installer"
	if err := os.MkdirAll(output, 0755); err != nil {
		panic(err)
	}
	arrow := ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 44 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M3 12h36M30 3l9 9-9 9"/></svg>`))
	for _, scale := range []float32{1, 2} {
		img := ui.Render(func(c *ui.Context) {
			c.Root().Background(ui.Hex("#FFFFFF"))
			ui.Row(c).Absolute().Left(0).Right(0).Top(35).Height(32).Justify(ui.Center).Children(func() {
				ui.Text(c, "Live MyGo").FontSize(23).FontWeight(600).TextColor(ui.Hex("#292929"))
			})
			ui.Icon(c, arrow).Absolute().Left(298).Top(178).Size(44, 24).TextColor(ui.Hex("#B0B0B0"))
			ui.Row(c).Absolute().Left(0).Right(0).Top(310).Height(24).Justify(ui.Center).Children(func() {
				ui.Text(c, "拖入 Applications").FontSize(13).TextColor(ui.Hex("#808080"))
			})
		}, 640, 380, scale)
		name := "background.png"
		if scale == 2 {
			name = "background@2x.png"
		}
		f, err := os.Create(filepath.Join(output, name))
		if err != nil {
			panic(err)
		}
		if err = png.Encode(f, img); err != nil {
			f.Close()
			panic(err)
		}
		if err = f.Close(); err != nil {
			panic(err)
		}
	}
}
