//go:build darwin || windows

package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"live-mygo/internal/library"
)

var backupFormats = []string{
	string(library.BackupNative), string(library.BackupSimple), string(library.BackupPure),
	string(library.BackupSimpleSettings), string(library.BackupSimpleFollows), string(library.BackupPureFollows),
}

func (a *app) backupSettings(c *ui.Context) {
	p := colors(c.Theme())
	settingsGroup(c, "数据", func() {
		settingsRow(c, "关注与偏好", func() {
			if settingsAction(c, "导入").Label("导入并合并备份").Tooltip("自动识别 Live MyGo、Simple Live 和纯粹直播").Disabled(a.backupBusy).Clicked() {
				a.importBackup()
			}
			if settingsAction(c, "导出").Label("导出备份").Tooltip("按所选格式导出，不含账号和历史").Disabled(a.backupBusy).Clicked() {
				a.exportBackup()
			}
		})
		if a.settings.backupFormat == "" {
			a.settings.backupFormat = string(library.BackupNative)
		}
		settingsRow(c, "导出格式", func() {
			ui.Select(c, &a.settings.backupFormat, backupFormats).Width(188).Height(settingsControlHeight).FontSize(12).Label("导出格式")
		})
		a.backupLANSettings(c)
		if a.settings.message != "" {
			ui.Text(c, a.settings.message).FontSize(11.5).TextColor(p.muted).MaxLines(3).Padding(0, 15, 12, 15)
		}
	})
}

func (a *app) finishBackup(message string) {
	a.backupBusy = false
	a.backupMessage(message)
}

func (a *app) backupMessage(message string) {
	if message != "" {
		a.settings.message = message
		a.toast = message
	}
	if a.accountWin != nil && !a.accountWin.IsDestroyed() {
		a.accountWin.Invalidate()
	}
}

func (a *app) backupDialogParent() *mygo.Window {
	if a.accountWin != nil && !a.accountWin.IsDestroyed() {
		return a.accountWin
	}
	return a.win
}

func (a *app) exportBackup() {
	if a.backupBusy || a.library == nil || a.win == nil {
		return
	}
	format := library.BackupFormat(a.settings.backupFormat)
	data, skipped, err := a.library.BackupAs(format)
	if err != nil {
		a.finishBackup("生成备份失败")
		return
	}
	parent := a.backupDialogParent()
	filename := format.Filename()
	ext := strings.TrimPrefix(filepath.Ext(filename), ".")
	a.backupBusy = true
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: parent, Title: "导出备份", DefaultPath: filename, Filters: []mygo.FileFilter{{Name: "直播备份", Extensions: []string{ext}}}})
		if err == nil && path != "" {
			if filepath.Ext(path) == "" {
				path += "." + ext
			}
			err = os.WriteFile(path, append(data, '\n'), 0600)
		}
		a.win.Update(func() {
			if a.closing {
				return
			}
			message := ""
			if err != nil {
				message = "导出备份失败"
			} else if path != "" {
				message = "已导出备份"
				if skipped > 0 {
					message += fmt.Sprintf("，跳过目标应用不支持的 %d 个关注", skipped)
				}
			}
			a.finishBackup(message)
		})
	}()
}

var errBackupSize = errors.New("备份文件超过 8 MB")

func readBackupFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, library.BackupMaxBytes+1))
	if len(data) > library.BackupMaxBytes {
		return nil, errBackupSize
	}
	return data, err
}

func (a *app) importBackup() {
	if a.backupBusy || a.library == nil || a.win == nil {
		return
	}
	parent := a.backupDialogParent()
	a.backupBusy = true
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: parent, Title: "导入并合并备份", Filters: []mygo.FileFilter{{Name: "直播备份", Extensions: []string{"json", "txt"}}}})
		var data []byte
		if err == nil && len(paths) > 0 {
			data, err = readBackupFile(paths[0])
		}
		a.win.Update(func() {
			if a.closing {
				return
			}
			switch {
			case errors.Is(err, errBackupSize):
				a.finishBackup(err.Error())
			case err != nil:
				a.finishBackup("读取备份失败")
			case len(paths) == 0:
				a.finishBackup("")
			default:
				a.applyBackup(data)
			}
		})
	}()
}

