//go:build windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"bmclient/bm"
)

// Account 本机保存的账号
type Account struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Pwd  string `json:"pwd"`
}

// Config 本机配置
type Config struct {
	Accounts []Account `json:"accounts"`
	Last     string    `json:"last"`
}

func cfgPath() string {
	return filepath.Join(installDir(), "bm_config.json")
}

// loadConfig 兼容旧版配置迁移（安装目录 > exe 同目录）
func loadConfig() *Config {
	c := &Config{}
	if b, err := os.ReadFile(cfgPath()); err == nil {
		json.Unmarshal(b, c)
		return c
	}
	if exe, err := os.Executable(); err == nil {
		old := filepath.Join(filepath.Dir(exe), "bm_config.json")
		if b, err := os.ReadFile(old); err == nil {
			if json.Unmarshal(b, c) == nil && len(c.Accounts) > 0 {
				return c
			}
		}
	}
	return c
}

func saveConfig(c *Config) {
	os.MkdirAll(installDir(), 0755)
	b, _ := json.MarshalIndent(c, "", "  ")
	os.WriteFile(cfgPath(), b, 0600)
}

func upsertAccount(id, name string) {
	found := false
	for i := range cfg.Accounts {
		if cfg.Accounts[i].ID == id {
			cfg.Accounts[i].Name = name
			found = true
		}
	}
	if !found {
		cfg.Accounts = append(cfg.Accounts, Account{ID: id, Name: name, Pwd: bm.DefaultPassword})
	}
	cfg.Last = id
	saveConfig(cfg)
}

// removeAccount 删除账号记录；返回是否删除了当前登录账号
func removeAccount(id string) bool {
	kept := make([]Account, 0, len(cfg.Accounts))
	removed := false
	for _, a := range cfg.Accounts {
		if a.ID == id {
			removed = true
			continue
		}
		kept = append(kept, a)
	}
	if !removed {
		return false
	}
	cfg.Accounts = kept
	if cfg.Last == id {
		cfg.Last = ""
		if len(cfg.Accounts) > 0 {
			cfg.Last = cfg.Accounts[0].ID
		}
	}
	saveConfig(cfg)
	// 当前登录的就是被删账号 → 登出
	if c := rtClient(); c != nil && c.Account == id {
		rt.mu.Lock()
		if rt.running && rt.stopCh != nil {
			close(rt.stopCh)
		}
		rt.running = false
		rt.stopCh = nil
		rt.client = nil
		rt.lastPts = nil
		rt.mu.Unlock()
		return true
	}
	return false
}

// rtClient 当前登录的客户端（线程安全）
func rtClient() *bm.Client {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.client
}

var cfg = loadConfig()