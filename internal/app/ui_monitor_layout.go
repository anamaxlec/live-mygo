//go:build darwin || windows

package app

import (
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
	"live-mygo/internal/library"
	"live-mygo/internal/platforms"
)

const monitorMinTileWidth, monitorMinTileHeight = 220, 112

type monitorDrag struct {
	key     string
	resize  bool
	changed bool
	frame   ui.Rect
}

// 自动布局保持画面贴合。自定义布局使用相对坐标，窗口缩放后仍能恢复。
func monitorFrames(canvas ui.Rect, rooms []platforms.Room, layout map[string]library.MonitorRect) map[string]ui.Rect {
	frames := make(map[string]ui.Rect, len(rooms))
	if len(rooms) == 0 {
		return frames
	}
	cols := min(2, len(rooms))
	if len(rooms) > 4 {
		cols = 3
	}
	rows := (len(rooms) + cols - 1) / cols
	for i, room := range rooms {
		key := roomKey(room)
		if saved := layout[key]; saved.W > 0 && saved.H > 0 {
			width := min(canvas.W, max(monitorMinTileWidth, saved.W*canvas.W))
			height := min(canvas.H, max(monitorMinTileHeight, saved.H*canvas.H))
			x := max(0, min(canvas.W-width, saved.X*canvas.W))
			y := max(0, min(canvas.H-height, saved.Y*canvas.H))
			frames[key] = ui.Rect{X: x, Y: y, W: width, H: height}
			continue
		}
		row, col := i/cols, i%cols
		items := min(cols, len(rooms)-row*cols)
		x, right := canvas.W*float32(col)/float32(items), canvas.W*float32(col+1)/float32(items)
		y, bottom := canvas.H*float32(row)/float32(rows), canvas.H*float32(row+1)/float32(rows)
		frames[key] = ui.Rect{X: x, Y: y, W: right - x, H: bottom - y}
	}
	return frames
}

func (a *app) dragMonitorTile(c *ui.Context, grip ui.Element, key string, frame, canvas ui.Rect, resize bool) {
	dx, dy, pressed := grip.Dragged()
	if !pressed {
		if a.monitorDrag.key == key && a.monitorDrag.resize == resize {
			a.finishMonitorDrag(canvas)
		}
		return
	}
	if a.monitorDrag.key == "" {
		a.monitorSelected = key
		a.monitorDrag = monitorDrag{key: key, frame: frame, resize: resize}
		c.After(time.Millisecond)
	}
	drag := &a.monitorDrag
	if drag.key != key || drag.resize != resize || dx == 0 && dy == 0 {
		return
	}
	if resize {
		maxWidth, maxHeight := canvas.W-drag.frame.X, canvas.H-drag.frame.Y
		drag.frame.W = max(min(monitorMinTileWidth, maxWidth), min(maxWidth, drag.frame.W+dx))
		drag.frame.H = max(min(monitorMinTileHeight, maxHeight), min(maxHeight, drag.frame.H+dy))
	} else {
		drag.frame.X = max(0, min(canvas.W-drag.frame.W, drag.frame.X+dx))
		drag.frame.Y = max(0, min(canvas.H-drag.frame.H, drag.frame.Y+dy))
	}
	drag.changed = true
	c.After(time.Millisecond)
}

func (a *app) finishMonitorDrag(canvas ui.Rect) {
	drag := a.monitorDrag
	a.monitorDrag = monitorDrag{}
	if !drag.changed {
		return
	}
	next := a.monitorState()
	rooms := a.monitorRooms()
	frames := monitorFrames(canvas, rooms, next.Layout)
	frames[drag.key] = drag.frame
	next.Layout = make(map[string]library.MonitorRect, len(rooms))
	for _, room := range rooms {
		key := roomKey(room)
		frame := frames[key]
		next.Layout[key] = library.MonitorRect{
			X: frame.X / canvas.W, Y: frame.Y / canvas.H,
			W: frame.W / canvas.W, H: frame.H / canvas.H,
		}
	}
	// 保留画面被拖到前方后的叠放次序。
	if i := slices.IndexFunc(rooms, func(room platforms.Room) bool { return roomKey(room) == drag.key }); i >= 0 {
		room := rooms[i]
		next.Rooms = append(slices.Delete(rooms, i, i+1), room)
	}
	a.saveMonitor(next)
	a.invalidateMonitor()
}
