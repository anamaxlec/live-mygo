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
	ui.Column(c).FillWidth().Gap(settingsSectionGap).Children(func() {
		settingsGroup(c, "更新", func() {
			settingsRow(c, "更新通道", func() {
				channel := "正式版"
				if a.betaUpdates() {
					channel = "测试版"
				}
				if ui.Select(c, &channel, []string{"正式版", "测试版"}).Width(settingsControlWidth).Height(settingsControlHeight).FontSize(12).Label("更新通道").Disabled(a.updates.loading).Changed() && a.library != nil {
					next := a.library.Preferences
					next.UpdateChannel = "stable"
					if channel == "测试版" {
						next.UpdateChannel = "beta"
					}
					a.savePreferences(next)
					a.updates = releaseCheckState{}
				}
			})
			settingsDivider(c)
			settingsRow(c, "应用更新", func() {
				label := "检查更新"
				if a.updates.loading {
					label = "正在检查"
				}
				if settingsAction(c, label).Label("检查软件更新").Disabled(a.updates.loading).Clicked() {
					a.checkUpdates()
				}
			})
			if a.updates.checked {
				settingsDivider(c)
				if a.updates.failed || len(a.updates.releases) == 0 {
					settingsRow(c, "暂时无法获取版本信息", func() {
						if settingsAction(c, "版本页面").Label("打开版本页面").Clicked() {
							c.OpenURL(releasePage)
						}
					})
				} else {
					latest := a.updates.releases[0]
					label := "当前已是最新版本"
					if compareReleaseVersions(latest.Tag, appVersion) > 0 {
						label = "可更新至 " + strings.TrimPrefix(latest.Tag, "v")
					}
					previous := ""
					for _, release := range a.updates.releases {
						if compareReleaseVersions(release.Tag, appVersion) < 0 {
							previous = release.Tag
							break
						}
					}
					settingsRow(c, label, func() {
						if latest.Tag != previous && settingsAction(c, "版本详情").Label("查看版本 "+latest.Tag).Clicked() {
							c.OpenURL(releasePage + "/tag/" + url.PathEscape(latest.Tag))
						}
						if previous != "" && settingsAction(c, "上一版本").Label("查看上一版本 "+previous).Clicked() {
							c.OpenURL(releasePage + "/tag/" + url.PathEscape(previous))
						}
					})
				}
			}
		})
		settingsGroup(c, "诊断", func() {
			settingsRow(c, "播放诊断", func() {
				if settingsAction(c, "导出报告").Label("导出诊断报告").Disabled(a.diagnosticsExporting).Clicked() {
					a.exportDiagnostics()
				}
			})
			if runtime.GOOS == "darwin" {
				settingsDivider(c)
				settingsRow(c, "系统崩溃报告", func() {
					if settingsAction(c, "打开文件夹").Label("打开系统崩溃报告").Clicked() {
						a.openCrashReports()
					}
				})
			}
		})
		if a.settings.message != "" {
			ui.Text(c, a.settings.message).FontSize(11.5).TextColor(p.muted).MaxLines(2).PaddingX(15)
		}
	})
}
