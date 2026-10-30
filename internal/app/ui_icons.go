//go:build darwin || windows

package app

import (
	"strings"

	"github.com/egoist/mygo/ui"
	appassets "live-mygo/assets"
)

func lucide(name string) *ui.SVG {
	data, err := appassets.Files.ReadFile("icons/" + name + ".svg")
	if err != nil {
		panic(err)
	}
	return ui.MustParseSVG(data)
}

func filledHeart() *ui.SVG {
	data, _ := appassets.Files.ReadFile("icons/heart.svg")
	return ui.MustParseSVG([]byte(strings.Replace(string(data), `fill="none"`, `fill="currentColor"`, 1)))
}

var (
	iconTV              = lucide("tv-minimal-play")
	iconHome            = ui.MustParseSVG([]byte(`<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m3 10 9-7 9 7v10a1 1 0 0 1-1 1h-5v-7H9v7H4a1 1 0 0 1-1-1Z"/></svg>`))
	iconSidebar         = lucide("panel-left")
	iconPanelRight      = lucide("panel-right")
	iconFish            = lucide("fish")
	iconHeart           = lucide("heart")
	iconHeartFilled     = filledHeart()
	iconHistory         = lucide("clock-3")
	iconSearch          = lucide("search")
	iconPerson          = lucide("user-round")
	iconChevron         = lucide("chevron-right")
	iconChevronDown     = lucide("chevron-down")
	iconBack            = lucide("chevron-left")
	iconRefresh         = lucide("rotate-cw")
	iconVolume          = lucide("volume-2")
	iconMuted           = lucide("volume-x")
	iconFullscreen      = lucide("maximize")
	iconExitFullscreen  = lucide("minimize")
	iconCompanion       = ui.MustParseSVG([]byte(`<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="4" width="18" height="16" rx="2"/><rect x="12" y="12" width="7" height="5" rx="1" fill="currentColor" stroke="none"/></svg>`))
	iconClose           = lucide("x")
	iconLink            = lucide("link")
	iconSignal          = lucide("signal")
	iconSettings        = lucide("settings-2")
	iconExternal        = lucide("external-link")
	iconMore            = lucide("ellipsis")
	iconChat            = lucide("message-square")
	iconDanmaku         = lucide("captions")
	iconDanmakuOff      = lucide("captions-off")
	iconPanelRightOpen  = lucide("panel-right-open")
	iconPanelRightClose = lucide("panel-right-close")
	iconCheck           = lucide("check")
	iconPlay            = ui.MustParseSVG([]byte(`<svg width="24" height="24" viewBox="0 0 24 24"><path d="M7 4.6c0-.7.8-1.1 1.4-.7l12 7.4c.5.3.5 1.1 0 1.4l-12 7.4c-.6.4-1.4 0-1.4-.7Z" fill="currentColor"/></svg>`))
	iconPause           = ui.MustParseSVG([]byte(`<svg width="24" height="24" viewBox="0 0 24 24"><rect x="5" y="4" width="5" height="16" rx="1.2" fill="currentColor"/><rect x="14" y="4" width="5" height="16" rx="1.2" fill="currentColor"/></svg>`))
	iconStop            = ui.MustParseSVG([]byte(`<svg width="24" height="24" viewBox="0 0 24 24"><rect x="6" y="6" width="12" height="12" rx="2" fill="currentColor"/></svg>`))
)
