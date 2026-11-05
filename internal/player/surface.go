//go:build darwin || windows

package player

// VideoSurface is the native video output area the app lays out and shows.
// The mpv backend renders through OpenGL; the experimental macOS backend
// renders through a CAMetalLayer.
type VideoSurface interface {
	Close()
	Show(bool)
	SetBounds(x, y, w, h, windowHeight float64) bool
	SetCorners(radius float64, right bool)
	SetWindowButtonsVisible(bool)
	Metrics() RenderMetrics
	RenderError() string
	SetFrameCallback(func())
}

// FrameSource is implemented by the Windows software surface only.
type FrameSource interface {
	TakeFrame() ([]byte, int, int, int, bool)
	ReleaseFrame([]byte)
}

// vtBackend is the optional macOS VideoToolbox playback path.
type vtBackend interface {
	Load(string, map[string]string) error
	Command(...string) error
	Stats() Stats
	Close()
}
