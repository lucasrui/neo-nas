package watcher

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/lucasrui/usb-backup/internal/backup"
)

type Watcher struct {
	sourceDir    string
	targetDir    string
	progressFile string
	backupMgr    *backup.Manager
	stopChan     chan struct{}
	status       *DirectoryStatus
}

type DirectoryStatus struct {
	IsBackingUp       bool
	IsLastCheckExists bool
	LastSync          time.Time
}

func NewWatcher(sourceDir, targetDir, targetUser, progressFile string) (*Watcher, error) {
	w := &Watcher{
		sourceDir:    sourceDir,
		targetDir:    targetDir,
		progressFile: progressFile,
		stopChan:     make(chan struct{}),
		status:       &DirectoryStatus{},
	}

	// 创建备份管理器
	var err error
	w.backupMgr, err = backup.NewManager(sourceDir, targetDir, targetUser, progressFile)

	return w, err
}

func (w *Watcher) Start() error {
	go w.checkDirectory()
	return nil
}

func (w *Watcher) Stop() error {
	close(w.stopChan)
	w.status.IsBackingUp = false
	log.Printf("停止监控目录: %s", w.sourceDir)
	return nil
}

func (w *Watcher) checkDirectory() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := w.checkDirectoryExists(); err != nil {
				log.Printf("检查目录失败: %v", err)
			}
		case <-w.stopChan:
			return
		}
	}
}

func (w *Watcher) checkDirectoryExists() error {
	// 检查源目录是否存在
	if _, err := os.Stat(w.sourceDir); err != nil {
		if os.IsNotExist(err) {
			if w.status.IsLastCheckExists {
				log.Printf("检测到源目录不存在：%s", w.sourceDir)
				w.status.IsLastCheckExists = false
			}
			return nil
		}
		return fmt.Errorf("检查源目录失败: %w", err)
		// 这里也可以考虑认为源目录不存在了
	}
	
	// 如果目录存在且上次是未挂载，重新启动监控 TODO 可以考虑支持定时备份，暂时用不到
	if !w.status.IsBackingUp && !w.status.IsLastCheckExists {
		log.Printf("检测到源目录已创建或挂载，开始监控: %s", w.sourceDir)
		w.status.IsLastCheckExists = true
		w.status.IsBackingUp = true
		// 执行初始目录扫描
		go w.scanDirectory()

	}

	return nil
}

func (w *Watcher) handleFileChange(filePath string) {
	// 执行备份
	if err := w.backupMgr.Backup(filePath); err != nil {
		log.Printf("备份文件失败: %v", err)
		return
	}

}

func (w *Watcher) scanDirectory() {
	log.Printf("开始扫描目录: %s", w.sourceDir)
	err := w.scanSubDirectory(w.sourceDir)

	if err != nil {
		log.Printf("扫描目录失败: %v", err)
	} else {
		log.Printf("目录扫描完成: %s", w.sourceDir)
		// 所有文件处理完成后，更新同步时间
		w.status.LastSync = time.Now()
		if err := w.backupMgr.SaveProgress(); err != nil {
			log.Printf("保存进度失败: %v", err)
		}
	}
	w.status.IsBackingUp = false
}

// scanSubDirectory 递归处理子目录
func (w *Watcher) scanSubDirectory(dirPath string) error {
	return filepath.WalkDir(dirPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			log.Printf("访问路径失败 %s: %v", path, err)
			return nil
		}
		if dirPath == path {
			return nil
		}
		// 获取源文件/目录信息
		srcInfo, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("获取文件信息失败: %w", err)
		}

		// 构建目标路径
		targetPath := w.backupMgr.BuildTargetPath(path)
		if targetPath == "" {
			return fmt.Errorf("无法构建目标路径: %s", path)
		}

		if d.IsDir() {
			// 创建目标目录
			if err := os.MkdirAll(targetPath, srcInfo.Mode()); err != nil {
				return fmt.Errorf("创建目标目录失败: %w", err)
			}

			// 递归处理子目录
			w.scanSubDirectory(path)

			// 同步目录时间
			atime := srcInfo.ModTime() // 使用修改时间作为访问时间
			mtime := srcInfo.ModTime() // 修改时间
			if err := os.Chtimes(targetPath, atime, mtime); err != nil {
				log.Printf("设置目录时间失败: %v", err)
			}

			// 验证时间是否设置成功
			if targetInfo, err := os.Stat(targetPath); err == nil {
				log.Printf("目录时间同步: %s -> %s (源目录时间: %v, 目标目录时间: %v)",
					path, targetPath, mtime, targetInfo.ModTime())
			}

			return filepath.SkipDir
		} else {
			// 处理文件，不更新时间
			w.handleFileChange(path)
		}
		return nil
	})
}
