//go:build darwin || windows

package app

import (
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestSettingsActionsMatchLabelInsetTypeAndRowHeight(t *testing.T) {
	for _, full := range []bool{false, true} {
		for _, chat := range []bool{false, true} {
			a := polishApp(t)
			a.danmakuTab = 2
			tester := ui.NewTester(func(c *ui.Context) {
				if chat {
					a.chatSettingsContent(c, full)
				} else {
					a.barrageSettings(c, full)
				}
			}, 560, 1000)
			label, ok := tester.Find("过滤重复消息")
			if !ok {
				t.Fatal("缺少开关文字")
			}
			action, ok := tester.Find("关键词与用户屏蔽")
			if !ok || math.Abs(float64(action.X-label.X)) > .01 || math.Abs(float64(action.H-label.H)) > .01 {
				t.Fatalf("导航行的文字起点或字号与开关行不同：full=%v chat=%v label=%+v action=%+v", full, chat, label, action)
			}
			row, _ := tester.Find("打开关键词与用户屏蔽")
			want := float32(35)
			if full {
				want = 50
			}
			if row.H != want || math.Abs(float64(action.Y+action.H/2-row.Y-row.H/2)) > .5 {
				t.Fatalf("导航行行高或垂直对齐不一致：%+v %+v", row, action)
			}
		}
	}
}

func TestFullSettingsUseOneTextColumnAcrossCategories(t *testing.T) {
	for _, size := range [][2]int{{760, 900}, {820, 1000}} {
		for _, dark := range []bool{false, true} {
			a := polishApp(t)
			tester := ui.NewTester(a.settingsView, size[0], size[1])
			tester.SetDark(dark)
			for _, tc := range []struct {
				tab, section int
				labels       []string
			}{
				{settingsPlayback, 0, []string{"播放音量", "离开直播间时暂停", "窗口跟随画面比例", "斗鱼弹幕兼容连接"}},
				{settingsBarrage, 0, []string{"显示画面弹幕", "字体", "字重", "字号", "文字颜色", "描边", "不透明度"}},
				{settingsBarrage, 2, []string{"显示画面弹幕", "过滤画面重复弹幕", "启用屏蔽规则", "过滤重复消息", "关键词与用户屏蔽"}},
				{settingsChat, 0, []string{"显示弹幕列表", "列表样式", "文字大小", "显示接收时间", "显示粉丝牌", "显示连接面板", "过滤重复消息", "关键词与用户屏蔽"}},
			} {
				a.settings.tab, a.danmakuTab = tc.tab, tc.section
				tester.Frame()
				tester.Advance(250 * time.Millisecond)
				for _, label := range tc.labels {
					r, ok := tester.Find(label)
					if !ok || math.Abs(float64(r.X-216)) > .01 {
						t.Fatalf("设置文字列未对齐：size=%v dark=%v tab=%d label=%s rect=%+v found=%v", size, dark, tc.tab, label, r, ok)
					}
				}
			}
		}
	}
}
