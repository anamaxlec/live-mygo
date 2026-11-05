//go:build darwin || windows

package app

import "live-mygo/internal/library"

func (a *app) sessionToResume() *library.PlaybackSession {
	if a.library == nil || !a.library.Preferences.ResumeSession || a.library.LastSession == nil || a.opts.room != "" || a.opts.file != "" || a.opts.platform != "" {
		return nil
	}
	if a.library.LastSession.Room.ID == "" || a.library.LastSession.Room.Platform == "" {
		return nil
	}
	snapshot := *a.library.LastSession
	return &snapshot
}
