// 从可编辑的 SVG 源生成 macOS 图标的各档尺寸。
package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	xdraw "golang.org/x/image/draw"
)

// 应用图标与交互角色共用一份 SVG 原稿，分层文件由打包过程生成。
func mascotLayer(data []byte, id string) []byte {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var out bytes.Buffer
	out.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="1024" height="1024" viewBox="0 0 1024 1024">`)
	encoder := xml.NewEncoder(&out)
	encoder.Indent("", "  ")
	depth := 0
	found := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(err)
		}
		if start, ok := token.(xml.StartElement); ok {
			if depth == 0 {
				keep := start.Name.Local == "defs"
				for _, attr := range start.Attr {
					if attr.Name.Local == "id" && attr.Value == id {
						keep, found = true, true
					}
				}
				if !keep {
					continue
				}
			}
			depth++
		}
		if depth == 0 {
			continue
		}
		switch element := token.(type) {
		case xml.StartElement:
			element.Name.Space = ""
			for i := range element.Attr {
				element.Attr[i].Name.Space = ""
			}
			token = element
		case xml.EndElement:
			element.Name.Space = ""
			token = element
		case xml.CharData:
			if strings.TrimSpace(string(element)) == "" {
				continue
			}
		}
		if err := encoder.EncodeToken(token); err != nil {
			panic(err)
		}
		if _, ok := token.(xml.EndElement); ok {
			depth--
		}
	}
	if !found {
		panic("图标缺少图层：" + id)
	}
	if err := encoder.Flush(); err != nil {
		panic(err)
	}
	out.WriteString("</svg>\n")
	return out.Bytes()
}

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
	if err := os.MkdirAll("assets/mascot", 0755); err != nil {
		panic(err)
	}
	body := mascotLayer(data, "mascot-body")
	if err := os.WriteFile("assets/mascot/body.svg", body, 0644); err != nil {
		panic(err)
	}
	mascot := ui.MustParseSVG(body)
	mascotPreview := ui.NewTester(func(c *ui.Context) {
		c.Root().Background(ui.Transparent)
		ui.Image(c, mascot).Fill().Fit(ui.Contain)
	}, 246, 246)
	save("assets/mascot/body.png", mascotPreview.Image())
	source := ui.MustParseSVG(data)
	preview := ui.NewTester(func(c *ui.Context) {
		c.Root().Background(ui.Transparent)
		ui.Image(c, source).Fill().Fit(ui.Contain)
	}, 1024, 1024)
	img := preview.Image()
	save("assets/app-icon.png", img)
	darkSource := strings.NewReplacer("#FAFBFF", "#343A50", "#EDEEFA", "#252A38", "#D5DCEB", "#414B63").Replace(string(data))
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
