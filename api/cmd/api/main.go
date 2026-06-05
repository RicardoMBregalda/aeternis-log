package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/cache"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/database"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/fabric"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/handlers"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/logger"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/merkle"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/middleware"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/models"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/wal"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/pkg/config"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "github.com/RicardoMBregalda/tcc-log-management/go-api/docs" // swagger docs
)

// Version and build information (set via ldflags)
var (
	Version   = "dev"
	BuildTime = "unknown"
)

// @title Log Management API
// @version 1.0
// @description High-performance log management API with WAL, Merkle Tree, and Fabric integration
// @termsOfService http://swagger.io/terms/

// @contact.name Ricardo M. Bregalda
// @contact.email ricardo@example.com

// @license.name MIT
// @license.url https://opensource.org/licenses/MIT

// @host localhost:5001
// @BasePath /

// @schemes http https

// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization

func main() {
	printBanner()

	// Bootstrap logger writes to stderr until the configured logger is ready,
	// so failures during configuration loading are never silent.
	boot := zerolog.New(os.Stderr).With().Timestamp().Logger()

	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		boot.Fatal().Err(err).Msg("failed to load configuration")
	}

	lg, logCloser, err := logger.Init(logger.Config{
		Level:        cfg.Logging.Level,
		Format:       cfg.Logging.Format,
		Output:       cfg.Logging.Output,
		EnableCaller: cfg.Logging.EnableCaller,
	})
	if err != nil {
		boot.Fatal().Err(err).Msg("failed to initialize logger")
	}
	if logCloser != nil {
		defer logCloser.Close()
	}
	lg.Info().
		Str("version", Version).
		Str("build_time", BuildTime).
		Str("log_level", cfg.Logging.Level).
		Str("log_format", cfg.Logging.Format).
		Msg("configuration loaded, logger initialized")

	// MongoDB ------------------------------------------------------------
	mongoClient, err := database.ConnectWithRetry(&cfg.MongoDB, 5)
	if err != nil {
		lg.Fatal().Err(err).Msg("failed to connect to MongoDB")
	}
	defer mongoClient.Close(context.Background())
	lg.Info().Str("database", cfg.MongoDB.Database).Msg("MongoDB connected")

	collections := database.NewCollections(mongoClient)

	// Redis (optional, graceful degradation) -----------------------------
	redisCache, err := cache.NewRedisClient(&cfg.Redis)
	if err != nil {
		lg.Warn().Err(err).Msg("failed to create Redis client, continuing without cache")
		redisCache = &cache.RedisCache{Enabled: false, Config: &cfg.Redis}
	} else if redisCache.Enabled {
		defer redisCache.Close()
		lg.Info().Msg("Redis cache connected")
	} else {
		lg.Warn().Msg("Redis cache disabled (graceful degradation)")
	}

	// Write-Ahead Log ----------------------------------------------------
	insertCallback := func(logEntry *models.Log) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return collections.InsertLog(ctx, logEntry)
	}

	walInstance, err := wal.New(&cfg.WAL, redisCache.Client)
	if err != nil {
		lg.Warn().Err(err).Msg("failed to create WAL, durability guarantees disabled")
		walInstance = wal.NoopWAL{}
	}
	if cfg.WAL.Enabled {
		walInstance.StartProcessor(insertCallback)
		lg.Info().
			Str("backend", cfg.WAL.Backend).
			Str("directory", cfg.WAL.Directory).
			Msg("WAL processor started")
	} else {
		lg.Warn().Msg("WAL disabled")
	}
	defer walInstance.StopProcessor()

	// Fabric -------------------------------------------------------------
	fabricClient, err := fabric.NewFabricClient(&cfg.Fabric)
	if err != nil {
		lg.Fatal().Err(err).Msg("failed to create Fabric client")
	}
	defer fabricClient.Close()
	if cfg.Fabric.SyncEnabled {
		lg.Info().
			Str("channel", cfg.Fabric.Channel).
			Str("transport", cfg.Fabric.Transport).
			Msg("Fabric client initialized")
	} else {
		lg.Warn().Msg("Fabric sync disabled")
	}

	// Merkle batch processor ---------------------------------------------
	batchProcessor := merkle.NewBatchProcessor(collections, fabricClient, &cfg.Batching)
	if err := batchProcessor.Start(context.Background()); err != nil {
		lg.Fatal().Err(err).Msg("failed to start batch processor")
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := batchProcessor.Stop(stopCtx); err != nil {
			lg.Error().Err(err).Msg("error stopping batch processor")
		}
	}()
	lg.Info().Int("workers", cfg.Batching.BatchExecutorWorkers).Msg("batch processor started")

	// HTTP router --------------------------------------------------------
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.RequestID())
	router.Use(middleware.RequestLogger())
	router.Use(middleware.CORS())
	router.Use(middleware.SecurityHeaders())
	if cfg.RateLimit.Enabled {
		router.Use(middleware.RateLimiter(cfg.RateLimit.MaxRequests, cfg.RateLimit.Window))
		lg.Info().
			Int("max_requests", cfg.RateLimit.MaxRequests).
			Dur("window", cfg.RateLimit.Window).
			Msg("rate limiting enabled")
	}

	// API key authentication for protected routes (nil when disabled).
	var authMW gin.HandlerFunc
	if cfg.Auth.Enabled {
		authMW = middleware.APIKeyAuth(cfg.Auth.HeaderName, cfg.Auth.APIKeys)
		lg.Info().
			Str("header", cfg.Auth.HeaderName).
			Int("keys", len(cfg.Auth.APIKeys)).
			Msg("API key authentication enabled")
	} else {
		lg.Warn().Msg("API authentication disabled (all requests accepted)")
	}

	healthHandler := handlers.NewHealthHandler(mongoClient, collections, redisCache, fabricClient, batchProcessor, Version, BuildTime)
	logHandler := handlers.NewLogHandler(collections, redisCache, walInstance)
	recordHandler := handlers.NewRecordHandler(collections)
	merkleHandler := handlers.NewMerkleHandler(batchProcessor, redisCache)
	walHandler := handlers.NewWALHandler(walInstance)
	statsHandler := handlers.NewStatsHandler(collections, mongoClient, redisCache, fabricClient, batchProcessor, walInstance)

	registerRoutes(router, authMW, healthHandler, logHandler, recordHandler, merkleHandler, walHandler, statsHandler)

	srv := &http.Server{
		Addr:         cfg.GetServerAddr(),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		lg.Info().
			Str("addr", srv.Addr).
			Str("docs", fmt.Sprintf("http://%s/swagger/index.html", srv.Addr)).
			Msg("HTTP server starting")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			lg.Fatal().Err(err).Msg("failed to start server")
		}
	}()

	// Graceful shutdown --------------------------------------------------
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	lg.Info().Str("signal", sig.String()).Msg("shutdown signal received")

	// Stop accepting new requests first; the deferred stops above then drain
	// the batch processor, WAL, Redis and MongoDB in reverse order.
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		lg.Error().Err(err).Msg("server forced to shutdown")
	}
	lg.Info().Msg("server exited gracefully")
}

