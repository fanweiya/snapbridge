//go:build windows

package main

import (
	"golang.org/x/sys/windows/registry"
)

// Windows 11 的「使用 Print Screen 键打开屏幕截图」由这个值控制：
// 1 或缺失表示系统拿走这个键，0 表示交还给应用程序。
// ShareX 在安装器里把它写成 0，本程序把这步做成托盘菜单里的开关。
const (
	snippingRegPath  = `Control Panel\Keyboard`
	snippingRegValue = "PrintScreenKeyForSnippingEnabled"
)

// snippingKeyOwned 报告当前 Print Screen 键是否归属应用程序。
func snippingKeyOwned() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, snippingRegPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue(snippingRegValue)
	if err != nil {
		// 值不存在时 Windows 按默认处理，即系统接管这个键。
		return false
	}
	return v == 0
}

// setSnippingKeyOwned 把 Print Screen 键在系统截图与应用程序之间来回切换。
func setSnippingKeyOwned(own bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, snippingRegPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	v := uint32(1)
	if own {
		v = 0
	}
	return k.SetDWordValue(snippingRegValue, v)
}
