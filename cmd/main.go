package main

import (
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/lucasrui/usb-backup/internal/config"
	"github.com/lucasrui/usb-backup/internal/watcher"
)

type WatcherManager struct {
	watchers map[string]*watcher.Watcher
	mu       sync.RWMutex
}

func NewWatcherManager() *WatcherManager {
	return &WatcherManager{
		watchers: make(map[string]*watcher.Watcher),
	}
}

func (wm *WatcherManager) AddWatcher(sourceDir, targetDir, progressFile string) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	// 检查是否已存在
	if _, exists := wm.watchers[sourceDir]; exists {
		return nil
	}

	// 创建新的 watcher
	w, err := watcher.NewWatcher(sourceDir, targetDir, progressFile)
	if err != nil {
		return err
	}

	// 启动监控
	if err := w.Start(); err != nil {
		return err
	}

	wm.watchers[sourceDir] = w
	log.Printf("已添加目录监控: %s", sourceDir)
	return nil
}

func (wm *WatcherManager) RemoveWatcher(sourceDir string) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	w, exists := wm.watchers[sourceDir]
	if !exists {
		return nil
	}

	if err := w.Stop(); err != nil {
		return err
	}

	delete(wm.watchers, sourceDir)
	log.Printf("已移除目录监控: %s", sourceDir)
	return nil
}

func (wm *WatcherManager) StopAll() {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	for sourceDir, w := range wm.watchers {
		if err := w.Stop(); err != nil {
			log.Printf("停止监控失败 %s: %v", sourceDir, err)
		}
		delete(wm.watchers, sourceDir)
	}
}

func (wm *WatcherManager) GetStatus(sourceDir string) *watcher.DirectoryStatus {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	w, exists := wm.watchers[sourceDir]
	if !exists {
		return nil
	}

	return w.GetStatus()
}

func main() {
	log.Println("正在启动 USB 备份程序...")

	// 加载配置
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	log.Printf("成功加载配置，配置目录: %s", cfg.ConfigDir)
	log.Printf("已配置 %d 个备份任务:", len(cfg.BackupConfigs))
	for i, bc := range cfg.BackupConfigs {
		log.Printf("  任务 %d: %s -> %s", i+1, bc.SourceDir, bc.TargetDir)
	}

	// 验证配置
	if len(cfg.BackupConfigs) == 0 {
		log.Fatal("未配置备份目录")
	}

	// 创建 watcher 管理器
	wm := NewWatcherManager()

	// 为每个配置创建 watcher
	for _, backupCfg := range cfg.BackupConfigs {
		if err := wm.AddWatcher(backupCfg.SourceDir, backupCfg.TargetDir, cfg.ProgressFile); err != nil {
			log.Printf("添加目录监控失败 %s: %v", backupCfg.SourceDir, err)
		}
	}

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	// 停止所有监控
	wm.StopAll()
	log.Println("程序已停止")
}
