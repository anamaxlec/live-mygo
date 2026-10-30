package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"golang.org/x/image/draw"
	appassets "live-mygo/assets"
)

func main() {
	output := flag.String("out", "build/AppIcon.ico", "Windows 图标路径")
	flag.Parse()
	data, err := appassets.Files.ReadFile("app-icon.png")
	must(err)
	source, err := png.Decode(bytes.NewReader(data))
	must(err)
	sizes := []int{16, 24, 32, 48, 128, 256}
	images := make([][]byte, len(sizes))
	for i, size := range sizes {
		img := image.NewNRGBA(image.Rect(0, 0, size, size))
		draw.CatmullRom.Scale(img, img.Bounds(), source, source.Bounds(), draw.Src, nil)
		var b bytes.Buffer
		must(png.Encode(&b, img))
		images[i] = b.Bytes()
	}
	var icon bytes.Buffer
	must(binary.Write(&icon, binary.LittleEndian, []uint16{0, 1, uint16(len(images))}))
	offset := uint32(6 + 16*len(images))
	for i, png := range images {
		width := byte(sizes[i])
		icon.Write([]byte{width, width, 0, 0})
		must(binary.Write(&icon, binary.LittleEndian, []uint16{1, 32}))
		must(binary.Write(&icon, binary.LittleEndian, []uint32{uint32(len(png)), offset}))
		offset += uint32(len(png))
	}
	for _, png := range images {
		icon.Write(png)
	}
	must(os.MkdirAll(filepath.Dir(*output), 0755))
	must(os.WriteFile(*output, icon.Bytes(), 0644))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
