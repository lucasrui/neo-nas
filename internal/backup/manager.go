package backup

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lucasrui/usb-backup/internal/config"
)

// 定义备份状态码
type BackupStatus int

const (
	Success BackupStatus = iota
	Failed
	Skipped
)

type Manager struct {
	sourceDir    string
	targetDir    string
	targetUid    int
	targetGid    int
	progressFile string
	progress     *config.ProgressConfig
	activeOps    sync.WaitGroup
	progressLock sync.Mutex
}

func NewManager(sourceDir, targetDir, targetUser, progressFile string) (*Manager, error) {
	// 确保目标目录存在
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		log.Printf("创建目标目录失败: %v", err)
		return nil, err
	}

	// 从targetUser中解析出uid和gid，格式为uid:gid
	targetUid, targetGid := 0, 0
	if targetUser != "" {
		uidGid := strings.Split(targetUser, ":")
		if len(uidGid) == 2 {
			targetUid, _ = strconv.Atoi(uidGid[0])
			targetGid, _ = strconv.Atoi(uidGid[1])
		}
	}

	m := &Manager{
		sourceDir:    sourceDir,
		targetDir:    targetDir,
		targetUid:    targetUid,
		targetGid:    targetGid,
		progressFile: progressFile,
	}

	// 加载上次同步时间
	if err := m.loadProgress(); err != nil {
		log.Printf("加载进度文件失败: %v", err)
		return nil, err
	}
	return m, nil
}

// 返回一个状态码，用于表示备份结果，可能是成功，失败，或者跳过
func (m *Manager) Backup(sourcePath string) BackupStatus {
	m.activeOps.Add(1)
	defer m.activeOps.Done()

	// 构建目标路径
	targetPath := m.BuildTargetPath(sourcePath)
	if targetPath == "" {
		return Failed
	}

	// 获取文件信息
	fileInfo, err := os.Stat(sourcePath)
	if err != nil {
		return Failed
	}

	// 获取对应配置的同步时间
	lastSyncTime := m.getLastSyncTime()
	if lastSyncTime != nil {
		// 使用修改时间作为判断依据
		fileTime := fileInfo.ModTime()
		if fileTime.Before(*lastSyncTime) {
			return Skipped
		}
	}

	// 检查目标文件是否存在，如果存在就跳过
	_, err = os.Stat(targetPath)
	if err == nil {
		return Skipped
	}

	// 执行备份（覆盖已存在的文件）
	if err := m.copyFile(sourcePath, targetPath); err != nil {
		return Failed
	}

	log.Printf("文件备份完成: %s -> %s", sourcePath, targetPath)
	return Success
}

// func (m *Manager) calculateFileHash(path string) (string, error) {
// 	file, err := os.Open(path)
// 	if err != nil {
// 		return "", err
// 	}
// 	defer file.Close()

// 	hash := sha256.New()
// 	if _, err := io.Copy(hash, file); err != nil {
// 		return "", err
// 	}

// 	return hex.EncodeToString(hash.Sum(nil)), nil
// }

func (m *Manager) copyFile(src, dst string) error {
	// 打开源文件
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("打开源文件失败: %w", err)
	}
	defer srcFile.Close()

	// 创建目标文件
	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("创建目标文件失败: %w", err)
	}
	defer dstFile.Close()

	// 复制文件内容
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("复制文件内容失败: %w", err)
	}

	// 获取源文件信息
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("获取源文件信息失败: %w", err)
	}

	// 设置目标文件权限
	if err := os.Chmod(dst, srcInfo.Mode()); err != nil {
		log.Printf("设置目标文件权限失败: %v", err)
	}

	// 设置目标文件时间
	if err := os.Chtimes(dst, srcInfo.ModTime(), srcInfo.ModTime()); err != nil {
		log.Printf("设置目标文件时间失败: %v", err)
	}

	// 设置目标文件的 UID 和 GID
	if m.targetUid != 0 || m.targetGid != 0 {
		if err := os.Chown(dst, m.targetUid, m.targetGid); err != nil {
			log.Printf("设置目标文件 UID 和 GID 失败: %v", err)
		}
	}

	return nil
}

func (m *Manager) loadProgress() error {
	log.Printf("尝试加载进度文件: %s", m.progressFile)

	progress, err := config.LoadProgress(m.progressFile)
	if err != nil {
		return err
	}

	m.progress = progress
	log.Printf("成功加载进度配置")
	return nil
}

func (m *Manager) SaveProgress() error {
	m.progressLock.Lock()
	defer m.progressLock.Unlock()

	// 检查源目录是否存在
	if _, err := os.Stat(m.sourceDir); err != nil {
		log.Printf("源目录不存在，跳过保存进度: %s", m.sourceDir)
		return nil
	}

	// 更新同步时间
	now := time.Now()
	m.updateProgressTime(now)

	// 保存进度
	if err := m.progress.Save(m.progressFile); err != nil {
		return fmt.Errorf("保存进度失败: %w", err)
	}

	log.Printf("成功保存进度配置")
	return nil
}

func (m *Manager) updateProgressTime(time time.Time) {
	for i, item := range m.progress.BackupConfigs {
		if item.SourceDir == m.sourceDir {
			m.progress.BackupConfigs[i].ProgressTime = time
			return
		}
	}
	// 如果没有找到，添加新的
	m.progress.BackupConfigs = append(m.progress.BackupConfigs, config.ProgressConfigItem{
		SourceDir:    m.sourceDir,
		ProgressTime: time,
	})
}

func (m *Manager) getLastSyncTime() *time.Time {
	// 检查源目录是否存在
	if _, err := os.Stat(m.sourceDir); err != nil {
		log.Printf("源目录不存在，不检查上次同步时间: %s", m.sourceDir)
		return nil
	}

	// 查找对应的进度时间
	for _, item := range m.progress.BackupConfigs {
		if item.SourceDir == m.sourceDir {
			// 检查源目录是否仍然存在
			if _, err := os.Stat(m.sourceDir); err != nil {
				log.Printf("源目录已不存在，不检查上次同步时间: %s", m.sourceDir)
				return nil
			}
			return &item.ProgressTime
		}
	}

	return nil
}

// BuildTargetPath 构建目标路径
func (m *Manager) BuildTargetPath(sourcePath string) string {
	// 获取相对路径
	relPath, err := filepath.Rel(m.sourceDir, sourcePath)
	if err != nil {
		log.Printf("无法获取相对路径: %v", err)
		return ""
	}

	return filepath.Join(m.targetDir, relPath)
}

// 添加 WaitForCompletion 方法
func (m *Manager) WaitForCompletion() {
	m.activeOps.Wait()
}

// 添加目录同步方法
func (m *Manager) SyncDirectory(sourcePath string) error {
	// 构建目标目录路径
	targetPath := m.BuildTargetPath(sourcePath)
	if targetPath == "" {
		return fmt.Errorf("无法构建目标路径: %s", sourcePath)
	}

	// 获取源目录信息
	srcInfo, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("获取源目录信息失败: %w", err)
	}

	// 确保目标目录存在
	if err := os.MkdirAll(targetPath, srcInfo.Mode()); err != nil {
		return fmt.Errorf("创建目标目录失败: %w", err)
	}

	// 设置目录时间
	atime := srcInfo.ModTime() // 使用修改时间作为访问时间
	mtime := srcInfo.ModTime() // 修改时间
	if err := os.Chtimes(targetPath, atime, mtime); err != nil {
		log.Printf("设置目录时间失败: %v", err)
	}

	log.Printf("目录同步完成: %s -> %s", sourcePath, targetPath)
	return nil
}
