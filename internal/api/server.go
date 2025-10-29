package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Server는 HTTP API 서버입니다
type Server struct {
	logger     *zap.Logger
	httpServer *http.Server
	router     *gin.Engine
	port       int

	// 핸들러
	healthHandler      func() map[string]interface{}
	streamsHandler     func() []map[string]interface{}
	websocketHandler   func(http.ResponseWriter, *http.Request)
}

// ServerConfig는 API 서버 설정
type ServerConfig struct {
	Port               int
	Production         bool
	Logger             *zap.Logger
	HealthHandler      func() map[string]interface{}
	StreamsHandler     func() []map[string]interface{}
	WebSocketHandler   func(http.ResponseWriter, *http.Request)
}

// NewServer는 새로운 API 서버를 생성합니다
func NewServer(config ServerConfig) *Server {
	if !config.Production {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())
	router.Use(loggerMiddleware(config.Logger))

	server := &Server{
		logger:           config.Logger,
		router:           router,
		port:             config.Port,
		healthHandler:    config.HealthHandler,
		streamsHandler:   config.StreamsHandler,
		websocketHandler: config.WebSocketHandler,
	}

	server.setupRoutes()

	return server
}

// setupRoutes는 라우트를 설정합니다
func (s *Server) setupRoutes() {
	// Health check
	s.router.GET("/health", s.handleHealth)

	// API v1
	v1 := s.router.Group("/api/v1")
	{
		v1.GET("/streams", s.handleStreams)
		v1.GET("/stats", s.handleStats)
	}

	// WebSocket signaling
	s.router.GET("/ws", gin.WrapF(s.websocketHandler))

	// Static files
	s.router.Static("/static", "./web/static")
	s.router.StaticFile("/", "./web/static/index.html")
}

// Start는 API 서버를 시작합니다
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.port)

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s.logger.Info("Starting API server",
		zap.String("addr", addr),
	)

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("API server error", zap.Error(err))
		}
	}()

	return nil
}

// Stop은 API 서버를 종료합니다
func (s *Server) Stop() error {
	s.logger.Info("Stopping API server")

	if s.httpServer != nil {
		return s.httpServer.Close()
	}

	return nil
}

// handleHealth는 헬스 체크를 처리합니다
func (s *Server) handleHealth(c *gin.Context) {
	var health map[string]interface{}

	if s.healthHandler != nil {
		health = s.healthHandler()
	} else {
		health = map[string]interface{}{
			"status": "ok",
			"time":   time.Now().UTC(),
		}
	}

	c.JSON(http.StatusOK, health)
}

// handleStreams는 스트림 목록을 반환합니다
func (s *Server) handleStreams(c *gin.Context) {
	var streams []map[string]interface{}

	if s.streamsHandler != nil {
		streams = s.streamsHandler()
	} else {
		streams = []map[string]interface{}{}
	}

	c.JSON(http.StatusOK, gin.H{
		"streams": streams,
	})
}

// handleStats는 서버 통계를 반환합니다
func (s *Server) handleStats(c *gin.Context) {
	// TODO: 실제 통계 수집
	c.JSON(http.StatusOK, gin.H{
		"uptime": "0h 0m 0s",
		"streams": 0,
		"clients": 0,
	})
}

// corsMiddleware는 CORS 미들웨어입니다
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// loggerMiddleware는 로깅 미들웨어입니다
func loggerMiddleware(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)

		logger.Info("HTTP request",
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", latency),
			zap.String("ip", c.ClientIP()),
		)
	}
}
