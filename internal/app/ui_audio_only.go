//go:build darwin || windows

package app

import "live-mygo/internal/player"

func (a *app) audioOnly() bool { return a.library != nil && a.library.Preferences.AudioOnly }

func (a *app) setAudioOnly(enabled bool) {
	if a.library == nil || a.audioModeChanging || a.audioOnly() == enabled {
		return
	}
	if enabled {
		a.closeSystemPicture()
	}
	a.stopRecording()
	apply := func() {
		next := a.library.Preferences
		next.AudioOnly = enabled
		a.savePreferences(next)
		a.updateSystemPlayback()
		if enabled {
			a.clearFlyingChat()
		}
		a.revealControls()
	}
	if a.player == nil {
		apply()
		return
	}
	a.audioModeChanging = true
	mode := "auto"
	if enabled {
		mode = "no"
	}
	a.doPlayer(func(p *player.Player) error { return p.Command("set", "vid", mode) }, func(err error) {
		defer func() { a.audioModeChanging = false }()
		if err != nil {
			a.toast = "切换播放模式失败"
			a.recordFailure("video", err.Error())
			return
		}
		apply()
	})
}
