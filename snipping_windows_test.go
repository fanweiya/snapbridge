//go:build windows

package main

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

// 读写 Print Screen 归属开关，并还原用户原来的设置。
func TestSnippingKeyOwnership(t *testing.T) {
	const keyPath = `Control Panel\Keyboard`
	const valueName = "PrintScreenKeyForSnippingEnabled"

	// 记录原始状态：0/1/不存在三种可能都要还原成原样。
	k, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("打开注册表项失败: %v", err)
	}
	original, _, originalErr := k.GetIntegerValue(valueName)
	k.Close()

	defer func() {
		rk, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.SET_VALUE)
		if err != nil {
			t.Fatalf("还原时打开注册表项失败: %v", err)
		}
		defer rk.Close()
		if originalErr != nil {
			if err := rk.DeleteValue(valueName); err != nil {
				t.Fatalf("还原时删除值失败: %v", err)
			}
			return
		}
		if err := rk.SetDWordValue(valueName, uint32(original)); err != nil {
			t.Fatalf("还原时写值失败: %v", err)
		}
	}()

	if err := setSnippingKeyOwned(true); err != nil {
		t.Fatalf("写入接管状态失败: %v", err)
	}
	if !snippingKeyOwned() {
		t.Fatalf("写入接管后读到的仍是未接管")
	}

	if err := setSnippingKeyOwned(false); err != nil {
		t.Fatalf("写入归还状态失败: %v", err)
	}
	if snippingKeyOwned() {
		t.Fatalf("写入归还后读到的仍是接管")
	}
}
