//go:build windows

package main

import (
	"testing"
)

func TestCaptureScreen(t *testing.T) {
	cap, err := CaptureScreen()
	if err != nil {
		t.Fatalf("CaptureScreen 失败: %v", err)
	}
	if cap == nil || cap.Image == nil {
		t.Fatal("CaptureScreen 返回空图片")
	}
	b := cap.Image.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		t.Fatalf("截图尺寸无效: %d × %d", b.Dx(), b.Dy())
	}
	t.Logf("成功截取屏幕: 尺寸=%d×%d 坐标=(%d,%d)", b.Dx(), b.Dy(), cap.X, cap.Y)
}
