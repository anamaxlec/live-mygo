//go:build darwin || windows

package player

type RenderMetrics struct {
	Resizes         uint64  `json:"resizes"`
	DeferredResizes uint64  `json:"deferred_resizes"`
	MaxResizeMS     float64 `json:"max_main_thread_resize_ms"`
	RenderedFrames  uint64  `json:"rendered_frames"`
	UpdateCallbacks uint64  `json:"update_callbacks"`
	Backend         string  `json:"backend"`
}
