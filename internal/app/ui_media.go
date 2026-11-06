//go:build darwin || windows

package app

import (
	"time"

	"github.com/egoist/mygo"
)

func (a *app) mediaInfo() mygo.NowPlayingInfo {
	state := mygo.PlaybackPlaying
	if a.stats.Paused {
		state = mygo.PlaybackPaused
	}
	artwork := a.covers[a.targetRoom.Cover].data
	if len(artwork) == 0 {
		artwork = appIconPNG
	}
	return mygo.NowPlayingInfo{Title: a.title, Artist: a.displayRoomName(a.targetRoom), Album: a.targetRoom.Platform.Name(), Artwork: artwork, Duration: 0, State: state, Commands: mygo.MediaPlay | mygo.MediaPause | mygo.MediaStop}
}

func (a *app) registerMediaControls() {
	a.mediaCommandsOff = mygo.NowPlaying.OnCommand(func(cmd mygo.MediaCommand, _ time.Duration) {
		owner := a.mediaOwner
		if owner == nil {
			owner = a
		}
		owner.handleMediaCommand(cmd)
	})
}

func (a *app) handleMediaCommand(cmd mygo.MediaCommand) {
	if a.closing || a.player == nil {
		return
	}
	switch cmd {
	case mygo.MediaPlay:
		if a.shown && a.stats.Paused {
			a.togglePause()
		}
	case mygo.MediaPause:
		if a.shown && !a.stats.Paused {
			a.togglePause()
		}
	case mygo.MediaStop:
		a.closePlayback()
	}
}

func (a *app) ownsSystemMedia() bool {
	root := a.rootApp()
	owner := root.mediaOwner
	if owner == nil {
		owner = root
	}
	return root.mediaCommandsOff != nil && owner == a
}

func (a *app) publishMediaInfo() {
	if !a.ownsSystemMedia() {
		return
	}
	if !a.shown {
		mygo.NowPlaying.Clear()
		return
	}
	mygo.NowPlaying.Set(a.mediaInfo())
	a.publishNativeMediaMetadata()
	a.cover(a.targetRoom.Cover)
}

func (a *app) updateSystemPlayback() {
	if !a.ownsSystemMedia() {
		return
	}
	if !a.shown {
		mygo.NowPlaying.Clear()
		return
	}
	state := mygo.PlaybackPlaying
	if a.stats.Paused {
		state = mygo.PlaybackPaused
	}
	mygo.NowPlaying.SetPlayback(state, 0)
	a.publishNativeMediaMetadata()
}
