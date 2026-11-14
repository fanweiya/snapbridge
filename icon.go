package main

import _ "embed"

//go:embed resources/tray-32.png
var trayIconData []byte

//go:embed resources/icon.png
var appIconData []byte

// trayIconPNG 返回托盘图标的 PNG 字节。
func trayIconPNG() []byte { return trayIconData }

// appIconPNG 返回应用图标的 PNG 字节，用于更新窗口。
func appIconPNG() []byte { return appIconData }
