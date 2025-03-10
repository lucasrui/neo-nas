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
	TotalFiles        int
	SuccessFiles      int
	FailedFiles       int
	SkippedFiles      int
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
				log.Printf("检测到源目录已离线：%s", w.sourceDir)
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
	status := w.backupMgr.Backup(filePath)
	switch status {
	case backup.Success:
		w.status.SuccessFiles++
	case backup.Failed:
		log.Printf("备份文件失败: %v", filePath)
		w.status.FailedFiles++
	case backup.Skipped:
		w.status.SkippedFiles++
	}
}

func (w *Watcher) scanDirectory() {
	log.Printf("开始扫描目录: %s", w.sourceDir)
	// 清空数量记录数
	w.status.TotalFiles = 0
	w.status.SuccessFiles = 0
	w.status.FailedFiles = 0
	w.status.SkippedFiles = 0
	err := w.scanSubDirectory(w.sourceDir)

	// 扫描数量 = 同步成功 + 失败 + 跳过，结果日志包含这些信息，失败了也需要这些信息
	if err != nil {
		log.Printf("目录扫描失败: %s, 扫描数量: %d, 同步成功: %d, 失败: %d, 跳过: %d, 错误原因: %v", w.sourceDir, w.status.TotalFiles, w.status.SuccessFiles, w.status.FailedFiles, w.status.SkippedFiles, err)
	} else {
		log.Printf("目录扫描完成: %s, 扫描数量: %d, 同步成功: %d, 失败: %d, 跳过: %d", w.sourceDir, w.status.TotalFiles, w.status.SuccessFiles, w.status.FailedFiles, w.status.SkippedFiles)
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
		// 跳过自身
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
			isNewDir := false
			if _, err := os.Stat(targetPath); err != nil {
				if err := os.MkdirAll(targetPath, srcInfo.Mode()); err != nil {
					return fmt.Errorf("创建目标目录失败: %w", err)
				}
				isNewDir = true
			}

			// 递归处理子目录
			w.scanSubDirectory(path)
			
			// 如果是新创建的目录，且里面不存在文件，说明是无效目录，需要删除
			if isNewDir {
				// check files count in target path
				files, err := os.ReadDir(targetPath)
				if err == nil && len(files) == 0 {
					// 删除targetPath目录
					if err := os.Remove(targetPath); err != nil {
						log.Printf("删除目标目录失败: %v", err)
					}
					return filepath.SkipDir
				}
			}
			// 同步目录时间 TODO 设置用户属性
			atime := srcInfo.ModTime() // 使用修改时间作为访问时间
			mtime := srcInfo.ModTime() // 修改时间
			if err := os.Chtimes(targetPath, atime, mtime); err != nil {
				log.Printf("设置目录时间失败: %v", err)
			}

			return filepath.SkipDir
		} else {
			w.status.TotalFiles++
			// 处理文件，不更新时间
			w.handleFileChange(path)
		}
		return nil
	})
}
