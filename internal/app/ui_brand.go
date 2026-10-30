//go:build darwin || windows

package app

import (
	"github.com/egoist/mygo/ui"
	appassets "live-mygo/assets"
	"live-mygo/internal/platforms"
)

func sourceSVG(name string) *ui.SVG {
	data, err := appassets.Files.ReadFile("platforms/" + name + ".svg")
	if err != nil {
		panic(err)
	}
	return ui.MustParseSVG(data)
}

var (
	logoBilibili = sourceSVG("bilibili")
	logoDouyu    = sourceSVG("douyu")
	logoHuya     = sourceSVG("huya")
	logoDouyin   = sourceSVG("douyin")
	logoQie      = sourceSVG("qie")
	logoFLive    = sourceSVG("flive")
)

func platformLogo(source platforms.Platform) *ui.SVG {
	switch source {
	case platforms.FLive:
		return logoFLive
	case platforms.Douyin:
		return logoDouyin
	case platforms.Qie:
		return logoQie
	case platforms.Bilibili:
		return logoBilibili
	case platforms.Douyu:
		return logoDouyu
	case platforms.Huya:
		return logoHuya
	default:
		return iconTV
	}
}

func sourceBadge(c *ui.Context, source platforms.Platform, compact bool) {
	p := colors(c.Theme())
	size, inset, logo := float32(30), float32(8), float32(23)
	if compact {
		size, inset, logo = 20, 4, 16
	}
	ui.Box(c.Key("source-badge")).Absolute().Top(inset).Left(inset).Size(size, size).Radius(size*.27).Center().Background(p.bg).Border(.5, p.line).Label("来源：" + source.Name()).Tooltip(source.Name()).Children(func() {
		ui.Icon(c, platformLogo(source)).Size(logo, logo).TextColor(p.muted)
	})
}
