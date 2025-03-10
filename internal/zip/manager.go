package zip

import (
	"archive/zip"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/lucasrui/neo-nas/internal/config"
)

type ZipManager struct {
	IntervalSeconds int              `json:"interval_seconds"` // 压缩间隔时间
	Items           []config.ZipItem `json:"items"`            // 压缩配置列表
}

func StartZipManager(config config.ZipConfig) {
	zipMgr := &ZipManager{
		IntervalSeconds: config.IntervalSeconds,
		Items:           config.Items,
	}
	// 判断items的长度，如果为0，则不启动压缩任务
	if len(zipMgr.Items) == 0 {
		log.Printf("压缩任务列表为空，不启动压缩任务")
		return
	}
	log.Printf("已配置 %d 个压缩任务", len(zipMgr.Items))
	zipMgr.Start()
}

func (z *ZipManager) Start() {
	// 以intervalSeconds为时间间隔启动定时任务
	ticker := time.NewTicker(time.Duration(z.IntervalSeconds) * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// 遍历items，执行压缩任务
		for _, item := range z.Items {
			z.Zip(item)
		}
	}
}

// 压缩实现方法
func (z *ZipManager) Zip(item config.ZipItem) {
	// 输入item的日志
	log.Printf("执行压缩任务，源路径: %s, 目标路径: %s", item.Source, item.Target)
	// 创建压缩文件
	zipFile, err := os.Create(item.Target)
	if err != nil {
		log.Printf("创建压缩文件失败: %v", err)
		return
	}
	defer zipFile.Close()

	// 创建 zip.Writer
	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// 检查item.Source是否存在，以及是否为文件夹、文件
	info, err := os.Stat(item.Source)
	if err != nil {
		log.Printf("源路径不存在: %v", err)
		return
	}
	if info.IsDir() {
		// 遍历源路径中的文件并添加到压缩文件中
		err = filepath.Walk(item.Source, func(file string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil // 跳过目录
			}

			// 创建压缩文件中的文件
			// 获取相对路径
			relPath, err := filepath.Rel(item.Source, file)
			if err != nil {
				return err
			}
			zipFileWriter, err := zipWriter.Create(relPath)
			if err != nil {
				return err
			}

			// 打开源文件
			srcFile, err := os.Open(file)
			if err != nil {
				return err
			}
			defer srcFile.Close()

			// 复制文件内容到压缩文件
			_, err = io.Copy(zipFileWriter, srcFile)
			return err
		})
	} else {
		// 创建压缩文件中的文件
		// 获取相对路径
		relPath, err := filepath.Rel(filepath.Dir(item.Source), item.Source)
		if err != nil {
			return
		}
		zipFileWriter, err := zipWriter.Create(relPath)
		if err != nil {
			log.Printf("创建压缩文件中的文件失败: %v", err)
			return
		}

		// 打开源文件
		srcFile, err := os.Open(item.Source)
		if err != nil {
			log.Printf("打开源文件失败: %v", err)
			return
		}
		defer srcFile.Close()

		// 复制文件内容到压缩文件
		_, err = io.Copy(zipFileWriter, srcFile)
		if err != nil {
			log.Printf("复制文件内容到压缩文件失败: %v", err)
			return
		}

	}

	if err != nil {
		log.Printf("压缩文件失败: %v", err)
		return
	}

	log.Printf("压缩任务完成，源路径: %s, 目标路径: %s", item.Source, item.Target)
}
