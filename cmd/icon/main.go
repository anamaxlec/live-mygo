// 从可编辑的 SVG 源生成 macOS 图标的各档尺寸。
package main

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	xdraw "golang.org/x/image/draw"
)

func save(path string, img image.Image) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		panic(err)
	}
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err = png.Encode(f, img); err != nil {
		panic(err)
	}
}

func main() {
	data, err := os.ReadFile("assets/app-icon.svg")
	if err != nil {
		panic(err)
	}
	source := ui.MustParseSVG(data)
	preview := ui.NewTester(func(c *ui.Context) {
		c.Root().Background(ui.Transparent)
		ui.Image(c, source).Fill().Fit(ui.Contain)
	}, 1024, 1024)
	img := preview.Image()
	save("assets/app-icon.png", img)
	darkSource := strings.NewReplacer("#F1F4FC", "#252A38", "#D5DCEB", "#414B63").Replace(string(data))
	if err = os.WriteFile("assets/app-icon-dark.svg", []byte(darkSource), 0644); err != nil {
		panic(err)
	}
	darkPreview := ui.NewTester(func(c *ui.Context) {
		c.Root().Background(ui.Transparent)
		ui.Image(c, ui.MustParseSVG([]byte(darkSource))).Fill().Fit(ui.Contain)
	}, 1024, 1024)
	save("assets/app-icon-dark.png", darkPreview.Image())
	for _, size := range []int{16, 32, 128, 256, 512} {
		for _, scale := range []int{1, 2} {
			n := size * scale
			small := image.NewRGBA(image.Rect(0, 0, n, n))
			xdraw.CatmullRom.Scale(small, small.Bounds(), img, img.Bounds(), draw.Src, nil)
			suffix := ""
			if scale == 2 {
				suffix = "@2x"
			}
			save(fmt.Sprintf("build/AppIcon.iconset/icon_%dx%d%s.png", size, size, suffix), small)
		}
	}
}
