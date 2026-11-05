//go:build darwin || windows

package app

import (
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
	"live-mygo/internal/platforms"
)

func TestReconnectPlanRefreshesCurrentThenTriesOtherSources(t *testing.T) {
	stream := &platforms.Stream{SourceID: "b", Sources: []platforms.Source{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "a"}}}
	if got := reconnectSourcePlan(stream); !reflect.DeepEqual(got, []string{"b", "a", "c"}) {
		t.Fatal("重试没有保留当前线路和不重复的备选线路", got)
	}
	if reconnectSourcePlan(nil) != nil {
		t.Fatal("本地文件不应生成直播恢复计划")
	}
}

func TestExhaustedRecoveryKeepsRoomIdentityAndOffersSettings(t *testing.T) {
	a := polishApp(t)
	a.page, a.focusMode = "player", true
	a.targetRoom = platforms.Room{Platform: platforms.Douyu, ID: "6979222", Title: "6657"}
	a.stream = &platforms.Stream{Room: a.targetRoom}
	a.status = "视频连接中断"
	a.reconnectAttempts = 3
	tester := ui.NewTester(a.view, 1180, 780)
	for _, label := range []string{"重新连接", "播放设置", "解析房间：房间已确认"} {
		if _, ok := tester.Find(label); !ok {
			t.Fatal("自动恢复耗尽后丢失恢复入口或房间身份", label)
		}
	}
}
