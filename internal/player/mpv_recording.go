//go:build darwin || windows

package player

import (
	"errors"
	"os"
	"strconv"
	"unsafe"
)

const (
	mpvFormatDouble    = 5
	mpvFormatNode      = 6
	mpvFormatNodeArray = 7
	mpvFormatNodeMap   = 8
)

func (p *Player) StartRecording(path string) error {
	if p.vt != nil {
		return p.vt.Command("set", "stream-record", path)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.handle == 0 {
		return errors.New("播放器已关闭")
	}

	var position float64
	if p.api.getProperty(p.handle, "time-pos", mpvFormatDouble, unsafe.Pointer(&position)) < 0 {
		return errors.New("直播还未开始播放，请稍后再录制")
	}
	start, end, ok := p.recordingCacheRange()
	if !ok {
		return errors.New("正在等待可用的关键帧，请稍后再录制")
	}
	// dump-cache 回到所选位置之前的关键帧，再沿用当前连接写入后续包。
	// 直播靠近缓存末端时，最后一个完整 GOP 可能还在当前播放位置之前。
	position = max(start, min(position, end-0.001))
	if err := p.command("async", "dump-cache", strconv.FormatFloat(position, 'f', -1, 64), "no", path); err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		return err
	}
	return nil
}

func (p *Player) StopRecording(path string) error {
	if p.vt != nil {
		return p.vt.Command("set", "stream-record", "")
	}
	// 空文件名结束录制，命令返回时 mpv 已写完文件尾。
	if err := p.Command("dump-cache", "no", "no", ""); err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return errors.New("片段没有录到音视频数据")
	}
	return nil
}

// p.mu 须已持有。libmpv 将最近的缓存范围放在首项。
func (p *Player) recordingCacheRange() (start, end float64, ok bool) {
	var cache mpvNode
	if p.api.getProperty(p.handle, "demuxer-cache-state", mpvFormatNode, unsafe.Pointer(&cache)) < 0 {
		return 0, 0, false
	}
	defer p.api.freeNode(&cache)

	ranges := cache.mapField("seekable-ranges")
	if ranges == nil || ranges.Format != mpvFormatNodeArray {
		return 0, 0, false
	}
	list := *(**mpvNodeList)(unsafe.Pointer(&ranges.Value))
	if list.Count == 0 {
		return 0, 0, false
	}
	latest := (*mpvNode)(list.Values)
	first, last := latest.mapField("start"), latest.mapField("end")
	if first == nil || last == nil || first.Format != mpvFormatDouble || last.Format != mpvFormatDouble {
		return 0, 0, false
	}
	start = *(*float64)(unsafe.Pointer(&first.Value))
	end = *(*float64)(unsafe.Pointer(&last.Value))
	return start, end, start < end
}

func (n *mpvNode) mapField(name string) *mpvNode {
	if n.Format != mpvFormatNodeMap {
		return nil
	}
	list := *(**mpvNodeList)(unsafe.Pointer(&n.Value))
	for i, key := range unsafe.Slice((*unsafe.Pointer)(list.Keys), list.Count) {
		if cstring(key) == name {
			return &unsafe.Slice((*mpvNode)(list.Values), list.Count)[i]
		}
	}
	return nil
}
