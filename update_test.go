package main

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

func testContext(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// TestManifestURL 走真实网络取最新发布里的清单地址。
func TestManifestURL(t *testing.T) {
	url, err := manifestURL(testContext(t))
	if err != nil {
		t.Fatalf("manifestURL 失败: %v", err)
	}
	if !strings.HasSuffix(url, "update-windows-amd64.json") {
		t.Errorf("清单地址不对: %s", url)
	}
	if !strings.Contains(url, updateRepo) {
		t.Errorf("清单地址不在本仓库: %s", url)
	}
	t.Logf("清单地址: %s", url)
}

// TestManifestParses 取回清单并确认字段完整，签名必须是 base64 的 Ed25519。
func TestManifestParses(t *testing.T) {
	body, err := (githubSource{}).Manifest(testContext(t), false)
	if err != nil {
		t.Fatalf("取清单失败: %v", err)
	}
	defer body.Close()

	raw, err := io.ReadAll(io.LimitReader(body, 1<<20))
	if err != nil {
		t.Fatalf("读清单失败: %v", err)
	}
	var m struct {
		Version   string `json:"version"`
		URL       string `json:"url"`
		Size      int64  `json:"size"`
		Signature string `json:"signature"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("清单不是合法 JSON: %v\n%s", err, raw)
	}
	if m.Version == "" || m.URL == "" || m.Signature == "" {
		t.Fatalf("清单字段不完整: %s", raw)
	}
	if m.Size <= 0 {
		t.Errorf("归档大小不对: %d", m.Size)
	}
	if len(m.Signature) != 88 {
		t.Errorf("签名长度应为 88（base64 的 Ed25519），实际 %d", len(m.Signature))
	}
	t.Logf("最新版本 %s，归档 %d 字节", m.Version, m.Size)
}

// TestOpenThroughFallsBack 前一个通道不通时要用下一个把同一个文件取回来。
func TestOpenThroughFallsBack(t *testing.T) {
	url, err := manifestURL(testContext(t))
	if err != nil {
		t.Fatalf("manifestURL 失败: %v", err)
	}
	prefixes := []string{"https://snapshot.invalid.", "https://also-broken.invalid/", ""}
	body, err := openThrough(testContext(t), prefixes, url)
	if err != nil {
		t.Fatalf("回退后仍然失败: %v", err)
	}
	defer body.Close()
	raw, err := io.ReadAll(io.LimitReader(body, 1<<20))
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if !json.Valid(raw) {
		t.Fatalf("回退取回的内容不是 JSON: %.200s", raw)
	}
}

// TestOpenThroughReportsEveryChannel 全部通道失败时报错要说清试过哪些。
func TestOpenThroughReportsEveryChannel(t *testing.T) {
	prefixes := []string{"https://one.invalid.", "https://two.invalid./"}
	_, err := openThrough(testContext(t), prefixes, "https://example.invalid/x.json")
	if err == nil {
		t.Fatal("全部失败时应该报错")
	}
	for _, want := range []string{"one.invalid", "two.invalid"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("错误信息里缺少 %s: %v", want, err)
		}
	}
}
