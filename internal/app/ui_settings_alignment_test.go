//go:build darwin || windows

package app

import (
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"live-mygo/internal/platforms"
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
			a.accountStatus = map[platforms.Platform]string{platforms.Bilibili: "已保存"}
			a.settings.manual = true
			tester := ui.NewTester(a.settingsView, size[0], size[1])
			tester.SetDark(dark)
			for _, tc := range []struct {
				tab, section int
				labels       []string
				actions      []string
			}{
				{settingsGeneral, 0, []string{"启动页", "应用外观", "关注与偏好"}, []string{"导出备份"}},
				{settingsPlayback, 0, []string{"默认画质", "播放音量", "离开直播间时暂停", "窗口跟随画面比例", "斗鱼弹幕兼容连接"}, nil},
				{settingsBarrage, 0, []string{"显示画面弹幕", "字体", "字重", "字号", "文字颜色", "描边", "不透明度"}, nil},
				{settingsBarrage, 2, []string{"显示画面弹幕", "过滤画面重复弹幕", "启用屏蔽规则", "过滤重复消息", "关键词与用户屏蔽"}, nil},
				{settingsChat, 0, []string{"显示弹幕列表", "列表样式", "文字大小", "显示接收时间", "显示粉丝牌", "显示弹幕连接提示", "过滤重复消息", "关键词与用户屏蔽"}, nil},
				{settingsAccounts, 0, []string{"来源"}, []string{"重新登录哔哩哔哩", "登录斗鱼", "登录抖音", "登录企鹅体育", "保存导入 Cookie"}},
				{settingsAbout, 0, []string{"更新通道", "应用更新", "播放诊断"}, []string{"检查软件更新", "导出诊断报告"}},
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
				for _, label := range tc.actions {
					r, ok := tester.Find(label)
					if !ok || r.W != settingsActionWidth || r.H != settingsControlHeight || math.Abs(float64(r.X+r.W-float32(size[0]-41))) > .01 {
						t.Fatalf("设置操作尺寸或右边线不同：size=%v dark=%v tab=%d label=%s rect=%+v found=%v", size, dark, tc.tab, label, r, ok)
					}
				}
			}
		}
	}
}
