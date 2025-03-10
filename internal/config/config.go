package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type NeoConfig struct {
	ConfigDir     string    `json:"config_dir"`     // 配置文件目录
	BackupConfigs []Config  `json:"backup_configs"` // 备份配置列表
	ZipConfig     ZipConfig `json:"zip_config"`     // 压缩配置列表
	ProgressFile  string    `json:"progress_file"`  // 进度文件路径
}

type Config struct {
	SourceDir  string `json:"source_dir"`  // 源目录
	TargetDir  string `json:"target_dir"`  // 目标目录
	TargetUser string `json:"target_user"` // 目标用户
}

type ZipConfig struct {
	IntervalSeconds int       `json:"interval_seconds"` // 压缩间隔时间
	Items           []ZipItem `json:"items"`            // 压缩配置列表
}

type ZipItem struct {
	Source string `json:"source"` // 源文件
	Target string `json:"target"` // 目标文件
	Key    string `json:"key"`    // 密钥
}

type ProgressConfig struct {
	BackupConfigs []ProgressConfigItem `json:"backup_configs"`
}

type ProgressConfigItem struct {
	SourceDir    string    `json:"source_dir"`
	TargetDir    string    `json:"target_dir"`
	ProgressTime time.Time `json:"progress_time"`
}

func LoadConfig() (*NeoConfig, error) {
	// 首先从环境变量读取配置目录
	configDir := os.Getenv("BACKUP_CONFIG_DIR")
	if configDir == "" {
		// 如果环境变量未设置，使用默认目录
		configDir = "/config"
	}

	// 判断配置目录是否存在，不存在直接返回异常
	if _, err := os.Stat(configDir); err != nil {
		return nil, fmt.Errorf("配置目录: %s 不存在, %w", configDir, err)
	}

	// 读取配置文件
	configPath := filepath.Join(configDir, "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// 如果配置文件不存在，直接返回异常
			return nil, fmt.Errorf("配置文件: %s 不存在, %w", configPath, err)
		}
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var config NeoConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 确保配置目录和进度文件路径正确
	config.ConfigDir = configDir
	config.ProgressFile = filepath.Join(configDir, ".backup-progress")

	return &config, nil
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
