//go:build darwin && !cgo

package player

type pictureInPictureSlot struct{}
type glPictureTarget struct{}

func (*glPictureTarget) close()                                                         {}
func (*glPictureTarget) draw(*Surface, uintptr, func(uintptr, *renderParam) int32) bool { return false }
