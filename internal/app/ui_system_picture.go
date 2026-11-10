//go:build darwin || windows

package app

type systemPictureInPicture interface {
	Close()
	SetPaused(bool)
}

func (a *app) canOpenSystemPicture() bool {
	return a.win != nil && a.surface != nil && a.shown && !a.waitingForVideo() && !a.stats.Paused && !a.audioOnly() && !a.busy && !a.embedded && !a.hasSideViewer()
}

func (a *app) toggleSystemPicture() {
	if a.closeSystemPicture() || !a.canOpenSystemPicture() {
		return
	}
	picture, err := a.startSystemPicture()
	if err != nil {
		a.toast = err.Error()
		return
	}
	a.systemPicture = picture
}

func (a *app) closeSystemPicture() bool {
	if a.systemPicture == nil {
		return false
	}
	picture := a.systemPicture
	a.systemPicture = nil
	picture.Close()
	return true
}
