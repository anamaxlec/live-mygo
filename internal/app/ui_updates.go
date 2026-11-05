//go:build darwin || windows

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

const releasePage = "https://github.com/anamaxlec/live-mygo/releases"
const releaseAPI = "https://api.github.com/repos/anamaxlec/live-mygo/releases?per_page=30"

type appRelease struct {
	Tag        string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

type releaseCheckState struct {
	loading  bool
	checked  bool
	failed   bool
	releases []appRelease
}

var releaseVersionPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$`)

func compareReleaseVersions(a, b string) int {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	x, y := releaseVersionPattern.FindStringSubmatch(a), releaseVersionPattern.FindStringSubmatch(b)
	if x == nil || y == nil {
		return 0
	}
	for i := 1; i <= 3; i++ {
		left, _ := strconv.ParseUint(x[i], 10, 64)
		right, _ := strconv.ParseUint(y[i], 10, 64)
		if left < right {
			return -1
		}
		if left > right {
			return 1
		}
	}
	if x[4] == y[4] {
		return 0
	}
	if x[4] == "" {
		return 1
	}
	if y[4] == "" {
		return -1
	}
	xp, yp := strings.Split(x[4], "."), strings.Split(y[4], ".")
	for i := 0; i < min(len(xp), len(yp)); i++ {
		if xp[i] == yp[i] {
			continue
		}
		xn, xe := strconv.ParseUint(xp[i], 10, 64)
		yn, ye := strconv.ParseUint(yp[i], 10, 64)
		if xe == nil && ye == nil {
			if xn < yn {
				return -1
			}
			return 1
		}
		if xe == nil {
			return -1
		}
		if ye == nil {
			return 1
		}
		if xp[i] < yp[i] {
			return -1
		}
		return 1
	}
	if len(xp) < len(yp) {
		return -1
	}
	return 1
}

func fetchAppReleases(ctx context.Context, client *http.Client, endpoint string, beta bool) ([]appRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("版本服务返回 %d", resp.StatusCode)
	}
	var all []appRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&all); err != nil {
		return nil, err
	}
	var releases []appRelease
	for _, release := range all {
		parts := releaseVersionPattern.FindStringSubmatch(release.Tag)
		if parts == nil || release.Draft || !beta && (release.Prerelease || parts[4] != "") {
			continue
		}
		releases = append(releases, release)
	}
	sort.SliceStable(releases, func(i, j int) bool { return compareReleaseVersions(releases[i].Tag, releases[j].Tag) > 0 })
	return releases, nil
}

func (a *app) betaUpdates() bool {
	if a.library != nil && a.library.Preferences.UpdateChannel != "" {
		return a.library.Preferences.UpdateChannel == "beta"
	}
	return strings.Contains(appVersion, "-")
}

func (a *app) checkUpdates() {
	if a.updates.loading || a.win == nil {
		return
	}
	a.updates = releaseCheckState{loading: true}
	beta := a.betaUpdates()
	go func() {
		ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
		defer cancel()
		releases, err := fetchAppReleases(ctx, &http.Client{Timeout: 15 * time.Second}, releaseAPI, beta)
		a.win.Update(func() {
			if a.closing {
				return
			}
			a.updates = releaseCheckState{checked: true, failed: err != nil, releases: releases}
			if a.accountWin != nil && !a.accountWin.IsDestroyed() {
				a.accountWin.Invalidate()
			}
		})
	}()
}

func (a *app) updateControls(c *ui.Context) {
	p := colors(c.Theme())
	ui.Row(c).FillWidth().Gap(8).Children(func() {
		label := "检查更新"
		if a.updates.loading {
			label = "正在检查"
		}
		if ui.Button(c, label).Grow(1).Basis(0).Height(30).FontSize(12).Disabled(a.updates.loading).Clicked() {
			a.checkUpdates()
		}
		if ui.Button(c, "导出诊断报告").Grow(1).Basis(0).Height(30).FontSize(12).Disabled(a.diagnosticsExporting).Clicked() {
			a.exportDiagnostics()
		}
	})
	ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
		ui.Text(c, "更新通道").FontSize(11.5).TextColor(p.muted).Grow(1)
		channel := "正式版"
		if a.betaUpdates() {
			channel = "测试版"
		}
		if ui.Select(c, &channel, []string{"正式版", "测试版"}).Width(116).Height(28).FontSize(11.5).Label("更新通道").Disabled(a.updates.loading).Changed() && a.library != nil {
			next := a.library.Preferences
			next.UpdateChannel = "stable"
			if channel == "测试版" {
				next.UpdateChannel = "beta"
			}
			a.savePreferences(next)
			a.updates = releaseCheckState{}
		}
	})
	if a.settings.message != "" {
		ui.Text(c, a.settings.message).FontSize(11.5).TextColor(p.muted).MaxLines(2)
	}
	if runtime.GOOS == "darwin" && ui.Button(c, "打开系统崩溃报告").FillWidth().Height(28).FontSize(11.5).Clicked() {
		a.openCrashReports()
	}
	if !a.updates.checked {
		return
	}
	if a.updates.failed || len(a.updates.releases) == 0 {
		ui.Text(c, "暂时没有获取到版本信息").FontSize(11.5).TextColor(p.muted)
		if ui.Button(c, "打开版本页面").FillWidth().Height(30).Clicked() {
			c.OpenURL(releasePage)
		}
		return
	}
	latest := a.updates.releases[0]
	label := "当前已是最新版本"
	if compareReleaseVersions(latest.Tag, appVersion) > 0 {
		label = "可更新至 " + strings.TrimPrefix(latest.Tag, "v")
	}
	ui.Text(c, label).FontSize(11.5).TextColor(p.muted)
	ui.Row(c).FillWidth().Gap(8).Children(func() {
		if ui.Button(c, "查看版本 "+strings.TrimPrefix(latest.Tag, "v")).Grow(1).Basis(0).Height(30).FontSize(11.5).Clicked() {
			c.OpenURL(releasePage + "/tag/" + url.PathEscape(latest.Tag))
		}
		for _, release := range a.updates.releases {
			if compareReleaseVersions(release.Tag, appVersion) < 0 {
				if ui.Button(c, "上一版 "+strings.TrimPrefix(release.Tag, "v")).Grow(1).Basis(0).Height(30).FontSize(11.5).Clicked() {
					c.OpenURL(releasePage + "/tag/" + url.PathEscape(release.Tag))
				}
				break
			}
		}
	})
}
