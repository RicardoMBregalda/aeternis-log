package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all application configuration
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	MongoDB  MongoDBConfig  `yaml:"mongodb"`
	Redis    RedisConfig    `yaml:"redis"`
	Fabric   FabricConfig   `yaml:"fabric"`
	WAL      WALConfig      `yaml:"wal"`
	Batching BatchingConfig `yaml:"batching"`
	Logging  LoggingConfig  `yaml:"logging"`
	Metrics  MetricsConfig  `yaml:"metrics"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Host            string        `yaml:"host"`
	Port            int           `yaml:"port"`
	Debug           bool          `yaml:"debug"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

// MongoDBConfig holds MongoDB connection configuration
type MongoDBConfig struct {
	URL                        string        `yaml:"url"`
	Database                   string        `yaml:"database"`
	Collection                 string        `yaml:"collection"`
	SyncControlCollection      string        `yaml:"sync_control_collection"`
	MinPoolSize                int           `yaml:"min_pool_size"`
	MaxPoolSize                int           `yaml:"max_pool_size"`
	MaxIdleTimeMS              int           `yaml:"max_idle_time_ms"`
	ServerSelectionTimeoutMS   int           `yaml:"server_selection_timeout_ms"`
	ConnectTimeout             time.Duration `yaml:"connect_timeout"`
	SocketTimeout              time.Duration `yaml:"socket_timeout"`
}

// RedisConfig holds Redis configuration
type RedisConfig struct {
	Host         string        `yaml:"host"`
	Port         int           `yaml:"port"`
	Password     string        `yaml:"password"`
	DB           int           `yaml:"db"`
	PoolSize     int           `yaml:"pool_size"`
	MinIdleConns int           `yaml:"min_idle_conns"`
	MaxRetries   int           `yaml:"max_retries"`
	DialTimeout  time.Duration `yaml:"dial_timeout"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	CacheTTL     int           `yaml:"cache_ttl"` // seconds
	CacheEnabled bool          `yaml:"cache_enabled"`
}

// FabricConfig holds Hyperledger Fabric configuration
type FabricConfig struct {
	APIURL         string        `yaml:"api_url"`
	Channel        string        `yaml:"channel"`
	Chaincode      string        `yaml:"chaincode"`
	SyncEnabled    bool          `yaml:"sync_enabled"`
	SyncMaxWorkers int           `yaml:"sync_max_workers"`
	InvokeTimeout  time.Duration `yaml:"invoke_timeout"`
	QueryTimeout   time.Duration `yaml:"query_timeout"`
}

// WALConfig holds Write-Ahead Log configuration
type WALConfig struct {
	Enabled         bool          `yaml:"enabled"`
	Directory       string        `yaml:"directory"`
	CheckInterval   time.Duration `yaml:"check_interval"`
	MaxFileSizeMB   int           `yaml:"max_file_size_mb"`
	RotationEnabled bool          `yaml:"rotation_enabled"`
	RetentionDays   int           `yaml:"retention_days"`
}

// BatchingConfig holds Merkle Tree batching configuration
type BatchingConfig struct {
	Enabled              bool          `yaml:"enabled"`
	AutoBatchSize        int           `yaml:"auto_batch_size"`
	AutoBatchInterval    time.Duration `yaml:"auto_batch_interval"`
	BatchExecutorWorkers int           `yaml:"batch_executor_workers"`
	VerificationEnabled  bool          `yaml:"verification_enabled"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level            string `yaml:"level"`
	Format           string `yaml:"format"` // json or console
	Output           string `yaml:"output"` // stdout or file path
	EnableCaller     bool   `yaml:"enable_caller"`
	EnableStacktrace bool   `yaml:"enable_stacktrace"`
}

// MetricsConfig holds Prometheus metrics configuration
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Port    int    `yaml:"port"`
	Path    string `yaml:"path"`
}

