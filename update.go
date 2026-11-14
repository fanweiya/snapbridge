package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

// 国内直连 GitHub 经常在 TLS 握手或传输中途断掉，
// 这里按实测可用性排好序，空串代表直连，前面失败就换下一个。
var downloadMirrors = []string{
	"",
	"https://ghfast.top/",
	"https://gh-proxy.com/",
	"https://ghproxy.net/",
	"https://ghproxy.cfd/",
	"https://gh-proxy.net/",
}

// 能转发 api.github.com 请求的加速站点。
// ghfast.top 与 ghproxy.net 只处理 release 与 raw 路径，对 API 返回 403，不列入。
var apiMirrors = []string{
	"",
	"https://gh-proxy.com/",
	"https://ghproxy.cfd/",
	"https://gh-proxy.net/",
}

const (
	updateRepo   = "fanweiya/snapbridge"
	updateTarget = "windows-amd64"
)

var updateHTTP = &http.Client{Timeout: 30 * time.Minute}

// openThrough 依次用各前缀拼接原始地址发起 GET，返回第一个成功的响应体。
func openThrough(ctx context.Context, prefixes []string, raw string) (io.ReadCloser, error) {
	var lastErr error
	for _, prefix := range prefixes {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, prefix+raw, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("User-Agent", "SnapBridge-Updater")
		resp, err := updateHTTP.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", mirrorName(prefix), err)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("%s: %s", mirrorName(prefix), resp.Status)
			continue
		}
		return resp.Body, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("没有可用的下载通道")
	}
	return nil, lastErr
}

func mirrorName(prefix string) string {
	if prefix == "" {
		return "直连"
	}
	return strings.TrimSuffix(strings.TrimPrefix(prefix, "https://"), "/")
}

type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type releaseInfo struct {
	Tag    string         `json:"tag_name"`
	Assets []releaseAsset `json:"assets"`
}

// manifestURL 通过 GitHub API 找到最新发布里的更新清单地址。
func manifestURL(ctx context.Context) (string, error) {
	api := "https://api.github.com/repos/" + updateRepo + "/releases/latest"
	body, err := openThrough(ctx, apiMirrors, api)
	if err != nil {
		return "", err
	}
	defer body.Close()

	var rel releaseInfo
	if err := json.NewDecoder(body).Decode(&rel); err != nil {
		return "", fmt.Errorf("解析发布信息失败: %w", err)
	}
	if len(rel.Assets) == 0 {
		return "", fmt.Errorf("发布 %s 里没有可下载的文件", rel.Tag)
	}
	want := "update-" + updateTarget + ".json"
	for _, a := range rel.Assets {
		if a.Name == want {
			return a.URL, nil
		}
	}
	return "", fmt.Errorf("发布 %s 缺少 %s，这个版本还不支持自动更新", rel.Tag, want)
}

// githubSource 把 mygo.Updater 的请求导向带镜像回退的 GitHub 下载。
type githubSource struct{}

func (githubSource) Manifest(ctx context.Context, _ bool) (io.ReadCloser, error) {
	url, err := manifestURL(ctx)
	if err != nil {
		return nil, err
	}
	return openThrough(ctx, downloadMirrors, url)
}

func (githubSource) Open(ctx context.Context, url string) (io.ReadCloser, error) {
	return openThrough(ctx, downloadMirrors, url)
}

// useMirroredUpdates 让更新检查与下载都走镜像回退，必须在第一次检查之前调用。
func useMirroredUpdates() {
	mygo.Updater.SetSource(githubSource{})
}
