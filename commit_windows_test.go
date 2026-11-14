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

func TestValidatePNGAcceptsValidPNG(t *testing.T) {
	data := encodeTestPNG(t, 64, 32)
	w, h, err := validatePNG(data)
	if err != nil {
		t.Fatalf("合法 PNG 被拒绝: %v", err)
	}
	if w != 64 || h != 32 {
		t.Fatalf("尺寸错误: 得到 %d×%d，期望 64×32", w, h)
	}
}

func TestValidatePNGRejectsTruncatedPNG(t *testing.T) {
	data := encodeTestPNG(t, 64, 32)
	// 砍掉尾部，模拟传输截断。
	truncated := data[:len(data)-16]
	if _, _, err := validatePNG(truncated); err == nil {
		t.Fatal("截断的 PNG 没有被拒绝")
	}
}

func TestValidatePNGRejectsCorruptedIDATWithValidIEND(t *testing.T) {
	data := encodeTestPNG(t, 64, 32)
	// 找到 "IDAT" 关键字的位置，确保 IHDR 块与其 CRC（前 33 字节）保持完全完好。
	idatIdx := bytes.Index(data, []byte("IDAT"))
	if idatIdx == -1 {
		t.Fatal("未在测试图片中找到 IDAT 块")
	}

	corrupted := make([]byte, len(data))
	copy(corrupted, data)

	// 从 IDAT 数据部分开始翻转字节，但保留尾部 12 字节的合法 IEND 块。
	startCorrupt := idatIdx + 4
	endCorrupt := len(corrupted) - 16
	for i := startCorrupt; i < endCorrupt; i++ {
		corrupted[i] = 0x5A
	}

	// 显式核实：纯 DecodeConfig 仅读 IHDR，会被假象蒙骗判定通过。
	if _, err := png.DecodeConfig(bytes.NewReader(corrupted)); err != nil {
		t.Fatalf("测试构造错误：IHDR 不应受损，但 DecodeConfig 报错: %v", err)
	}

	// validatePNG 执行完整解码，必须准确发现 IDAT 损坏并拦截。
	if _, _, err := validatePNG(corrupted); err == nil {
		t.Fatal("中间 IDAT 损坏但包含合法 IHDR 与 IEND 的 PNG 未被拦截")
	}
}

func TestValidatePNGRejectsNonPNG(t *testing.T) {
	if _, _, err := validatePNG([]byte("这不是图片")); err == nil {
		t.Fatal("非 PNG 数据没有被拒绝")
	}
	if _, _, err := validatePNG(nil); err == nil {
		t.Fatal("空数据没有被拒绝")
	}
}