// LoadConfig loads configuration from file and environment variables
func LoadConfig(configPath string) (*Config, error) {
	// Default configuration
	config := &Config{
		Server: ServerConfig{
			Host:            "0.0.0.0",
			Port:            5001,
			Debug:           false,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    30 * time.Second,
			ShutdownTimeout: 10 * time.Second,
		},
		MongoDB: MongoDBConfig{
			URL:                       "mongodb://localhost:27017",
			Database:                  "logdb",
			Collection:                "logs",
			SyncControlCollection:     "sync_control",
			MinPoolSize:               10,
			MaxPoolSize:               100,
			MaxIdleTimeMS:             300000,
			ServerSelectionTimeoutMS:  5000,
			ConnectTimeout:            10 * time.Second,
			SocketTimeout:             30 * time.Second,
		},
		Redis: RedisConfig{
			Host:         "localhost",
			Port:         6379,
			DB:           0,
			PoolSize:     50,
			MinIdleConns: 10,
			MaxRetries:   3,
			DialTimeout:  5 * time.Second,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
			CacheTTL:     600,
			CacheEnabled: true,
		},
		Fabric: FabricConfig{
			APIURL:         "http://localhost:4000",
			Channel:        "logchannel",
			Chaincode:      "logchaincode",
			SyncEnabled:    true,
			SyncMaxWorkers: 10,
			InvokeTimeout:  30 * time.Second,
			QueryTimeout:   10 * time.Second,
		},
		WAL: WALConfig{
			Enabled:         true,
			Directory:       "/var/log/tcc-wal",
			CheckInterval:   5 * time.Second,
			MaxFileSizeMB:   100,
			RotationEnabled: true,
			RetentionDays:   7,
		},
		Batching: BatchingConfig{
			Enabled:              true,
			AutoBatchSize:        100,
			AutoBatchInterval:    30 * time.Second,
			BatchExecutorWorkers: 5,
			VerificationEnabled:  true,
		},
		Logging: LoggingConfig{
			Level:            "info",
			Format:           "json",
			Output:           "stdout",
			EnableCaller:     true,
			EnableStacktrace: false,
		},
		Metrics: MetricsConfig{
			Enabled: true,
			Port:    9090,
			Path:    "/metrics",
		},
	}

	// Load from YAML file if exists
	if configPath != "" {
		if err := loadFromFile(configPath, config); err != nil {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}
	}

	// Override with environment variables
	overrideFromEnv(config)

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
}

// loadFromFile loads configuration from YAML file
func loadFromFile(path string, config *Config) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(config); err != nil {
		return err
	}

	return nil
}

