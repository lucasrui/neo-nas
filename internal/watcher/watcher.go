package watcher

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/lucasrui/usb-backup/internal/backup"
)

type Watcher struct {
	sourceDir    string
	targetDir    string
	progressFile string
	backupMgr    *backup.Manager
	watcher      *fsnotify.Watcher
	stopChan     chan struct{}
	status       *DirectoryStatus
	watchedDirs  sync.Map // 记录已监控的目录
}

type DirectoryStatus struct {
	IsWatching bool
	LastError  error
	LastSync   time.Time
}

// isWindows 检查当前操作系统是否为 Windows
func isWindows() bool {
	return runtime.GOOS == "windows"
}

func NewWatcher(sourceDir, targetDir, progressFile string) (*Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("创建文件监控器失败: %w", err)
	}

	w := &Watcher{
		sourceDir:    sourceDir,
		targetDir:    targetDir,
		progressFile: progressFile,
		watcher:      watcher,
		stopChan:     make(chan struct{}),
		status:       &DirectoryStatus{},
	}

	// 创建备份管理器
	w.backupMgr = backup.NewManager(sourceDir, targetDir, progressFile)

	return w, nil
}

func (w *Watcher) addToWatch(dirPath string) error {
	// 在 Linux 上，只需要监控根目录
	if !isWindows() && dirPath != w.sourceDir {
		return nil
	}

	// 检查是否已经监控
	if _, exists := w.watchedDirs.Load(dirPath); exists {
		return nil
	}

	// 添加到监控
	if err := w.watcher.Add(dirPath); err != nil {
		return fmt.Errorf("添加目录到监控失败: %w", err)
	}

	// 记录已监控
	w.watchedDirs.Store(dirPath, true)
	log.Printf("已添加目录到监控: %s", dirPath)
	return nil
}

func (w *Watcher) Start() error {
	// 检查目录是否存在
	if _, err := os.Stat(w.sourceDir); err != nil {
		w.status.IsWatching = false
		w.status.LastError = err
		log.Printf("源目录不存在，等待目录创建或挂载: %s", w.sourceDir)
	} else {
		w.status.IsWatching = true
		w.status.LastError = nil

		// 添加根目录到监控列表
		if err := w.addToWatch(w.sourceDir); err != nil {
			w.status.LastError = err
			return err
		}

		// 启动文件监控
		go w.watchFiles()

		// 执行初始目录扫描
		go w.scanDirectory()

		log.Printf("开始监控目录: %s", w.sourceDir)
	}

	// 无论目录是否存在，都启动定期检查
	go w.checkDirectory()

	return nil
}

func (w *Watcher) Stop() error {
	close(w.stopChan)
	w.watcher.Close()
	w.status.IsWatching = false
	// 清空已监控的目录列表
	w.watchedDirs = sync.Map{}
	log.Printf("停止监控目录: %s", w.sourceDir)
	return nil
}

func (w *Watcher) GetStatus() *DirectoryStatus {
	return w.status
}

func (w *Watcher) handleDirectoryCreate(dirPath string) error {
	// 获取源目录信息
	srcInfo, err := os.Stat(dirPath)
	if err != nil {
		return fmt.Errorf("获取目录信息失败: %w", err)
	}

	// 构建目标目录路径
	targetPath := w.backupMgr.BuildTargetPath(dirPath)
	if targetPath == "" {
		return fmt.Errorf("无法构建目标路径: %s", dirPath)
	}

	// 确保目标目录存在
	if err := os.MkdirAll(targetPath, srcInfo.Mode()); err != nil {
		return fmt.Errorf("创建目标目录失败: %w", err)
	}

	// 同步目录时间
	atime := srcInfo.ModTime() // 使用修改时间作为访问时间
	mtime := srcInfo.ModTime() // 修改时间
	if err := os.Chtimes(targetPath, atime, mtime); err != nil {
		log.Printf("设置目录时间失败: %v", err)
	}

	log.Printf("目录同步完成: %s -> %s", dirPath, targetPath)
	return nil
}

