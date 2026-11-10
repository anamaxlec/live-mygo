//go:build darwin || windows

package app

import (
	"strconv"

	"github.com/egoist/mygo/ui"
	"live-mygo/internal/library"
	"live-mygo/internal/platforms"
)

func initialPage(state *library.State, opts options) string {
	if opts.platform == string(platforms.FLive) {
		return opts.platform
	}
	if opts.platform != "" {
		return startupPage(opts.platform)
	}
	if state == nil {
		return "home"
	}
	if !state.WelcomeCompleted && opts.room == "" && opts.file == "" {
		return "welcome"
	}
	return startupPage(state.Preferences.StartupPage)
}

func (a *app) finishWelcome() {
	if err := a.library.CompleteWelcome(); err != nil {
		a.toast = "未能保存首次启动状态"
	}
	a.openPage("home")
}

func (a *app) welcomeView(c *ui.Context) {
	p := colors(c.Theme())
	c.OnShortcut(ui.Cmd, ui.KeyComma, a.accountsWindow)
	c.OnShortcut(ui.Cmd, ui.KeyW, func() { a.win.Close() })
	c.OnShortcut(0, ui.KeyEscape, a.finishWelcome)
	ui.Column(c.Key("welcome")).Fill().Background(p.bg).Children(func() {
		ui.Box(c).FillWidth().Height(watchHeaderHeight).Shrink(0).DragWindow()
		ui.Box(c).Grow(1).MinHeight(0).Padding(0, 36, 38, 36).Center().Children(func() {
			ui.Row(c).Width(844).MaxWidthPercent(100).Gap(42).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Width(286).Shrink(0).Gap(14).Children(func() {
					playfulMascot(c)
					ui.Text(c, "欢迎使用 Live MyGo").FontSize(28).FontWeight(650).TextColor(p.text)
					ui.Text(c, "登录对应平台后，可以使用账号支持的画质。").FontSize(13).LineHeight(1.5).TextColor(p.muted)
				})
				ui.Column(c).Grow(1).MinWidth(0).Gap(14).Children(func() {
					ui.Text(c, "登录平台账号").FontSize(17).FontWeight(600).TextColor(p.text)
					ui.Column(c).FillWidth().Radius(12).Background(p.soft).Padding(15, 16).Gap(14).Children(func() {
						for i, step := range [][2]string{
							{"选择平台", "打开账号设置，在对应平台一行点「登录」。"},
							{"在官网登录", "按官网提示完成登录，然后留在登录窗口。"},
							{"保存登录状态", "点右上角「完成并保存」，看到「Cookie 已保存」即可。"},
						} {
							ui.Row(c.Key(i)).FillWidth().Gap(10).AlignItems(ui.Start).Children(func() {
								ui.Box(c).Size(23, 23).Shrink(0).Radius(7).Background(p.bg).Center().Children(func() {
									ui.Text(c, strconv.Itoa(i+1)).FontSize(11).FontWeight(650).TextColor(p.accent)
								})
								ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
									ui.Text(c, step[0]).FontSize(12.5).FontWeight(600).TextColor(p.text)
									ui.Text(c, step[1]).FontSize(12).LineHeight(1.4).TextColor(p.muted)
								})
							})
						}
					})
					ui.Column(c).FillWidth().Gap(4).Children(func() {
						ui.Text(c, "Cookie 加密保存在本机，只随对应平台的请求发送。").FontSize(11.5).TextColor(p.muted)
						ui.Text(c, "已有 Cookie 可在账号设置里直接导入。").FontSize(11.5).TextColor(p.muted)
					})
					ui.Row(c).FillWidth().Gap(10).Children(func() {
						if ui.PrimaryButton(c, "打开账号设置").Label("打开账号设置").Grow(1).Basis(0).Height(34).Radius(7).Border(1, p.accent).Clicked() {
							a.accountsWindow()
						}
						if ui.Button(c, "进入首页").Label("进入首页").Grow(1).Basis(0).Height(34).Radius(7).Clicked() {
							a.finishWelcome()
						}
					})
				})
			})
		})
	})
}
