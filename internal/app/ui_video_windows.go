//go:build windows && (amd64 || arm64)

package app

func (a *app) captureReturnFrame() bool { return false }
