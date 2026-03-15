package main

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"monitor-agent/internal/cache"
	"monitor-agent/internal/collector"
	"monitor-agent/internal/command"
	"monitor-agent/internal/config"
	"monitor-agent/internal/device"
	"monitor-agent/internal/identity"
	"monitor-agent/internal/openclawstate"
	"monitor-agent/internal/pairing"
	"monitor-agent/internal/transport"
	"monitor-agent/internal/uploader"
	"monitor-agent/pkg/client"
	"monitor-agent/pkg/logger"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "pairing-code":
			if err := runPairingCodeCommand(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		case "status":
			if err := runStatusCommand(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		}
	}

	if err := runAgent(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runAgent(args []string) error {
	fs := flag.NewFlagSet("monitor-agent", flag.ContinueOnError)
	configPath := fs.String("config", "", "config file path")
	showInfo := fs.Bool("info", false, "show node identity and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, nodeIdentity, err := loadRuntime(*configPath)
	if err != nil {
		return err
	}

	if *showInfo {
		fp, _ := nodeIdentity.FingerprintStr()
		fmt.Printf("Node ID:      %s\n", nodeIdentity.NodeID)
		fmt.Printf("Key Storage:  %s\n", nodeIdentity.StorageType())
		fmt.Printf("Public Key:   %s (SHA-256)\n", fp)
		fmt.Printf("Data Dir:     %s\n", cfg.Device.DataDir)
		return nil
	}

	if err := logger.Init(cfg.Logs.Level, cfg.Logs.File, cfg.Logs.MaxSize, cfg.Logs.MaxBackups); err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	defer logger.Sync()

	logger.Info("monitor-agent starting",
		"version", device.AgentVersion,
		"server", cfg.Server.URL,
		"node_id", nodeIdentity.NodeID,
		"key_storage", nodeIdentity.StorageType())

	retry := retryConfig(cfg)
	cli := client.New(cfg.Server.URL, cfg.Server.Timeout, retry)
	up := uploader.New(cli)

	deviceID := nodeIdentity.NodeID
	deviceInfo, err := device.Collect(deviceID)
	if err != nil {
		logger.Warn("collect device info failed", "err", err)
	}

	apiKey, _ := device.LoadAPIKey(cfg.Device.APIKeyFile)
	if apiKey == "" {
		if deviceInfo == nil {
			deviceInfo, err = device.Collect(deviceID)
		}
		if err != nil || deviceInfo == nil {
			logger.Error("collect device info", "err", err)
			return fmt.Errorf("collect device info: %w", err)
		}

		apiKey, err = pairing.RunPairing(cli, nodeIdentity, deviceInfo.Hostname, deviceInfo.OSVersion)
		if err != nil {
			logger.Error("pairing failed", "err", err)
			return fmt.Errorf("pairing failed: %w", err)
		}

		if err := device.StoreAPIKey(cfg.Device.APIKeyFile, apiKey); err != nil {
			logger.Error("store api key", "err", err)
			return fmt.Errorf("store api key: %w", err)
		}
		logger.Info("device registered", "device_id", deviceID)
	}
	cli.SetAPIKey(apiKey)

	var cmdBroker transport.CommandBroker
	var resultReporter transport.ResultReporter
	var rdb *goredis.Client

	switch cfg.Transport.Type {
	case "mqtt":
		mqttBrokerURL := cfg.Transport.MQTT.Broker
		if mqttBrokerURL == "" {
			mqttBrokerURL = "tcp://localhost:1883"
		}
		mqttBroker, err := transport.NewMQTTBroker(transport.MQTTBrokerConfig{
			BrokerURL:     mqttBrokerURL,
			DeviceID:      deviceID,
			KeepAlive:     cfg.Transport.MQTT.KeepAlive,
			AutoReconnect: cfg.Transport.MQTT.AutoReconnect,
			Username:      cfg.Transport.MQTT.Username,
			Password:      cfg.Transport.MQTT.Password,
			UseTLS:        cfg.Transport.MQTT.UseTLS,
		})
		if err != nil {
			logger.Warn("MQTT connection failed, falling back to Redis", "err", err)
		} else {
			cmdBroker = mqttBroker
			resultReporter = mqttBroker
			logger.Info("transport: MQTT", "broker", mqttBrokerURL)
		}
	}

	if cmdBroker == nil {
		rdb = goredis.NewClient(&goredis.Options{
			Addr:     cfg.Redis.Addr,
			Password: cfg.Redis.Password,
			DB:       cfg.Redis.DB,
		})
		if err := rdb.Ping(context.Background()).Err(); err != nil {
			logger.Warn("redis connection failed, command consumer disabled", "err", err)
			rdb = nil
		} else {
			logger.Info("transport: Redis", "addr", cfg.Redis.Addr)
			cmdBroker = transport.NewRedisBroker(rdb)
		}
	}

	if resultReporter == nil {
		resultReporter = transport.NewHTTPResultReporter(cli)
	}

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

	if c != nil {
		wg.Add(1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("cache recovery goroutine panic", "recover", r)
				}
			}()
			defer wg.Done()

			time.Sleep(10 * time.Second)
			logger.Info("starting cache recovery")

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
						for i := range metrics {
							_ = c.Push(cache.KindMetrics, metrics[i])
						}
					} else {
						logger.Info("cached metrics uploaded", "count", len(metrics))
					}
				}
			}

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
		heartbeatCount := 0
		const openclawInterval = 5
		const recentMessageWindow = 10 * time.Minute
		const agentsChangeCooldown = 30 * time.Second
		const messageActivityCooldown = 2 * time.Minute
		var lastExtraHash string
		var lastAgentsFingerprint string
		var lastAgentsTriggeredAt time.Time
		var lastMessageTriggeredAt time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				heartbeatCount++
				var extraData *string
				intervalDue := heartbeatCount%openclawInterval == 1
				recentMessage := openclawstate.RecentMessageActivityWithin(recentMessageWindow)
				currentAgentsFingerprint := collector.CollectAgentsFingerprint()
				agentsChanged := currentAgentsFingerprint != "" && currentAgentsFingerprint != lastAgentsFingerprint
				agentsDue := agentsChanged && (lastAgentsTriggeredAt.IsZero() || time.Since(lastAgentsTriggeredAt) >= agentsChangeCooldown)
				messageDue := recentMessage && (lastMessageTriggeredAt.IsZero() || time.Since(lastMessageTriggeredAt) >= messageActivityCooldown)
				shouldCollectExtra := intervalDue || agentsDue || messageDue

				if shouldCollectExtra {
					if info, err := collector.CollectOpenClawInfo(); err == nil {
						hasOverview := info.Overview != nil
						hasDiagnosis := info.Diagnosis != nil
						if !hasOverview || !hasDiagnosis {
							logger.Info("openclaw parsed", "overview", hasOverview, "diagnosis", hasDiagnosis)
						}
						if b, err := json.Marshal(info); err == nil {
							sum := sha1.Sum(b)
							currentHash := fmt.Sprintf("%x", sum[:])
							if intervalDue || agentsDue || messageDue || currentHash != lastExtraHash {
								s := string(b)
								extraData = &s
								lastExtraHash = currentHash
							}
							if fp := collector.AgentsFingerprint(info.Agents); fp != "" {
								lastAgentsFingerprint = fp
							} else if currentAgentsFingerprint != "" {
								lastAgentsFingerprint = currentAgentsFingerprint
							}
							if agentsDue {
								lastAgentsTriggeredAt = time.Now()
							}
							if messageDue {
								lastMessageTriggeredAt = time.Now()
							}
						}
					} else {
						logger.Warn("collect openclaw info failed", "err", err)
						if fallback := collector.CollectAgentsOnlyInfo(); fallback != nil {
							if b, marshalErr := json.Marshal(fallback); marshalErr == nil {
								sum := sha1.Sum(b)
								currentHash := fmt.Sprintf("%x", sum[:])
								if intervalDue || agentsDue || messageDue || currentHash != lastExtraHash {
									s := string(b)
									extraData = &s
									lastExtraHash = currentHash
								}
								if fp := collector.AgentsFingerprint(fallback.Agents); fp != "" {
									lastAgentsFingerprint = fp
								} else if currentAgentsFingerprint != "" {
									lastAgentsFingerprint = currentAgentsFingerprint
								}
								if agentsDue {
									lastAgentsTriggeredAt = time.Now()
								}
								if messageDue {
									lastMessageTriggeredAt = time.Now()
								}
							}
						}
					}
				}
				_, err := up.Heartbeat(deviceInfo, device.AgentVersion, 1, extraData)
				if err != nil {
					logger.Warn("heartbeat failed", "err", err)
					if c != nil {
						// optional offline hint
					}
				}
			}
		}
	}()

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

	if cmdBroker != nil {
		wg.Add(1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("command consumer goroutine panic", "recover", r)
				}
			}()
			defer wg.Done()
			var acker transport.Acknowledger
			var progress transport.ProgressReporter
			if mqttBrk, ok := cmdBroker.(*transport.MQTTBroker); ok {
				acker = mqttBrk
				progress = mqttBrk
			}
			consumer := command.NewConsumer(cmdBroker, deviceID, resultReporter, nodeIdentity.PrivateKey, acker, progress)
			consumer.Run(ctx)
		}()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	logger.Info("shutting down")
	cancel()
	wg.Wait()
	if rdb != nil {
		_ = rdb.Close()
	}
	logger.Info("exit")
	return nil
}

