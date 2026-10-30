//go:build darwin || windows

package main

import "live-mygo/internal/app"

func main() {
	app.Run(appVersion)
}
