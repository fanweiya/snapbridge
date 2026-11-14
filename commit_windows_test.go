//go:build windows

package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func encodeTestPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("编码测试图片失败: %v", err)
	}
	return buf.Bytes()
}

func TestDecodePNGMetaAcceptsValidPNG(t *testing.T) {
	data := encodeTestPNG(t, 64, 32)
	w, h, err := decodePNGMeta(data)
	if err != nil {
		t.Fatalf("合法 PNG 被拒绝: %v", err)
	}
	if w != 64 || h != 32 {
		t.Fatalf("尺寸错误: 得到 %d×%d，期望 64×32", w, h)
	}
}

func TestDecodePNGMetaRejectsTruncatedPNG(t *testing.T) {
	data := encodeTestPNG(t, 64, 32)
	// 砍掉尾部 IEND 块，模拟传输截断。
	truncated := data[:len(data)-12]
	if _, _, err := decodePNGMeta(truncated); err == nil {
		t.Fatal("截断的 PNG 没有被拒绝")
	}
}

func TestDecodePNGMetaRejectsNonPNG(t *testing.T) {
	if _, _, err := decodePNGMeta([]byte("这不是图片")); err == nil {
		t.Fatal("非 PNG 数据没有被拒绝")
	}
	if _, _, err := decodePNGMeta(nil); err == nil {
		t.Fatal("空数据没有被拒绝")
	}
}