func runPairingCodeCommand(args []string) error {
	fs := flag.NewFlagSet("pairing-code", flag.ContinueOnError)
	configPath := fs.String("config", "", "config file path")
	jsonOutput := fs.Bool("json", false, "print machine-readable output")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, nodeIdentity, err := loadRuntime(*configPath)
	if err != nil {
		return err
	}
	apiKey, _ := device.LoadAPIKey(cfg.Device.APIKeyFile)
	if apiKey != "" {
		if *jsonOutput {
			return printJSON(map[string]any{
				"status":  "paired",
				"node_id": nodeIdentity.NodeID,
			})
		}
		fmt.Println("==================================================")
		fmt.Println("设备已绑定")
		fmt.Println("==================================================")
		fmt.Println("这台设备已经完成绑定，无需再次使用配对码。")
		return nil
	}

	deviceInfo, err := device.Collect(nodeIdentity.NodeID)
	if err != nil {
		return fmt.Errorf("collect device info: %w", err)
	}

	cli := client.New(cfg.Server.URL, cfg.Server.Timeout, retryConfig(cfg))
	statusInfo, err := pairing.GetPairingStatus(cli, nodeIdentity.NodeID)
	if err != nil {
		statusInfo = nil
	}

	var codeInfo *pairing.PairingCodeInfo
	if statusInfo != nil && statusInfo.Status == "pending" && statusInfo.PairingCode != "" && statusInfo.ExpiresIn > 0 {
		codeInfo = &pairing.PairingCodeInfo{
			NodeID:      nodeIdentity.NodeID,
			Hostname:    deviceInfo.Hostname,
			PairingCode: statusInfo.PairingCode,
			ExpiresIn:   statusInfo.ExpiresIn,
			ExpiresAt:   statusInfo.ExpiresAt,
			Status:      statusInfo.Status,
		}
	} else {
		codeInfo, err = pairing.RequestPairingCode(cli, nodeIdentity, deviceInfo.Hostname, deviceInfo.OSVersion)
		if err != nil {
			return err
		}
	}

	if *jsonOutput {
		return printJSON(codeInfo)
	}

	fmt.Println("==================================================")
	fmt.Println("当前配对码")
	fmt.Println("==================================================")
	fmt.Printf("设备:         %s\n", deviceInfo.Hostname)
	fmt.Println("")
	fmt.Println("******************** 配对码 ********************")
	fmt.Printf("                 %s\n", codeInfo.PairingCode)
	fmt.Println("************************************************")
	if codeInfo.ExpiresIn > 0 {
		minutes := codeInfo.ExpiresIn / 60
		if minutes < 1 {
			minutes = 1
		}
		fmt.Printf("有效期:       %d 分钟\n", minutes)
	}
	fmt.Println("")
	fmt.Println("使用方法:")
	fmt.Println("1. 打开 Web 或 App")
	fmt.Println("2. 进入“添加设备”页面")
	fmt.Println("3. 输入上面的配对码")
	fmt.Println("4. 绑定成功后，即可管理这台设备")
	return nil
}

