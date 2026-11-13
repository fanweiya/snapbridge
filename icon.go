package main

import _ "embed"

//go:embed resources/tray-32.png
var trayIconData []byte

// trayIconPNG 返回托盘图标的 PNG 字节。
func trayIconPNG() []byte { return trayIconData }