func (a *app) applyBackup(data []byte) {
	before := a.library.Preferences
	report, err := a.library.ImportBackupWithReport(data)
	if err != nil {
		a.finishBackup(err.Error())
		return
	}
	next := a.library.Preferences
	a.applySavedPreferences(before, next)
	if before.Volume != next.Volume {
		a.volume = next.Volume
		a.command("set", "volume", strconv.FormatFloat(a.volume, 'f', 0, 64))
	}
	if report.Added+report.Existing > 0 {
		a.favoriteGroup = ""
		a.grid = ui.ListState{}
		a.refreshFavorites(true)
	}
	message := fmt.Sprintf("已导入 %s，新增关注 %d 个，已有 %d 个，偏好 %d 项", report.Format, report.Added, report.Existing, report.Settings)
	if report.Skipped > 0 {
		message += fmt.Sprintf("，跳过 %d 个不支持或无效的关注", report.Skipped)
	}
	a.finishBackup(message)
}

func (a *app) backupLANSettings(c *ui.Context) {
	p := colors(c.Theme())
	if a.settings.backupLAN == nil && len(a.settings.backupLANAddresses) > 1 {
		if !slices.Contains(a.settings.backupLANAddresses, a.settings.backupLANAddress) {
			a.settings.backupLANAddress = a.settings.backupLANAddresses[0]
		}
		settingsRow(c, "局域网地址", func() {
			ui.Select(c, &a.settings.backupLANAddress, a.settings.backupLANAddresses).Width(188).Height(settingsControlHeight).FontSize(12).Label("局域网地址")
		})
	}
	settingsRow(c, "局域网互传", func() {
		if transfer := a.settings.backupLAN; transfer != nil {
			if settingsAction(c, "复制链接").Tooltip(transfer.URL).Clicked() {
				mygo.Clipboard.WriteText(transfer.URL)
				a.backupMessage("已复制互传链接")
			}
			if settingsAction(c, "结束").Clicked() {
				a.stopBackupLAN()
			}
		} else if settingsAction(c, "开启").Tooltip("在同一局域网的浏览器收发备份，链接有效 5 分钟").Disabled(a.backupBusy || len(a.settings.pendingLANBackup) > 0).Clicked() {
			a.startBackupLAN()
		}
	})
	if transfer := a.settings.backupLAN; transfer != nil {
		ui.Text(c, transfer.URL).FontSize(11.5).TextColor(p.muted).MaxLines(2).Padding(0, 15, 12, 15)
	}
	if len(a.settings.pendingLANBackup) > 0 {
		settingsRow(c, "已收到备份", func() {
			if settingsAction(c, "忽略").Clicked() {
				a.settings.pendingLANBackup = nil
				a.settings.message = ""
			}
			if settingsAction(c, "确认导入").Disabled(a.backupBusy).Clicked() {
				data := a.settings.pendingLANBackup
				a.settings.pendingLANBackup = nil
				a.applyBackup(data)
			}
		})
	}
}

func (a *app) startBackupLAN() {
	if a.library == nil || a.win == nil || a.settings.backupLAN != nil {
		return
	}
	addresses := library.BackupLANAddresses()
	a.settings.backupLANAddresses = addresses
	if len(addresses) == 0 {
		a.backupMessage("没有可用的局域网地址")
		return
	}
	if !slices.Contains(addresses, a.settings.backupLANAddress) {
		a.settings.backupLANAddress = addresses[0]
	}
	format := library.BackupFormat(a.settings.backupFormat)
	data, skipped, err := a.library.BackupAs(format)
	if err != nil {
		a.backupMessage("生成备份失败")
		return
	}
	transfer, err := library.StartBackupTransfer(a.ctx, a.settings.backupLANAddress, format.Filename(), data)
	if err != nil {
		a.backupMessage("无法开启局域网互传，请检查网络或防火墙")
		return
	}
	a.settings.backupLAN = transfer
	message := "用同一局域网的浏览器打开链接，5 分钟内有效"
	if skipped > 0 {
		message += fmt.Sprintf("。下载的备份跳过 %d 个不兼容的关注", skipped)
	}
	a.backupMessage(message)
	go func() {
		for {
			select {
			case data := <-transfer.Uploads:
				a.win.Update(func() {
					if !a.closing && a.settings.backupLAN == transfer {
						a.settings.pendingLANBackup = data
						a.backupMessage("已收到备份，请确认导入")
					}
				})
			case <-transfer.Done:
				a.win.Update(func() {
					if !a.closing && a.settings.backupLAN == transfer {
						a.settings.backupLAN = nil
						a.backupMessage("局域网互传已结束")
					}
				})
				return
			}
		}
	}()
}

func (a *app) stopBackupLAN() {
	if transfer := a.settings.backupLAN; transfer != nil {
		a.settings.backupLAN = nil
		transfer.Close()
		a.settings.message = ""
	}
}
