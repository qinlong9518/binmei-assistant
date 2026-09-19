//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// UpdateMeta desktop_update.json 结构
type UpdateMeta struct {
	VersionCode int    `json:"versionCode"`
	VersionName string `json:"versionName"`
	Changelog   string `json:"changelog"`
	ExeURL      string `json:"exeUrl"`
	Force       bool   `json:"force"`
}

var updateHTTP = &http.Client{Timeout: 30 * time.Second}

// fetchUpdateMeta 多源拉取更新元数据
func fetchUpdateMeta() (*UpdateMeta, error) {
	for _, src := range metaSources {
		u := fmt.Sprintf("%s?_t=%d", src, time.Now().UnixMilli())
		resp, err := updateHTTP.Get(u)
		if err != nil {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if err != nil || resp.StatusCode != 200 {
			continue
		}
		var m UpdateMeta
		if json.Unmarshal(body, &m) == nil && m.VersionCode > 0 {
			return &m, nil
		}
	}
	return nil, fmt.Errorf("所有更新源均不可达")
}

// downloadExe 镜像优先下载新 exe
func downloadExe(url string, progress func(p int)) (string, error) {
	tmp := os.TempDir() + "\\binmei-desktop-update.exe"
	var lastErr error
	for _, m := range downloadMirrors {
		u := fmt.Sprintf(m, url)
		done, err := downloadTo(u, tmp, progress)
		if done {
			return tmp, nil
		}
		lastErr = err
	}
	return "", lastErr
}

func downloadTo(url, dst string, progress func(int)) (bool, error) {
	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	total := resp.ContentLength
	f, err := os.Create(dst)
	if err != nil {
		return false, err
	}
	defer f.Close()
	var n int64
	buf := make([]byte, 64*1024)
	for {
		nr, er := resp.Body.Read(buf)
		if nr > 0 {
			f.Write(buf[:nr])
			n += int64(nr)
			if total > 0 && progress != nil {
				progress(int(n * 100 / total))
			}
		}
		if er != nil {
			if er == io.EOF {
				return true, nil
			}
			return false, er
		}
	}
}

// selfReplace 已安装场景：静默自替换
func selfReplace(newExe string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	old := exe + ".old"
	os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return fmt.Errorf("旧程序改名失败: %w", err)
	}
	if err := copyFile(newExe, exe); err != nil {
		os.Rename(old, exe)
		return fmt.Errorf("写入新程序失败: %w", err)
	}
	startExe(exe)
	log.Printf("已更新，重启进程")
	os.Exit(0)
	return nil
}

// cleanupOld 启动清理上一次更新遗留
func cleanupOld() {
	if exe, err := os.Executable(); err == nil {
		os.Remove(exe + ".old")
	}
}