package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type BackupConfig struct {
	ConfigDir     string   `json:"config_dir"`     // 配置文件目录
	BackupConfigs []Config `json:"backup_configs"` // 备份配置列表
	ProgressFile  string   `json:"progress_file"`  // 进度文件路径
}

type Config struct {
	SourceDir string `json:"source_dir"` // 源目录
	TargetDir string `json:"target_dir"` // 目标目录
}

type ProgressConfig struct {
	BackupConfigs []ProgressConfigItem `json:"backup_configs"`
}

type ProgressConfigItem struct {
	SourceDir    string    `json:"source_dir"`
	TargetDir    string    `json:"target_dir"`
	ProgressTime time.Time `json:"progress_time"`
}

func LoadConfig() (*BackupConfig, error) {
	// 首先从 .env 文件读取配置目录
	configDir := "C:\\Users\\tuilu\\Documents\\projects\\neo-nas"
	if configDir == "" {
		return nil, fmt.Errorf("BACKUP_CONFIG_DIR 环境变量未设置")
	}

	// 确保配置目录存在
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("创建配置目录失败: %w", err)
	}

	// 读取配置文件
	configPath := filepath.Join(configDir, "backup-config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// 如果配置文件不存在，创建默认配置
			config := &BackupConfig{
				ConfigDir:    configDir,
				ProgressFile: filepath.Join(configDir, ".backup-progress"),
			}
			if err := config.Save(); err != nil {
				return nil, err
			}
			return config, nil
		}
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var config BackupConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 确保配置目录和进度文件路径正确
	config.ConfigDir = configDir
	if config.ProgressFile == "" {
		config.ProgressFile = filepath.Join(configDir, ".backup-progress")
	}

	return &config, nil
}

func (c *BackupConfig) Save() error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	configPath := filepath.Join(c.ConfigDir, "backup-config.json")
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("保存配置文件失败: %w", err)
	}

	return nil
}

func LoadProgress(progressFile string) (*ProgressConfig, error) {
	data, err := os.ReadFile(progressFile)
	if err != nil {
		if os.IsNotExist(err) {
			return &ProgressConfig{}, nil
		}
		return nil, fmt.Errorf("读取进度文件失败: %w", err)
	}

	var progress ProgressConfig
	if err := json.Unmarshal(data, &progress); err != nil {
		return nil, fmt.Errorf("解析进度文件失败: %w", err)
	}

	return &progress, nil
}

func (p *ProgressConfig) Save(progressFile string) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化进度失败: %w", err)
	}

	if err := os.WriteFile(progressFile, data, 0644); err != nil {
		return fmt.Errorf("保存进度文件失败: %w", err)
	}

	return nil
}
