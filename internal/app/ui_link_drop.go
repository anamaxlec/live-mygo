//go:build darwin || windows

package app

import (
	"strings"
	"unicode"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"live-mygo/internal/platforms"
)

var liveLinkDropOptions = transfer.DropOptions{Formats: []transfer.Format{transfer.URIList, transfer.Text}}

func (a *app) acceptLiveLinkDrop(c *ui.Context) {
	if a.closing {
		return
	}
	room, ok := a.readLiveLinkDrop(c.Root())
	if !ok {
		return
	}
	if a.player == nil {
		a.toast = "播放器正在初始化，请稍后再拖入链接"
		return
	}
	if a.page == "welcome" {
		a.finishWelcome()
	}
	if a.quick.open {
		a.closeQuickSearch()
	}
	a.playRoom(room)
}

func (a *app) readLiveLinkDrop(target ui.Element) (platforms.Room, bool) {
	drop, ok := ui.DropData(target, liveLinkDropOptions)
	if !ok {
		return platforms.Room{}, false
	}
	room, ok := droppedLiveRoom(drop.Data)
	if !ok || room.Platform == platforms.FLive && !a.raceMenuUnlocked() {
		a.toast = "请拖入已支持平台的直播间链接"
		return platforms.Room{}, false
	}
	return room, true
}

func droppedLiveRoom(data transfer.Data) (platforms.Room, bool) {
	urls, _ := data.URLs()
	for _, link := range urls {
		if room, ok := liveRoomFromLink(link); ok {
			return room, true
		}
	}
	plain, err := data.Read(transfer.Text)
	if err == nil {
		for link := range strings.FieldsFuncSeq(string(plain), func(r rune) bool {
			return unicode.IsSpace(r) || strings.ContainsRune("<>\"'()[]{}“”‘’（）【】《》，。！？、；：", r)
		}) {
			if room, ok := liveRoomFromLink(strings.TrimRight(link, ",.;!?")); ok {
				return room, true
			}
		}
	}
	return platforms.Room{}, false
}

func liveRoomFromLink(link string) (platforms.Room, bool) {
	scheme, _, ok := strings.Cut(link, "://")
	if !ok || !strings.EqualFold(scheme, "https") && !strings.EqualFold(scheme, "http") {
		return platforms.Room{}, false
	}
	source, id, err := platforms.ParseRoom("", link)
	return platforms.Room{Platform: source, ID: id}, err == nil
}
