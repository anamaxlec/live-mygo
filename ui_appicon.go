//go:build darwin

package main

import (
	"bytes"
	_ "embed"
	"image/png"
	"sync"

	"github.com/egoist/mygo/ui"
)

//go:embed assets/app-icon.png
var appIconPNG []byte

//go:embed assets/app-icon-dark.png
var appIconDarkPNG []byte

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
