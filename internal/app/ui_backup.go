//go:build darwin || windows

package app

import (
	"errors"
	"io"
	"os"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *app) backupSettings(c *ui.Context) {
	p := colors(c.Theme())
	settingsGroup(c, "数据", func() {
		settingsRow(c, "关注与偏好", func() {
			if ui.Button(c, "导入").Width(76).Height(28).FontSize(12).Label("导入并合并备份").Tooltip("合并关注并导入偏好").Disabled(a.backupBusy).Clicked() {
				a.importBackup()
			}
			if ui.Button(c, "导出").Width(76).Height(28).FontSize(12).Label("导出备份").Tooltip("导出关注与偏好，不含账号和历史").Disabled(a.backupBusy).Clicked() {
				a.exportBackup()
			}
		})
		if a.settings.message != "" {
			ui.Text(c, a.settings.message).FontSize(11.5).TextColor(p.muted).MaxLines(2).Padding(0, 15, 12, 15)
		}
	})
}

func (a *app) finishBackup(message string) {
	a.backupBusy = false
	if message != "" {
		a.settings.message = message
		a.toast = message
	}
	if a.accountWin != nil && !a.accountWin.IsDestroyed() {
		a.accountWin.Invalidate()
	}
}

func (a *app) exportBackup() {
	if a.backupBusy || a.library == nil || a.win == nil {
		return
	}
	data, err := a.library.Backup()
	if err != nil {
		a.settings.message = "生成备份失败"
		return
	}
	parent := a.win
	if a.accountWin != nil && !a.accountWin.IsDestroyed() {
		parent = a.accountWin
	}
	a.backupBusy = true
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: parent, Title: "导出备份", DefaultPath: "LiveMyGo-backup-" + time.Now().Format("20060102") + ".json", Filters: []mygo.FileFilter{{Name: "JSON", Extensions: []string{"json"}}}})
		if err == nil && path != "" {
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
			}
			a.finishBackup(message)
		})
	}()
}

func (a *app) importBackup() {
	if a.backupBusy || a.library == nil || a.win == nil {
		return
	}
	parent := a.win
	if a.accountWin != nil && !a.accountWin.IsDestroyed() {
		parent = a.accountWin
	}
	a.backupBusy = true
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: parent, Title: "导入并合并备份", Filters: []mygo.FileFilter{{Name: "JSON", Extensions: []string{"json"}}}})
		var data []byte
		if err == nil && len(paths) > 0 {
			var file *os.File
			file, err = os.Open(paths[0])
			if err == nil {
				data, err = io.ReadAll(io.LimitReader(file, (8<<20)+1))
				file.Close()
				if len(data) > 8<<20 {
					err = errors.New("备份文件超过 8 MB")
				}
			}
		}
		a.win.Update(func() {
			if a.closing {
				a.backupBusy = false
				return
			}
			if err != nil {
				a.finishBackup("读取备份失败")
				return
			}
			if len(paths) == 0 {
				a.finishBackup("")
				return
			}
			before := a.library.Preferences
			if err := a.library.ImportBackup(data); err != nil {
				a.finishBackup(err.Error())
				return
			}
			a.applySavedPreferences(before, a.library.Preferences)
			a.volume = a.library.Preferences.Volume
			if a.player != nil {
				a.setVolume()
			}
			a.favoriteGroup = ""
			a.grid = ui.ListState{}
			a.refreshFavorites(true)
			a.finishBackup("已合并关注并导入偏好设置")
		})
	}()
}
