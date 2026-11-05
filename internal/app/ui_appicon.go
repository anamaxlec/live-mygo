//go:build darwin || windows

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
		iconBitmap = bitmapAsset("app-icon.png")
	})
	return iconBitmap
}

func bitmapAsset(name string) *ui.Bitmap {
	img, err := png.Decode(bytes.NewReader(readAsset(name)))
	if err != nil {
		panic(err)
	}
	return ui.NewBitmap(img)
}
