//go:build darwin

package app

import (
	"bytes"
	"image/png"
	"sync"

	"github.com/egoist/mygo/ui"
	appassets "live-mygo/assets"
)

var appIconPNG = readAsset("app-icon.png")
var appIconDarkPNG = readAsset("app-icon-dark.png")

func readAsset(name string) []byte {
	data, err := appassets.Files.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return data
}

var iconBitmap *ui.Bitmap
var iconOnce sync.Once

func appIcon() *ui.Bitmap {
	iconOnce.Do(func() {
		img, err := png.Decode(bytes.NewReader(appIconPNG))
		if err != nil {
			panic(err)
		}
		iconBitmap = ui.NewBitmap(img)
	})
	return iconBitmap
}
