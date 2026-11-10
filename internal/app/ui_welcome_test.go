//go:build darwin || windows

package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"live-mygo/internal/library"
)

func clickPreferenceSwitch(t *testing.T, tester *ui.Tester, label string) {
	t.Helper()
	row, found := tester.Find(label)
	if !found {
		t.Fatalf("缺少设置 %q", label)
	}
	tester.ClickAt(row.X+row.W+25, row.Y+row.H/2)
}

func TestWelcomeShowsLoginGuideAndCanGoStraightToHome(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	a := polishApp(t)
	var err error
	a.library, err = library.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	a.page = initialPage(a.library, options{})
	if a.page != "welcome" || initialPage(a.library, options{platform: "douyu"}) != "douyu" ||
		initialPage(a.library, options{file: "test.mp4"}) != "home" {
		t.Fatal("首次启动或显式启动入口选择错误")
	}
	// 即使在欢迎页退出时写过本机偏好，也应保留尚未完成的状态。
	if err := a.library.SetPreferences(a.library.Preferences); err != nil {
		t.Fatal(err)
	}
	a.library, err = library.Load(path)
	if err != nil || initialPage(a.library, options{}) != "welcome" {
		t.Fatal("未完成的欢迎页在重启后消失", err)
	}
	a.homeCatalog.updated = time.Now()
	tester := ui.NewTester(a.view, 1000, 580)
	for _, label := range []string{"选择平台", "在官网登录", "保存登录状态", "Cookie 加密保存在本机，只随对应平台的请求发送。", "打开账号设置", "进入首页"} {
		r, ok := tester.Find(label)
		if !ok || r.Y < watchHeaderHeight || r.Y+r.H > 580 {
			t.Fatalf("最小窗口的登录指引被裁切：%q %+v", label, r)
		}
	}
	login, _ := tester.Find("打开账号设置")
	home, _ := tester.Find("进入首页")
	if login.W != home.W || login.H != home.H || login.Y != home.Y {
		t.Fatalf("欢迎页按钮未对齐：%+v %+v", login, home)
	}
	if err := tester.Click("进入首页"); err != nil {
		t.Fatal(err)
	}
	reloaded, err := library.Load(path)
	if err != nil || a.page != "home" || initialPage(reloaded, options{}) != "home" {
		t.Fatal("进入首页后没有记录完成状态", err)
	}
}

func TestExistingConfigurationKeepsStartupPage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	if err := os.WriteFile(path, []byte(`{"preferences":{"startup_page":"favorites"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := library.Load(path)
	if err != nil || initialPage(s, options{}) != "favorites" || s.Preferences.DisableRoomPlaybackMemory || s.Preferences.DisableHomeRecommendationFilter {
		t.Fatal("升级改变了启动页或关闭了新功能", err)
	}
}
