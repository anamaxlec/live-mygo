//go:build darwin

package app

import "github.com/egoist/mygo/ui"

var fullscreenModifiers = ui.Cmd | ui.Ctrl

const fullscreenKey = ui.KeyF
