//go:build darwin || windows

package app

import (
	"context"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"live-mygo/internal/accounts"
	"live-mygo/internal/platforms"
)

func (a *app) loginWindow(platform platforms.Platform) {
	session := mygo.NewEphemeralSession()
	var web *mygo.WebView
	var win *mygo.Window
	message := "在官方页面完成登录后保存"
	saving := false
	win = mygo.NewWindow(mygo.WindowOptions{Title: platform.Name() + "登录 · Live MyGo", Width: 1000, Height: 760, MinWidth: 760, MinHeight: 560, TitleBarStyle: mygo.TitleBarHidden, BackgroundColor: "light-dark(#ffffff, #171719)", Parent: a.accountWin, Content: ui.View(func(c *ui.Context) {
		a.applyTheme(c)
		c.OnShortcut(ui.Cmd, ui.KeyW, func() { win.Close() })
		p := colors(c.Theme())
		bar := c.TitleBar()
		ui.Column(c).Fill().Background(p.bg).Children(func() {
			ui.Row(c).Height(48).Padding(0, titlebarRight(c, 16), 0, max(bar.Left+10, float32(16))).Gap(8).AlignItems(ui.Center).DragWindow().Children(func() {
				if web != nil {
					if iconButton(c.Key("back"), iconBack, "返回").Disabled(!web.Page().CanGoBack()).Clicked() {
						web.Page().GoBack()
					}
					if iconButton(c.Key("reload"), iconRefresh, "重新加载").Clicked() {
						web.Page().Reload()
					}
				}
				ui.Text(c, platform.Name()).FontSize(13).FontWeight(600).TextColor(p.text)
				ui.Box(c).Grow(1)
				ui.Text(c, message).FontSize(11.5).TextColor(p.muted).SingleLine().MaxWidth(390)
				if ui.PrimaryButton(c, "完成并保存").Height(30).FontSize(12).Disabled(web == nil || saving).Clicked() {
					saving, message = true, "正在保存登录"
					go func() {
						cookies, err := session.Cookies.List(mygo.CookieFilter{Domain: platform.Domain()})
						var saved []accounts.Cookie
						for _, v := range cookies {
							saved = append(saved, accounts.Cookie{Name: v.Name, Value: v.Value, Domain: v.Domain, Path: v.Path, Expires: v.Expires, Secure: v.Secure, HTTPOnly: v.HTTPOnly})
						}
						if err == nil && platform == platforms.Qie && !accounts.HasLogin(platform, saved) {
							ctx, cancel := context.WithTimeout(a.ctx, 4*time.Second)
							var value any
							value, err = web.Page().EvalContext(ctx, qieLoginStorageScript)
							cancel()
							if data, ok := value.(map[string]any); ok {
								for _, name := range []string{"token", "tokenId", "uid"} {
									if value, ok := data[name].(string); ok && value != "" {
										saved = append(saved, accounts.Cookie{Name: name, Value: value, Domain: ".qie.tv", Path: "/", Secure: true, HTTPOnly: true})
									}
								}
							}
						}
						if err == nil {
							err = a.vault.Save(platform, saved)
						}
						win.Update(func() {
							saving = false
							if err != nil {
								message = err.Error()
								return
							}
							a.refreshAccountStatus()
							if a.accountWin != nil && !a.accountWin.IsDestroyed() {
								a.accountWin.Invalidate()
							}
							a.win.Invalidate()
							win.Close()
						})
					}()
				}
			})
			ui.Box(c).Height(1).Background(p.line)
			if web != nil {
				ui.WebView(c, web).Grow(1)
			} else {
				ui.Column(c).Grow(1).Center().Children(func() { ui.Spinner(c).Size(18, 18) })
			}
		})
	})})
	var err error
	web, err = win.NewWebView(mygo.WebViewOptions{URL: platform.LoginURL(), Page: mygo.PageOptions{Session: session, Untrusted: true}})
	if err != nil {
		message = "登录页面打开失败"
	}
	win.Invalidate()
}

// 新官网将 token 放在自己的 localStorage 中，读取后沿用本机加密账号存储。
const qieLoginStorageScript = `
if (location.protocol !== "https:" || !["www.qie.tv", "qie.tv"].includes(location.hostname)) return null;
const account = JSON.parse(localStorage.getItem("QieNext::") || "{}");
return {token: String(account.token || ""), tokenId: String(account.tokenId || ""), uid: String(account.uid || "")};`
