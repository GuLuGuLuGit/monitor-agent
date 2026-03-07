package main

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"monitor-agent/internal/cache"
	"monitor-agent/internal/config"
	"monitor-agent/internal/collector"
	"monitor-agent/internal/device"
	"monitor-agent/internal/uploader"
	"monitor-agent/pkg/client"
	"monitor-agent/pkg/logger"
)

func main() {
	configPath := flag.String("config", "", "config file path")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		panic("load config: " + err.Error())
	}

	if err := logger.Init(cfg.Logs.Level, cfg.Logs.File, cfg.Logs.MaxSize, cfg.Logs.MaxBackups); err != nil {
		panic("init logger: " + err.Error())
	}
	defer logger.Sync()

	logger.Info("monitor-agent starting", "version", device.AgentVersion, "server", cfg.Server.URL)

	// HTTP 客户端
	retry := client.RetryConfig{
		MaxAttempts:     cfg.Retry.MaxAttempts,
		InitialInterval: time.Duration(cfg.Retry.InitialInterval) * time.Second,
		MaxInterval:     time.Duration(cfg.Retry.MaxInterval) * time.Second,
	}
	cli := client.New(cfg.Server.URL, cfg.Server.Timeout, retry)
	up := uploader.New(cli)

	// 设备 ID 与 API Key
	deviceID, err := device.LoadID(cfg.Device.IDFile)
	if err != nil {
		logger.Error("load device id", "err", err)
		os.Exit(1)
	}

	apiKey, _ := device.LoadAPIKey(cfg.Device.APIKeyFile)
	if apiKey == "" || deviceID == "" {
		// 采集并注册
		info, err := device.Collect(deviceID)
		if err != nil {
			logger.Error("collect device info", "err", err)
			os.Exit(1)
		}
		_, err = device.LoadOrStoreID(cfg.Device.IDFile, info.DeviceID)
		if err != nil {
			logger.Error("store device id", "err", err)
			os.Exit(1)
		}
		deviceID = info.DeviceID

		resp, err := up.Register(info)
		if err != nil {
			logger.Error("register device", "err", err)
			os.Exit(1)
		}
		apiKey = resp.APIKey
		if err := device.StoreAPIKey(cfg.Device.APIKeyFile, apiKey); err != nil {
			logger.Error("store api key", "err", err)
			os.Exit(1)
		}
		logger.Info("device registered", "device_id", deviceID)
	}
	cli.SetAPIKey(apiKey)

	// 离线缓存（可选）
	var c *cache.Cache
	if cfg.Cache.Dir != "" {
		c, err = cache.New(cfg.Cache.Dir, cfg.Cache.MaxSizeMB)
		if err != nil {
			logger.Warn("cache init failed, running without cache", "err", err)
		} else {
			logger.Info("cache initialized", "dir", cfg.Cache.Dir)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup

	// 缓存恢复上报（启动后延迟执行）
	if c != nil {
		wg.Add(1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("cache recovery goroutine panic", "recover", r)
				}
			}()
			defer wg.Done()

			// 等待 10 秒，确保注册和认证完成
			time.Sleep(10 * time.Second)

			logger.Info("starting cache recovery")

			// 恢复指标缓存
			cachedMetrics, err := c.PopMetrics(100)
			if err != nil {
				logger.Warn("pop cached metrics failed", "err", err)
			} else if len(cachedMetrics) > 0 {
				logger.Info("recovering cached metrics", "count", len(cachedMetrics))
				var metrics []collector.MetricItem
				for _, data := range cachedMetrics {
					var m collector.MetricItem
					if err := json.Unmarshal(data, &m); err != nil {
						logger.Warn("unmarshal cached metric failed", "err", err)
						continue
					}
					metrics = append(metrics, m)
				}
				if len(metrics) > 0 {
					if err := up.UploadMetrics(metrics); err != nil {
						logger.Warn("upload cached metrics failed", "err", err)
						// 重新放回缓存
						for i := range metrics {
							_ = c.Push(cache.KindMetrics, metrics[i])
						}
					} else {
						logger.Info("cached metrics uploaded", "count", len(metrics))
					}
				}
			}

			// 恢复日志缓存
			cachedLogs, err := c.PopLogs(100)
			if err != nil {
				logger.Warn("pop cached logs failed", "err", err)
			} else if len(cachedLogs) > 0 {
				logger.Info("recovering cached logs", "count", len(cachedLogs))
				var logs []logger.LogEntry
				for _, data := range cachedLogs {
					var le logger.LogEntry
					if err := json.Unmarshal(data, &le); err != nil {
						logger.Warn("unmarshal cached log failed", "err", err)
						continue
					}
					logs = append(logs, le)
				}
				if len(logs) > 0 {
					batchID := time.Now().UTC().Format("20060102-150405") + "-recovery"
					if err := up.UploadLogs(batchID, logs); err != nil {
						logger.Warn("upload cached logs failed", "err", err)
						// 重新放回缓存
						for _, le := range logs {
							_ = c.Push(cache.KindLogs, le)
						}
					} else {
						logger.Info("cached logs uploaded", "count", len(logs))
					}
				}
			}
		}()
	}

	// 心跳
	wg.Add(1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("heartbeat goroutine panic", "recover", r)
			}
		}()
		defer wg.Done()
		ticker := time.NewTicker(time.Duration(cfg.Intervals.Heartbeat) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, err := up.Heartbeat(device.AgentVersion, 1)
				if err != nil {
					logger.Warn("heartbeat failed", "err", err)
					if c != nil {
						// 可选：标记离线，后续上报走缓存
					}
				}
			}
		}
	}()

	// 指标采集与上报
	metricQueue := make([]collector.MetricItem, 0, cfg.Metrics.BatchSize*2)
	wg.Add(1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("metrics goroutine panic", "recover", r)
			}
		}()
		defer wg.Done()
		ticker := time.NewTicker(time.Duration(cfg.Intervals.Metrics) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				if len(metricQueue) > 0 {
					_ = up.UploadMetrics(metricQueue)
				}
				return
			case <-ticker.C:
				m, err := collector.Collect()
				if err != nil {
					logger.Warn("collect metrics", "err", err)
					continue
				}
				metricQueue = append(metricQueue, *m)
				if len(metricQueue) >= cfg.Metrics.BatchSize {
					if err := up.UploadMetrics(metricQueue); err != nil {
						logger.Warn("upload metrics failed", "err", err)
						if c != nil {
							for i := range metricQueue {
								_ = c.Push(cache.KindMetrics, metricQueue[i])
							}
						}
					}
					metricQueue = metricQueue[:0]
				}
			}
		}
	}()

	// Skills 扫描与上报
	wg.Add(1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("skills goroutine panic", "recover", r)
			}
		}()
		defer wg.Done()
		ticker := time.NewTicker(time.Duration(cfg.Intervals.Skills) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				skills := collector.ScanSkills(cfg.Skills.ScanPaths)
				if len(skills) == 0 {
					continue
				}
				if err := up.UploadSkills(skills); err != nil {
					logger.Warn("upload skills failed", "err", err)
				}
			}
		}
	}()

	// 日志上报
	wg.Add(1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("logs goroutine panic", "recover", r)
			}
		}()
		defer wg.Done()
		ticker := time.NewTicker(time.Duration(cfg.Intervals.LogUpload) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				logs := logger.DrainEntries(cfg.Logs.BatchSize)
				if len(logs) > 0 {
					batchID := time.Now().UTC().Format("20060102-150405")
					_ = up.UploadLogs(batchID, logs)
				}
				return
			case <-ticker.C:
				logs := logger.DrainEntries(cfg.Logs.BatchSize)
				if len(logs) == 0 {
					continue
				}
				batchID := time.Now().UTC().Format("20060102-150405")
				if err := up.UploadLogs(batchID, logs); err != nil {
					logger.Warn("upload logs failed", "err", err)
					if c != nil {
						for _, le := range logs {
							_ = c.Push(cache.KindLogs, le)
						}
					}
				}
			}
		}
	}()

	// 优雅退出
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	logger.Info("shutting down")
	cancel()
	wg.Wait()
	logger.Info("exit")
}
