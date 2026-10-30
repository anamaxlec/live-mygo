//go:build darwin || windows

package player

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"
	"unicode"
	"unsafe"
)

type mpvNode struct {
	Value  uintptr
	Format int32
	_      int32
}
type mpvNodeList struct {
	Count  int32
	_      int32
	Values unsafe.Pointer
	Keys   unsafe.Pointer
}

func (p *Player) namedCommand(name string, arguments map[string]any) error {
	arguments["name"] = name
	keys := make([]*byte, 0, len(arguments))
	values := make([]mpvNode, 0, len(arguments))
	buffers := make([][]byte, 0, len(arguments)*2)
	cstring := func(s string) *byte { b := append([]byte(s), 0); buffers = append(buffers, b); return &b[0] }
	for key, value := range arguments {
		keys = append(keys, cstring(key))
		node := mpvNode{}
		switch v := value.(type) {
		case string:
			if strings.ContainsRune(v, 0) {
				return errors.New("弹幕包含无效字符")
			}
			node = mpvNode{Value: uintptr(unsafe.Pointer(cstring(v))), Format: 1}
		case int:
			node = mpvNode{Value: uintptr(v), Format: 4}
		default:
			return errors.New("不支持的播放器参数")
		}
		values = append(values, node)
	}
	list := mpvNodeList{Count: int32(len(keys)), Values: unsafe.Pointer(&values[0]), Keys: unsafe.Pointer(&keys[0])}
	root := mpvNode{Value: uintptr(unsafe.Pointer(&list)), Format: 8}
	rc := p.api.commandNode(p.handle, unsafe.Pointer(&root), nil)
	runtime.KeepAlive(buffers)
	runtime.KeepAlive(keys)
	runtime.KeepAlive(values)
	runtime.KeepAlive(list)
	if rc < 0 {
		return fmt.Errorf("弹幕显示失败：%s", p.api.errorString(rc))
	}
	return nil
}

type flyingMessage struct {
	Text    string
	Color   uint32
	Lane    int
	Width   float64
	Started time.Time
}
type barrageState struct {
	Enabled       bool
	Size, Opacity float64
	Items         []flyingMessage
	Lanes         [32]time.Time
	Speed, Area   float64
	Drawn         bool
	VideoAspect   float64
}

func (p *Player) ConfigureBarrage(enabled bool, size, opacity float64, options ...float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	size = max(20, min(40, size))
	speed, area := float64(1), float64(.5)
	if len(options) > 0 && options[0] > 0 {
		speed = max(.5, min(2, options[0]))
	}
	if len(options) > 1 && options[1] > 0 {
		area = max(.25, min(1, options[1]))
	}
	if !enabled || p.barrage.Size != size || p.barrage.Speed != speed || p.barrage.Area != area {
		p.clearBarrage()
	}
	p.barrage.Speed, p.barrage.Area = speed, area
	p.barrage.Enabled = enabled
	p.barrage.Size = size
	p.barrage.Opacity = max(40, min(100, opacity))
}

func (p *Player) AddBarrage(text string, color uint32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.barrage.Enabled || p.renderer == nil || !p.renderer.surface.visible.Load() {
		return
	}
	now := time.Now()
	lane := -1
	aspect := p.barrage.VideoAspect
	if aspect <= 0 {
		aspect = 16.0 / 9
	}
	lines := max(1, min(32, int((1280/aspect*p.barrage.Area-35)/(p.barrage.Size+14))))
	for i, available := range p.barrage.Lanes[:lines] {
		if !now.Before(available) {
			lane = i
			break
		}
	}
	if lane < 0 {
		return
	}
	runes := []rune(text)
	if len(runes) > 80 {
		runes = append(runes[:80], '…')
	}
	width := float64(0)
	for _, r := range runes {
		if unicode.Is(unicode.Han, r) || r > 0xFF {
			width += p.barrage.Size
		} else {
			width += p.barrage.Size * .62
		}
	}
	p.barrage.Lanes[lane] = now.Add(time.Duration((width + 70) / (160 * p.barrage.Speed) * float64(time.Second)))
	if color == 0 {
		color = 0xffffff
	}
	p.barrage.Items = append(p.barrage.Items, flyingMessage{Text: escapeASS(string(runes)), Color: color, Lane: lane, Width: width, Started: now})
}

func escapeASS(text string) string {
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text)
	return strings.NewReplacer("\\", "\\\u2060", "{", "\\{", "}", "\\}").Replace(text)
}

func (p *Player) clearBarrage() {
	p.barrage.Items = nil
	p.barrage.Lanes = [32]time.Time{}
	if p.barrage.Drawn {
		_ = p.namedCommand("osd-overlay", map[string]any{"id": 7, "format": "none", "data": ""})
		p.barrage.Drawn = false
	}
}

func (p *Player) TickBarrage() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.handle == 0 || !p.barrage.Enabled || p.renderer == nil || !p.renderer.surface.visible.Load() {
		return nil
	}
	if len(p.barrage.Items) == 0 && !p.barrage.Drawn {
		return nil
	}
	now := time.Now()
	canvasWidth := 720 * float64(p.renderer.surface.width.Load()) / float64(max(1, p.renderer.surface.height.Load()))
	aspect := p.barrage.VideoAspect
	if aspect <= 0 {
		aspect = 16.0 / 9
	}
	videoHeight := 1280 / aspect
	scale := min(canvasWidth/1280, 720/videoHeight)
	offsetX, offsetY := (canvasWidth-1280*scale)/2, (720-videoHeight*scale)/2
	alive := p.barrage.Items[:0]
	var lines []string
	for _, item := range p.barrage.Items {
		x := 1280 - now.Sub(item.Started).Seconds()*160*p.barrage.Speed
		if x+item.Width < 0 {
			continue
		}
		alive = append(alive, item)
		color := item.Color&0xFF<<16 | item.Color&0xFF00 | item.Color>>16&0xFF
		alpha := int((100 - p.barrage.Opacity) * 255 / 100)
		lines = append(lines, fmt.Sprintf(`{\an7\pos(%.1f,%.1f)\fnPingFang SC\fs%.1f\bord1.3\shad0\1c&H%06X&\3c&H101010&\alpha&H%02X&}%s`, offsetX+x*scale, offsetY+(20+float64(item.Lane)*(p.barrage.Size+14))*scale, p.barrage.Size*scale, color, alpha, item.Text))
	}
	p.barrage.Items = alive
	format, data := "ass-events", strings.Join(lines, "\n")
	if data == "" {
		format = "none"
	}
	err := p.namedCommand("osd-overlay", map[string]any{"id": 7, "format": format, "data": data, "res_x": 0, "res_y": 720})
	p.barrage.Drawn = data != ""
	if err != nil {
		p.barrage.Enabled = false
	}
	return err
}

// ClearBarrage 移除当前屏幕上的消息，接收连接继续工作。
func (p *Player) ClearBarrage() { p.mu.Lock(); defer p.mu.Unlock(); p.clearBarrage() }

func (p *Player) BarrageActive() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.barrage.Enabled && p.renderer != nil && p.renderer.surface.visible.Load() && (len(p.barrage.Items) > 0 || p.barrage.Drawn)
}
