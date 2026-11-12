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

const monitorVideoAspect = float32(16.0 / 9)

type monitorDrag struct {
	key            string
	resize         bool
	changed        bool
	startX, startY float32
	origin, frame  ui.Rect
}

type monitorArrangement struct {
	id, label string
	columns   int
}

// 自由拖放之外的固定排列。选择后清除自定义坐标，拖动任一画面又回到自由布局。
var monitorArrangements = []monitorArrangement{
	{"", "自动排列", 0},
	{"focus", "一大多小", 0},
	{"1", "单列", 1},
	{"2", "两列", 2},
	{"3", "三列", 3},
}

func monitorArrangementLabel(id string) string {
	for _, item := range monitorArrangements {
		if item.id == id {
			return item.label
		}
	}
	return monitorArrangements[0].label
}

// 自动排列按 16:9 画面挑出让每格最大的列数，末行居中，不把画面拉成别的比例。
func monitorGridColumns(canvas ui.Rect, count int) int {
	best, bestArea := 1, float32(-1)
	for cols := 1; cols <= count; cols++ {
		rows := (count + cols - 1) / cols
		w, h := canvas.W/float32(cols), canvas.H/float32(rows)
		if w/h > monitorVideoAspect {
			w = h * monitorVideoAspect
		} else {
			h = w / monitorVideoAspect
		}
		if area := w * h; area > bestArea+.5 {
			best, bestArea = cols, area
		}
	}
	return best
}

func monitorGrid(canvas ui.Rect, count, cols int) []ui.Rect {
	cols = max(1, min(cols, count))
	rows := (count + cols - 1) / cols
	cellW, cellH := canvas.W/float32(cols), canvas.H/float32(rows)
	frames := make([]ui.Rect, count)
	for i := range frames {
		row, col := i/cols, i%cols
		items := min(cols, count-row*cols)
		offset := (canvas.W - cellW*float32(items)) / 2
		frames[i] = ui.Rect{X: canvas.X + offset + cellW*float32(col), Y: canvas.Y + cellH*float32(row), W: cellW, H: cellH}
	}
	return frames
}

// 一大多小：选中的画面放左侧，其余在右侧一列等分。
func monitorFocusGrid(canvas ui.Rect, count int) []ui.Rect {
	if count <= 1 {
		return []ui.Rect{canvas}
	}
	side := canvas.W * .3
	main := ui.Rect{X: canvas.X, Y: canvas.Y, W: canvas.W - side, H: canvas.H}
	frames := []ui.Rect{main}
	h := canvas.H / float32(count-1)
	for i := 0; i < count-1; i++ {
		frames = append(frames, ui.Rect{X: main.X + main.W, Y: canvas.Y + h*float32(i), W: side, H: h})
	}
	return frames
}

// 自定义布局使用相对坐标，窗口缩放后仍能恢复；没有坐标的画面按所选排列放置。
func monitorFrames(canvas ui.Rect, rooms []platforms.Room, state library.MonitorState, focus string) map[string]ui.Rect {
	frames := make(map[string]ui.Rect, len(rooms))
	if len(rooms) == 0 {
		return frames
	}
	order := rooms
	var grid []ui.Rect
	switch state.Arrange {
	case "focus":
		if i := slices.IndexFunc(rooms, func(room platforms.Room) bool { return roomKey(room) == focus }); i > 0 {
			order = append([]platforms.Room{rooms[i]}, slices.Delete(slices.Clone(rooms), i, i+1)...)
		}
		grid = monitorFocusGrid(canvas, len(rooms))
	default:
		cols := monitorGridColumns(canvas, len(rooms))
		for _, item := range monitorArrangements {
			if item.id == state.Arrange && item.columns > 0 {
				cols = item.columns
			}
		}
		grid = monitorGrid(canvas, len(rooms), cols)
	}
	for i, room := range order {
		key := roomKey(room)
		if saved := state.Layout[key]; saved.W > 0 && saved.H > 0 {
			width := min(canvas.W, max(monitorMinTileWidth, saved.W*canvas.W))
			height := min(canvas.H, max(monitorMinTileHeight, saved.H*canvas.H))
			x := max(0, min(canvas.W-width, saved.X*canvas.W))
			y := max(0, min(canvas.H-height, saved.Y*canvas.H))
			frames[key] = ui.Rect{X: x, Y: y, W: width, H: height}
			continue
		}
		frames[key] = grid[i]
	}
	return frames
}

func (a *app) setMonitorArrangement(id string) {
	next := a.monitorState()
	next.Arrange, next.Layout = id, nil
	a.monitorDrag = monitorDrag{}
	if a.saveMonitor(next) {
		a.invalidateMonitor()
	}
}

// 拖动按指针相对按下位置的总位移计算，画面被边界卡住后往回拖不会与指针错开。
func (a *app) dragMonitorTile(c *ui.Context, grip ui.Element, key string, frame, canvas ui.Rect, resize bool) {
	_, _, pressed := grip.Dragged()
	if !pressed {
		if a.monitorDrag.key == key && a.monitorDrag.resize == resize {
			a.finishMonitorDrag(canvas)
		}
		return
	}
	px, py, _ := c.Root().PointerPosition()
	if a.monitorDrag.key == "" {
		a.monitorSelected = key
		a.monitorDrag = monitorDrag{key: key, resize: resize, startX: px, startY: py, origin: frame, frame: frame}
		c.After(time.Millisecond)
		return
	}
	drag := &a.monitorDrag
	if drag.key != key || drag.resize != resize {
		return
	}
	dx, dy := px-drag.startX, py-drag.startY
	next := drag.origin
	if resize {
		maxWidth, maxHeight := canvas.W-next.X, canvas.H-next.Y
		next.W = max(min(monitorMinTileWidth, maxWidth), min(maxWidth, next.W+dx))
		next.H = max(min(monitorMinTileHeight, maxHeight), min(maxHeight, next.H+dy))
	} else {
		next.X = max(0, min(canvas.W-next.W, next.X+dx))
		next.Y = max(0, min(canvas.H-next.H, next.Y+dy))
	}
	if next != drag.frame {
		drag.frame, drag.changed = next, true
	}
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
	frames := monitorFrames(canvas, rooms, next, a.monitorSelected)
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
