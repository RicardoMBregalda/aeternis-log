package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
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
	Logging   LoggingConfig   `yaml:"logging"`
	Metrics   MetricsConfig   `yaml:"metrics"`
	Auth      AuthConfig      `yaml:"auth"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
	Webhook   WebhookConfig   `yaml:"webhook"`
}

// WebhookConfig configures the callback fired when a batch is anchored on the
// blockchain. The payload is signed with HMAC-SHA256 (header X-Webhook-Signature)
// when a secret is set.
type WebhookConfig struct {
	Enabled    bool          `yaml:"enabled"`
	URL        string        `yaml:"url"`
	Secret     string        `yaml:"secret"`
	Timeout    time.Duration `yaml:"timeout"`
	MaxRetries int           `yaml:"max_retries"`
}

// AuthConfig holds API authentication configuration. When enabled, requests to
// protected routes must present a valid API key (default header: X-API-Key, or
// "Authorization: Bearer <key>").
type AuthConfig struct {
	Enabled    bool     `yaml:"enabled"`
	HeaderName string   `yaml:"header_name"`
	APIKeys    []string `yaml:"api_keys"`
	// Tenants maps API keys to tenants for multi-tenant isolation. Keys under a
	// tenant resolve to that tenant; the flat api_keys above resolve to "default".
	Tenants []TenantKeys `yaml:"tenants"`
}

// TenantKeys associates a tenant id with its API keys.
type TenantKeys struct {
	ID   string   `yaml:"id"`
	Keys []string `yaml:"keys"`
}

// KeyToTenant builds a lookup from API key to tenant id.
func (a *AuthConfig) KeyToTenant() map[string]string {
	m := make(map[string]string)
	for _, k := range a.APIKeys {
		m[k] = "default"
	}
	for _, t := range a.Tenants {
		for _, k := range t.Keys {
			m[k] = t.ID
		}
	}
	return m
}

// RateLimitConfig holds per-client-IP rate limiting configuration.
type RateLimitConfig struct {
	Enabled     bool          `yaml:"enabled"`
	MaxRequests int           `yaml:"max_requests"`
	Window      time.Duration `yaml:"window"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Host            string        `yaml:"host"`
	Port            int           `yaml:"port"`
	Debug           bool          `yaml:"debug"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
	// MaxBodyBytes caps the accepted request body size (0 disables the cap).
	MaxBodyBytes int64 `yaml:"max_body_bytes"`
	// CORSAllowedOrigins is the allowlist of browser origins. "*" allows any
	// origin but disables credentialed CORS (the invalid "*"+credentials combo).
	CORSAllowedOrigins []string `yaml:"cors_allowed_origins"`
}

// MongoDBConfig holds MongoDB connection configuration
type MongoDBConfig struct {
	URL                        string        `yaml:"url"`
	Database                   string        `yaml:"database"`
	Collection                 string        `yaml:"collection"`
	RecordsCollection          string        `yaml:"records_collection"`
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

	// Transport selects how the API talks to the peer. Only "gateway" (the Fabric
	// Gateway gRPC SDK) is supported; an empty value defaults to gateway.
	Transport string `yaml:"transport"`

	// Gateway transport settings (used when Transport == "gateway"). The
	// identity is an X.509 enrollment (cert + private key) from the org MSP.
	MSPID                     string `yaml:"msp_id"`
	GatewayPeerEndpoint       string `yaml:"gateway_peer_endpoint"`
	GatewayPeerTLSCAFile      string `yaml:"gateway_peer_tls_ca_file"`
	GatewayServerNameOverride string `yaml:"gateway_server_name_override"`
	IdentityCertFile          string `yaml:"identity_cert_file"`
	IdentityKeyDir            string `yaml:"identity_key_dir"`

	// TenantChannels maps a tenant id to its own Fabric channel, raising
	// multi-tenant isolation from a "tenant" field on the shared channel to a
	// dedicated ledger per tenant. A tenant without a mapping anchors to Channel
	// (the default). Requires the channel to already exist with the chaincode
	// committed (see prod/scripts/create-tenant-channel.sh).
	TenantChannels map[string]string `yaml:"tenant_channels"`
}

// ChannelForTenant returns the Fabric channel a tenant anchors to: its dedicated
// channel when mapped, otherwise the default Channel.
func (fc *FabricConfig) ChannelForTenant(tenant string) string {
	if ch, ok := fc.TenantChannels[tenant]; ok && ch != "" {
		return ch
	}
	return fc.Channel
}

// WALConfig holds Write-Ahead Log configuration
type WALConfig struct {
	Enabled         bool          `yaml:"enabled"`
	Directory       string        `yaml:"directory"`
	CheckInterval   time.Duration `yaml:"check_interval"`
	MaxFileSizeMB   int           `yaml:"max_file_size_mb"`
	RotationEnabled bool          `yaml:"rotation_enabled"`
	RetentionDays   int           `yaml:"retention_days"`

	// Backend selects the WAL implementation:
	//   "file"  - local append-only file with fsync (single instance)
	//   "redis" - Redis Streams + consumer group (supports multiple API instances)
	// Durability in "redis" mode depends on Redis persistence: run Redis with
	// AOF enabled (appendfsync always for parity with the file fsync).
	Backend string `yaml:"backend"`

	// Redis Streams backend settings (used when Backend == "redis").
	StreamKey     string        `yaml:"stream_key"`     // stream that holds pending entries
	ConsumerGroup string        `yaml:"consumer_group"` // shared group across API instances
	ReadCount     int64         `yaml:"read_count"`     // max entries fetched per read
	BlockTimeout  time.Duration `yaml:"block_timeout"`  // XREADGROUP block duration
	ClaimMinIdle  time.Duration `yaml:"claim_min_idle"` // reclaim entries idle longer than this
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
			Host:               "0.0.0.0",
			Port:               5001,
			Debug:              false,
			ReadTimeout:        30 * time.Second,
			WriteTimeout:       30 * time.Second,
			ShutdownTimeout:    10 * time.Second,
			MaxBodyBytes:       1 << 20, // 1 MiB
			CORSAllowedOrigins: []string{"*"},
		},
		MongoDB: MongoDBConfig{
			URL:                       "mongodb://localhost:27017",
			Database:                  "logdb",
			Collection:                "logs",
			RecordsCollection:         "records",
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
			APIURL:           "http://localhost:4000",
			Channel:          "logchannel",
			Chaincode:        "logchaincode",
			SyncEnabled:      true,
			SyncMaxWorkers:   10,
			InvokeTimeout:    30 * time.Second,
			QueryTimeout:     10 * time.Second,
			Transport:                 "gateway",
			MSPID:                     "Org1MSP",
			GatewayPeerEndpoint:       "peer0.org1.example.com:7051",
			GatewayServerNameOverride: "peer0.org1.example.com",
			GatewayPeerTLSCAFile:      "/fabric-crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt",
			IdentityCertFile:          "/fabric-crypto/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp/signcerts/Admin@org1.example.com-cert.pem",
			IdentityKeyDir:            "/fabric-crypto/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp/keystore",
		},
		WAL: WALConfig{
			Enabled:         true,
			Directory:       "/var/log/aeternislog-wal",
			CheckInterval:   5 * time.Second,
			MaxFileSizeMB:   100,
			RotationEnabled: true,
			RetentionDays:   7,
			Backend:         "file",
			StreamKey:       "wal:pending:logs",
			ConsumerGroup:   "wal-processors",
			ReadCount:       128,
			BlockTimeout:    2 * time.Second,
			ClaimMinIdle:    30 * time.Second,
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
		Auth: AuthConfig{
			Enabled:    false,
			HeaderName: "X-API-Key",
		},
		RateLimit: RateLimitConfig{
			Enabled:     false,
			MaxRequests: 100,
			Window:      time.Minute,
		},
		Webhook: WebhookConfig{
			Enabled:    false,
			Timeout:    5 * time.Second,
			MaxRetries: 3,
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
	if val := os.Getenv("SERVER_MAX_BODY_BYTES"); val != "" {
		if n, err := strconv.ParseInt(val, 10, 64); err == nil {
			config.Server.MaxBodyBytes = n
		}
	}
	if val := os.Getenv("SERVER_CORS_ALLOWED_ORIGINS"); val != "" {
		if origins := splitAndTrim(val, ","); len(origins) > 0 {
			config.Server.CORSAllowedOrigins = origins
		}
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
	if val := os.Getenv("MONGO_RECORDS_COLLECTION"); val != "" {
		config.MongoDB.RecordsCollection = val
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
	if val := os.Getenv("FABRIC_TRANSPORT"); val != "" {
		config.Fabric.Transport = val
	}
	if val := os.Getenv("FABRIC_MSP_ID"); val != "" {
		config.Fabric.MSPID = val
	}
	if val := os.Getenv("FABRIC_GATEWAY_PEER_ENDPOINT"); val != "" {
		config.Fabric.GatewayPeerEndpoint = val
	}
	if val := os.Getenv("FABRIC_GATEWAY_PEER_TLS_CA_FILE"); val != "" {
		config.Fabric.GatewayPeerTLSCAFile = val
	}
	if val := os.Getenv("FABRIC_GATEWAY_SERVER_NAME_OVERRIDE"); val != "" {
		config.Fabric.GatewayServerNameOverride = val
	}
	if val := os.Getenv("FABRIC_IDENTITY_CERT_FILE"); val != "" {
		config.Fabric.IdentityCertFile = val
	}
	if val := os.Getenv("FABRIC_IDENTITY_KEY_DIR"); val != "" {
		config.Fabric.IdentityKeyDir = val
	}
	// FABRIC_TENANT_CHANNELS: comma-separated "tenant:channel" pairs, e.g.
	// "acme:acme-channel,globex:globex-channel".
	if val := os.Getenv("FABRIC_TENANT_CHANNELS"); val != "" {
		m := make(map[string]string)
		for _, pair := range splitAndTrim(val, ",") {
			kv := strings.SplitN(pair, ":", 2)
			if len(kv) == 2 {
				t := strings.TrimSpace(kv[0])
				ch := strings.TrimSpace(kv[1])
				if t != "" && ch != "" {
					m[t] = ch
				}
			}
		}
		if len(m) > 0 {
			config.Fabric.TenantChannels = m
		}
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
	if val := os.Getenv("WAL_BACKEND"); val != "" {
		config.WAL.Backend = val
	}
	if val := os.Getenv("WAL_STREAM_KEY"); val != "" {
		config.WAL.StreamKey = val
	}
	if val := os.Getenv("WAL_CONSUMER_GROUP"); val != "" {
		config.WAL.ConsumerGroup = val
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

	// Auth
	if val := os.Getenv("AUTH_ENABLED"); val != "" {
		config.Auth.Enabled = val == "true"
	}
	if val := os.Getenv("AUTH_HEADER_NAME"); val != "" {
		config.Auth.HeaderName = val
	}
	if val := os.Getenv("AUTH_API_KEYS"); val != "" {
		config.Auth.APIKeys = splitAndTrim(val, ",")
	}
	// AUTH_TENANTS lets tenant keys be injected from a Secret (not the ConfigMap).
	// Format: "tenantA:keyA1,keyA2;tenantB:keyB1". Keys may be plaintext or
	// "sha256:<hex>" hashes.
	if val := os.Getenv("AUTH_TENANTS"); val != "" {
		config.Auth.Tenants = parseTenants(val)
	}

	// Rate limiting
	if val := os.Getenv("RATE_LIMIT_ENABLED"); val != "" {
		config.RateLimit.Enabled = val == "true"
	}
	if val := os.Getenv("RATE_LIMIT_MAX_REQUESTS"); val != "" {
		if n, err := strconv.Atoi(val); err == nil {
			config.RateLimit.MaxRequests = n
		}
	}
	if val := os.Getenv("RATE_LIMIT_WINDOW"); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			config.RateLimit.Window = d
		}
	}

	// Webhook
	if val := os.Getenv("WEBHOOK_ENABLED"); val != "" {
		config.Webhook.Enabled = val == "true"
	}
	if val := os.Getenv("WEBHOOK_URL"); val != "" {
		config.Webhook.URL = val
	}
	if val := os.Getenv("WEBHOOK_SECRET"); val != "" {
		config.Webhook.Secret = val
	}
	if val := os.Getenv("WEBHOOK_TIMEOUT"); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			config.Webhook.Timeout = d
		}
	}
	if val := os.Getenv("WEBHOOK_MAX_RETRIES"); val != "" {
		if n, err := strconv.Atoi(val); err == nil {
			config.Webhook.MaxRetries = n
		}
	}
}

// splitAndTrim splits s by sep and drops empty/whitespace-only items.
// parseTenants parses the AUTH_TENANTS env format
// "tenantA:keyA1,keyA2;tenantB:keyB1" into TenantKeys. Groups or keys that are
// empty or malformed are skipped.
func parseTenants(s string) []TenantKeys {
	var out []TenantKeys
	for _, group := range strings.Split(s, ";") {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		i := strings.Index(group, ":")
		if i < 0 {
			continue
		}
		id := strings.TrimSpace(group[:i])
		keys := splitAndTrim(group[i+1:], ",")
		if id != "" && len(keys) > 0 {
			out = append(out, TenantKeys{ID: id, Keys: keys})
		}
	}
	return out
}

func splitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
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
	switch c.Fabric.Transport {
	case "", "gateway":
	default:
		return fmt.Errorf("invalid fabric transport: %q (only \"gateway\" is supported)", c.Fabric.Transport)
	}
	if c.Fabric.SyncEnabled {
		if c.Fabric.MSPID == "" || c.Fabric.GatewayPeerEndpoint == "" ||
			c.Fabric.GatewayPeerTLSCAFile == "" || c.Fabric.IdentityCertFile == "" ||
			c.Fabric.IdentityKeyDir == "" {
			return fmt.Errorf("fabric gateway transport requires msp_id, gateway_peer_endpoint, gateway_peer_tls_ca_file, identity_cert_file and identity_key_dir")
		}
	}

	// Validate WAL
	if c.WAL.Enabled {
		switch c.WAL.Backend {
		case "file":
			if c.WAL.Directory == "" {
				return fmt.Errorf("wal directory is required when wal backend is file")
			}
		case "redis":
			if c.WAL.StreamKey == "" {
				return fmt.Errorf("wal stream_key is required when wal backend is redis")
			}
			if c.WAL.ConsumerGroup == "" {
				return fmt.Errorf("wal consumer_group is required when wal backend is redis")
			}
		default:
			return fmt.Errorf("invalid wal backend: %q (must be \"file\" or \"redis\")", c.WAL.Backend)
		}
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

	// Validate auth
	if c.Auth.Enabled {
		if len(c.Auth.APIKeys) == 0 && len(c.Auth.Tenants) == 0 {
			return fmt.Errorf("auth is enabled but no api_keys or tenants are configured")
		}
		for _, t := range c.Auth.Tenants {
			if t.ID == "" || len(t.Keys) == 0 {
				return fmt.Errorf("each auth tenant requires an id and at least one key")
			}
		}
		if c.Auth.HeaderName == "" {
			return fmt.Errorf("auth header_name is required when auth is enabled")
		}
	}

	// Validate rate limiting
	if c.RateLimit.Enabled {
		if c.RateLimit.MaxRequests < 1 {
			return fmt.Errorf("invalid rate_limit max_requests: %d", c.RateLimit.MaxRequests)
		}
		if c.RateLimit.Window <= 0 {
			return fmt.Errorf("rate_limit window must be positive")
		}
	}

	// Validate webhook
	if c.Webhook.Enabled && c.Webhook.URL == "" {
		return fmt.Errorf("webhook is enabled but url is not configured")
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
