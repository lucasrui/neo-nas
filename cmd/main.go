package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/lucasrui/neo-nas/internal/config"
	"github.com/lucasrui/neo-nas/internal/watcher"
	"github.com/lucasrui/neo-nas/internal/zip"
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

func (wm *WatcherManager) AddWatcher(sourceDir, targetDir, targetUser, progressFile string) error {
	// 需要校验目录合法性，如果是空字符串，则返回异常
	if sourceDir == "" || targetDir == "" || progressFile == "" {
		return fmt.Errorf("目录不能为空")
	}

	wm.mu.Lock()
	defer wm.mu.Unlock()

	// 检查是否已存在
	if _, exists := wm.watchers[sourceDir]; exists {
		return nil
	}

	// 创建新的 watcher
	w, err := watcher.NewWatcher(sourceDir, targetDir, targetUser, progressFile)
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

func main() {
	log.Println("正在启动 USB 备份程序...")

	// 加载配置
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("程序已停止，加载配置失败: %v", err)
		return
	}
	log.Printf("成功加载配置，配置目录: %s", cfg.ConfigDir)

	// 备份相关任务
	log.Printf("已配置 %d 个备份任务:", len(cfg.BackupConfigs))

	for i, bc := range cfg.BackupConfigs {
		log.Printf("  任务 %d: %s -> %s", i+1, bc.SourceDir, bc.TargetDir)
	}

	// 创建 watcher 管理器
	wm := NewWatcherManager()

	// 为每个配置创建 watcher，当所有任务都失败时退出，否则继续
	allFailed := true
	for _, backupCfg := range cfg.BackupConfigs {
		if err := wm.AddWatcher(backupCfg.SourceDir, backupCfg.TargetDir, backupCfg.TargetUser, cfg.ProgressFile); err != nil {
			log.Printf("添加目录监控失败 %s: %v", backupCfg.SourceDir, err)
		} else {
			allFailed = false
		}
	}

	// 压缩相关任务，先校验zip配置是否存在
	if cfg.ZipConfig.IntervalSeconds > 0 {
		zip.StartZipManager(cfg.ZipConfig)
	}

	if allFailed {
		log.Fatal("程序已停止，所有任务都失败")
		return
	}

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	// 停止所有监控
	wm.StopAll()
	log.Println("程序已停止")
}