func (w *Watcher) watchFiles() {
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			// 检查源目录是否仍然存在
			if _, err := os.Stat(w.sourceDir); err != nil {
				if os.IsNotExist(err) {
					log.Printf("源目录已断开，停止监控: %s", w.sourceDir)
					w.Stop()
					return
				}
			}

			// 处理所有文件事件
			switch {
			case event.Op&fsnotify.Write == fsnotify.Write:
				w.handleFileChange(event.Name, true)
			case event.Op&fsnotify.Create == fsnotify.Create:
				// 检查是否是目录
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					// 创建目录
					if err := w.handleDirectoryCreate(event.Name); err != nil {
						log.Printf("处理目录创建失败: %v", err)
					}
					// 在 Windows 上需要添加子目录到监控
					if isWindows() {
						if err := w.addToWatch(event.Name); err != nil {
							log.Printf("添加子目录到监控失败: %v", err)
						}
					}
				} else {
					// 如果是文件，处理文件变更
					w.handleFileChange(event.Name, true)
				}
			case event.Op&fsnotify.Remove == fsnotify.Remove:
				log.Printf("文件或目录已删除: %s", event.Name)
				// 从监控列表中移除
				w.watchedDirs.Delete(event.Name)
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.status.LastError = err
			log.Printf("监控错误: %v", err)
		case <-w.stopChan:
			return
		}
	}
}

func (w *Watcher) checkDirectory() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := w.checkDirectoryExists(); err != nil {
				w.status.LastError = err
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
			// 目录不存在，如果正在监控则停止监控
			if w.status.IsWatching {
				log.Printf("源目录已断开，停止监控: %s", w.sourceDir)
				if err := w.Stop(); err != nil {
					log.Printf("停止监控失败: %v", err)
				}
			}
			w.status.IsWatching = false
			w.status.LastError = err
			return nil
		}
		return fmt.Errorf("检查源目录失败: %w", err)
	}

	// 如果目录存在但未在监控中，开始监控
	if !w.status.IsWatching {
		log.Printf("检测到源目录已创建或挂载，开始监控: %s", w.sourceDir)
		// 重新创建 stopChan
		w.stopChan = make(chan struct{})
		if err := w.Start(); err != nil {
			return fmt.Errorf("启动监控失败: %w", err)
		}
	}

	return nil
}

func (w *Watcher) handleFileChange(filePath string, updateTime bool) {
	// 检查文件是否存在
	if _, err := os.Stat(filePath); err != nil {
		log.Printf("文件不存在，跳过处理: %s", filePath)
		return
	}

	// 执行备份
	if err := w.backupMgr.Backup(filePath); err != nil {
		w.status.LastError = err
		log.Printf("备份文件失败: %v", err)
		return
	}

	// 只在需要时更新同步时间
	if updateTime {
		w.status.LastSync = time.Now()
		if err := w.backupMgr.SaveProgress(); err != nil {
			log.Printf("保存进度失败: %v", err)
		}
	}
}

func (w *Watcher) scanDirectory() {
	log.Printf("开始扫描目录: %s", w.sourceDir)
	totalFiles := 0
	totalDirs := 0
	err := w.scanSubDirectory(w.sourceDir)

	if err != nil {
		log.Printf("扫描目录失败: %v", err)
	} else {
		log.Printf("目录扫描完成: %s, 共处理 %d 个文件和 %d 个目录", w.sourceDir, totalFiles, totalDirs)
		// 所有文件处理完成后，更新同步时间
		w.status.LastSync = time.Now()
		if err := w.backupMgr.SaveProgress(); err != nil {
			log.Printf("保存进度失败: %v", err)
		}
	}
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
			// 在 Windows 上需要添加所有目录到监控
			if isWindows() {
				if err := w.addToWatch(path); err != nil {
					log.Printf("添加目录到监控失败: %v", err)
				}
			}

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
			w.handleFileChange(path, false)
		}
		return nil
	})
}
