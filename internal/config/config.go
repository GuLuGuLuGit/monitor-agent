package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Config 应用配置
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Device    DeviceConfig    `mapstructure:"device"`
	Redis     RedisConfig     `mapstructure:"redis"`
	Intervals IntervalsConfig `mapstructure:"intervals"`
	Metrics   MetricsConfig   `mapstructure:"metrics"`
	Skills    SkillsConfig    `mapstructure:"skills"`
	Logs      LogsConfig      `mapstructure:"logs"`
	Cache     CacheConfig     `mapstructure:"cache"`
	Retry     RetryConfig     `mapstructure:"retry"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type ServerConfig struct {
	URL     string `mapstructure:"url"`
	Timeout int    `mapstructure:"timeout"`
}

type DeviceConfig struct {
	IDFile     string `mapstructure:"id_file"`
	APIKeyFile string `mapstructure:"api_key_file"`
}

type IntervalsConfig struct {
	Heartbeat int `mapstructure:"heartbeat"`
	Metrics   int `mapstructure:"metrics"`
	Skills    int `mapstructure:"skills"`
	LogUpload int `mapstructure:"log_upload"`
}

type MetricsConfig struct {
	BatchSize int `mapstructure:"batch_size"`
}

type SkillsConfig struct {
	ScanPaths []string `mapstructure:"scan_paths"`
}

type LogsConfig struct {
	Level      string `mapstructure:"level"`
	File       string `mapstructure:"file"`
	MaxSize    int    `mapstructure:"max_size"`
	MaxBackups int    `mapstructure:"max_backups"`
	BatchSize  int    `mapstructure:"batch_size"`
}

type CacheConfig struct {
	Dir       string `mapstructure:"dir"`
	MaxSizeMB int    `mapstructure:"max_size_mb"`
}

type RetryConfig struct {
	MaxAttempts     int `mapstructure:"max_attempts"`
	InitialInterval int `mapstructure:"initial_interval"`
	MaxInterval     int `mapstructure:"max_interval"`
}

// Load 从文件或环境加载配置
func Load(configPath string) (*Config, error) {
	v := viper.New()
	if configPath != "" {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("read config: %w", err)
		}
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("config")
		v.AddConfigPath("/etc/monitor-agent")
		_ = v.ReadInConfig()
	}

	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := c.applyDefaults(); err != nil {
		return nil, err
	}

	if err := c.Validate(); err != nil {
		return nil, err
	}

	return &c, nil
}

func (c *Config) applyDefaults() error {
	if c.Server.URL == "" {
		c.Server.URL = "http://localhost:8080"
	}
	if c.Redis.Addr == "" {
		c.Redis.Addr = "localhost:6379"
	}
	if c.Server.Timeout <= 0 {
		c.Server.Timeout = 30
	}
	if c.Intervals.Heartbeat <= 0 {
		c.Intervals.Heartbeat = 60
	}
	if c.Intervals.Metrics <= 0 {
		c.Intervals.Metrics = 30
	}
	if c.Intervals.Skills <= 0 {
		c.Intervals.Skills = 300
	}
	if c.Intervals.LogUpload <= 0 {
		c.Intervals.LogUpload = 60
	}
	if c.Metrics.BatchSize <= 0 {
		c.Metrics.BatchSize = 10
	}
	if c.Metrics.BatchSize > 100 {
		c.Metrics.BatchSize = 100
	}
	if c.Logs.BatchSize <= 0 {
		c.Logs.BatchSize = 100
	}
	if c.Logs.BatchSize > 1000 {
		c.Logs.BatchSize = 1000
	}
	if c.Logs.MaxSize <= 0 {
		c.Logs.MaxSize = 100
	}
	if c.Logs.MaxBackups <= 0 {
		c.Logs.MaxBackups = 3
	}
	if c.Retry.MaxAttempts <= 0 {
		c.Retry.MaxAttempts = 5
	}
	if c.Retry.InitialInterval <= 0 {
		c.Retry.InitialInterval = 1
	}
	if c.Retry.MaxInterval <= 0 {
		c.Retry.MaxInterval = 30
	}
	if c.Cache.MaxSizeMB <= 0 {
		c.Cache.MaxSizeMB = 50
	}

	// 默认设备/缓存/日志路径：用户目录下
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "."
	}
	agentDir := filepath.Join(home, ".monitor-agent")
	if c.Device.IDFile == "" {
		c.Device.IDFile = filepath.Join(agentDir, "device_id")
	}
	if c.Device.APIKeyFile == "" {
		c.Device.APIKeyFile = filepath.Join(agentDir, "api_key")
	}
	if c.Cache.Dir == "" {
		c.Cache.Dir = filepath.Join(agentDir, "cache")
	}
	if c.Logs.File == "" {
		c.Logs.File = filepath.Join(agentDir, "agent.log")
	}

	// 展开 skills 路径中的 ~
	for i, p := range c.Skills.ScanPaths {
		if strings.HasPrefix(p, "~/") {
			c.Skills.ScanPaths[i] = filepath.Join(home, p[2:])
		}
	}

	return nil
}

// Validate 校验配置
func (c *Config) Validate() error {
	if c.Server.URL == "" {
		return fmt.Errorf("server.url is required")
	}
	return nil
}