func runStatusCommand(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	configPath := fs.String("config", "", "config file path")
	jsonOutput := fs.Bool("json", false, "print machine-readable output")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, nodeIdentity, err := loadRuntime(*configPath)
	if err != nil {
		return err
	}
	apiKey, _ := device.LoadAPIKey(cfg.Device.APIKeyFile)
	status := "unpaired"
	if apiKey != "" {
		status = "paired"
	}
	payload := map[string]any{
		"status":     status,
		"node_id":    nodeIdentity.NodeID,
		"server_url": cfg.Server.URL,
		"data_dir":   cfg.Device.DataDir,
	}
	if *jsonOutput {
		return printJSON(payload)
	}
	fmt.Printf("状态: %s\n", status)
	fmt.Printf("Node ID: %s\n", nodeIdentity.NodeID)
	fmt.Printf("Server: %s\n", cfg.Server.URL)
	fmt.Printf("Data Dir: %s\n", cfg.Device.DataDir)
	return nil
}

func loadRuntime(configPath string) (*config.Config, *identity.Identity, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, nil, fmt.Errorf("load config: %w", err)
	}
	nodeIdentity, err := identity.LoadOrCreate(cfg.Device.DataDir)
	if err != nil {
		return nil, nil, fmt.Errorf("init identity: %w", err)
	}
	return cfg, nodeIdentity, nil
}

func retryConfig(cfg *config.Config) client.RetryConfig {
	return client.RetryConfig{
		MaxAttempts:     cfg.Retry.MaxAttempts,
		InitialInterval: time.Duration(cfg.Retry.InitialInterval) * time.Second,
		MaxInterval:     time.Duration(cfg.Retry.MaxInterval) * time.Second,
	}
}

func printJSON(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}