// overrideFromEnv overrides configuration with environment variables
func overrideFromEnv(config *Config) {
	// Server
	if val := os.Getenv("SERVER_HOST"); val != "" {
		config.Server.Host = val
	}
	if val := os.Getenv("SERVER_PORT"); val != "" {
		if port, err := strconv.Atoi(val); err == nil {
			config.Server.Port = port
		}
	}
	if val := os.Getenv("SERVER_DEBUG"); val != "" {
		config.Server.Debug = val == "true"
	}

	// MongoDB
	if val := os.Getenv("MONGO_URL"); val != "" {
		config.MongoDB.URL = val
	}
	if val := os.Getenv("MONGO_DATABASE"); val != "" {
		config.MongoDB.Database = val
	}
	if val := os.Getenv("MONGO_COLLECTION"); val != "" {
		config.MongoDB.Collection = val
	}
	if val := os.Getenv("MONGO_MIN_POOL_SIZE"); val != "" {
		if size, err := strconv.Atoi(val); err == nil {
			config.MongoDB.MinPoolSize = size
		}
	}
	if val := os.Getenv("MONGO_MAX_POOL_SIZE"); val != "" {
		if size, err := strconv.Atoi(val); err == nil {
			config.MongoDB.MaxPoolSize = size
		}
	}

	// Redis
	if val := os.Getenv("REDIS_HOST"); val != "" {
		config.Redis.Host = val
	}
	if val := os.Getenv("REDIS_PORT"); val != "" {
		if port, err := strconv.Atoi(val); err == nil {
			config.Redis.Port = port
		}
	}
	if val := os.Getenv("REDIS_PASSWORD"); val != "" {
		config.Redis.Password = val
	}
	if val := os.Getenv("REDIS_DB"); val != "" {
		if db, err := strconv.Atoi(val); err == nil {
			config.Redis.DB = db
		}
	}
	if val := os.Getenv("REDIS_CACHE_ENABLED"); val != "" {
		config.Redis.CacheEnabled = val == "true"
	}
	if val := os.Getenv("REDIS_CACHE_TTL"); val != "" {
		if ttl, err := strconv.Atoi(val); err == nil {
			config.Redis.CacheTTL = ttl
		}
	}

	// Fabric
	if val := os.Getenv("FABRIC_API_URL"); val != "" {
		config.Fabric.APIURL = val
	}
	if val := os.Getenv("FABRIC_CHANNEL"); val != "" {
		config.Fabric.Channel = val
	}
	if val := os.Getenv("FABRIC_CHAINCODE"); val != "" {
		config.Fabric.Chaincode = val
	}
	if val := os.Getenv("FABRIC_SYNC_ENABLED"); val != "" {
		config.Fabric.SyncEnabled = val == "true"
	}

	// WAL
	if val := os.Getenv("WAL_ENABLED"); val != "" {
		config.WAL.Enabled = val == "true"
	}
	if val := os.Getenv("WAL_DIRECTORY"); val != "" {
		config.WAL.Directory = val
	}
	if val := os.Getenv("WAL_CHECK_INTERVAL"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			config.WAL.CheckInterval = duration
		}
	}

	// Batching
	if val := os.Getenv("BATCHING_ENABLED"); val != "" {
		config.Batching.Enabled = val == "true"
	}
	if val := os.Getenv("BATCHING_AUTO_BATCH_SIZE"); val != "" {
		if size, err := strconv.Atoi(val); err == nil {
			config.Batching.AutoBatchSize = size
		}
	}
	if val := os.Getenv("BATCHING_AUTO_BATCH_INTERVAL"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			config.Batching.AutoBatchInterval = duration
		}
	}

	// Logging
	if val := os.Getenv("LOG_LEVEL"); val != "" {
		config.Logging.Level = val
	}
	if val := os.Getenv("LOG_FORMAT"); val != "" {
		config.Logging.Format = val
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate server
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}

	// Validate MongoDB
	if c.MongoDB.URL == "" {
		return fmt.Errorf("mongodb url is required")
	}
	if c.MongoDB.Database == "" {
		return fmt.Errorf("mongodb database is required")
	}
	if c.MongoDB.Collection == "" {
		return fmt.Errorf("mongodb collection is required")
	}

	// Validate Redis
	if c.Redis.Port < 1 || c.Redis.Port > 65535 {
		return fmt.Errorf("invalid redis port: %d", c.Redis.Port)
	}

	// Validate Fabric
	if c.Fabric.Channel == "" {
		return fmt.Errorf("fabric channel is required")
	}
	if c.Fabric.Chaincode == "" {
		return fmt.Errorf("fabric chaincode is required")
	}

	// Validate WAL
	if c.WAL.Enabled && c.WAL.Directory == "" {
		return fmt.Errorf("wal directory is required when wal is enabled")
	}

	// Validate Batching
	if c.Batching.Enabled && c.Batching.AutoBatchSize < 1 {
		return fmt.Errorf("invalid auto batch size: %d", c.Batching.AutoBatchSize)
	}

	// Validate Logging
	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLogLevels[c.Logging.Level] {
		return fmt.Errorf("invalid log level: %s", c.Logging.Level)
	}

	return nil
}

// GetMongoURI returns the full MongoDB connection URI
func (c *Config) GetMongoURI() string {
	return c.MongoDB.URL
}

// GetRedisAddr returns the Redis address in host:port format
func (c *Config) GetRedisAddr() string {
	return fmt.Sprintf("%s:%d", c.Redis.Host, c.Redis.Port)
}

// GetServerAddr returns the server address in host:port format
func (c *Config) GetServerAddr() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

// GetMetricsAddr returns the metrics server address
func (c *Config) GetMetricsAddr() string {
	return fmt.Sprintf(":%d", c.Metrics.Port)
}

// IsProduction returns true if running in production mode
func (c *Config) IsProduction() bool {
	return !c.Server.Debug
}
