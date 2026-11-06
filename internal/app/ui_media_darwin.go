//go:build darwin

package app

import (
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

type macMediaKeys struct {
	live, mediaType, contentID objc.ID
}

var loadMacMediaKeys = sync.OnceValue(func() macMediaKeys {
	lib, err := purego.Dlopen("/System/Library/Frameworks/MediaPlayer.framework/MediaPlayer", purego.RTLD_NOW)
	if err != nil {
		return macMediaKeys{}
	}
	key := func(name string) objc.ID {
		symbol, err := purego.Dlsym(lib, name)
		if err != nil {
			return 0
		}
		return **(**objc.ID)(unsafe.Pointer(&symbol))
	}
	return macMediaKeys{
		live:      key("MPNowPlayingInfoPropertyIsLiveStream"),
		mediaType: key("MPNowPlayingInfoPropertyMediaType"),
		contentID: key("MPNowPlayingInfoPropertyExternalContentIdentifier"),
	}
})

func (a *app) publishNativeMediaMetadata() {
	if a.win == nil || a.targetRoom.ID == "" {
		return
	}
	generation := a.generation
	roomID := roomKey(a.targetRoom)
	// MyGo 先在主线程发布媒体词典，切换播放状态时也会重新生成它。
	mygo.App.WhenReady(func() {
		if a.closing || !a.shown || !a.ownsSystemMedia() || a.generation != generation || roomKey(a.targetRoom) != roomID {
			return
		}
		keys := loadMacMediaKeys()
		if keys.live == 0 || keys.mediaType == 0 || keys.contentID == 0 {
			return
		}
		sel := objc.RegisterName
		center := objc.ID(objc.GetClass("MPNowPlayingInfoCenter")).Send(sel("defaultCenter"))
		previous := center.Send(sel("nowPlayingInfo"))
		if previous == 0 {
			return
		}
		info := previous.Send(sel("mutableCopy"))
		defer info.Send(sel("release"))
		number := objc.ID(objc.GetClass("NSNumber"))
		// MPNowPlayingInfoMediaTypeAudio 和 MPNowPlayingInfoMediaTypeVideo。
		mediaType := 2
		if a.audioOnly() {
			mediaType = 1
		}
		info.Send(sel("setObject:forKey:"), number.Send(sel("numberWithBool:"), true), keys.live)
		info.Send(sel("setObject:forKey:"), number.Send(sel("numberWithInteger:"), mediaType), keys.mediaType)
		identifier := objc.ID(objc.GetClass("NSString")).Send(sel("stringWithUTF8String:"), roomID)
		info.Send(sel("setObject:forKey:"), identifier, keys.contentID)
		center.Send(sel("setNowPlayingInfo:"), info)
	})
}