// registerRoutes registers all API routes. authMW, when non-nil, protects the
// data routes (logs, merkle, wal, stats); health, root and swagger stay open.
func registerRoutes(
	router *gin.Engine,
	authMW gin.HandlerFunc,
	healthHandler *handlers.HealthHandler,
	logHandler *handlers.LogHandler,
	recordHandler *handlers.RecordHandler,
	merkleHandler *handlers.MerkleHandler,
	walHandler *handlers.WALHandler,
	statsHandler *handlers.StatsHandler,
) {
	router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"name":    "Go Log Management API",
			"version": Version,
			"docs":    "/swagger/index.html",
		})
	})

	router.GET("/health", healthHandler.HealthCheck)

	v1 := router.Group("/api/v1")
	{
		v1.GET("/ping", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "pong", "version": Version})
		})
	}

	// Generic, domain-scoped records: /api/v1/{domain}/records
	records := router.Group("/api/v1/:domain/records")
	if authMW != nil {
		records.Use(authMW)
	}
	{
		records.POST("", recordHandler.CreateRecord)
		records.GET("", recordHandler.ListRecords)
		records.GET("/:id", recordHandler.GetRecord)
		records.DELETE("/:id", recordHandler.DeleteRecord)
	}

	logs := router.Group("/logs")
	if authMW != nil {
		logs.Use(authMW)
	}
	{
		logs.POST("", logHandler.CreateLog)
		logs.GET("", logHandler.GetLogs)
		logs.GET("/:id", logHandler.GetLogByID)
		logs.DELETE("/:id", logHandler.DeleteLog)
	}

	merkleGroup := router.Group("/merkle")
	if authMW != nil {
		merkleGroup.Use(authMW)
	}
	{
		merkleGroup.POST("/batch", merkleHandler.CreateBatch)
		merkleGroup.GET("/batch/:id", merkleHandler.GetBatch)
		merkleGroup.POST("/verify/:id", merkleHandler.VerifyBatch)
		merkleGroup.GET("/batches", merkleHandler.ListBatches)
		merkleGroup.GET("/stats", merkleHandler.GetBatchStats)
		merkleGroup.POST("/force-batch", merkleHandler.ForceBatch)
	}

	walGroup := router.Group("/wal")
	if authMW != nil {
		walGroup.Use(authMW)
	}
	{
		walGroup.GET("/stats", walHandler.GetStats)
		walGroup.POST("/force-process", walHandler.ForceProcess)
		walGroup.GET("/health", walHandler.GetHealth)
	}

	stats := router.Group("/stats")
	if authMW != nil {
		stats.Use(authMW)
	}
	{
		stats.GET("", statsHandler.GetStats)
		stats.GET("/logs", statsHandler.GetLogStats)
		stats.GET("/sync", statsHandler.GetSyncStats)
	}

	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}

// printBanner prints the application banner.
func printBanner() {
	banner := `
==============================================================
  Log Management API
  Version: %-10s
  Build:   %-15s
  Tamper-evident log anchoring with Merkle Tree + WAL
==============================================================
`
	fmt.Printf(banner, Version, BuildTime)
}
