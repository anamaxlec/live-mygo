//go:build darwin || windows

package main

import _ "embed"

//go:embed VERSION
var appVersion string
