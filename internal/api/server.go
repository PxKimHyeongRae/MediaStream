package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/cctv3/internal/cctv"
	"go.uber.org/zap"
)

// Server는 HTTP API 서버입니다
type Server struct {
	logger     *zap.Logger
	httpServer *http.Server
	router     *gin.Engine
	port       int

	// 핸들러
	healthHandler    func() map[string]interface{}
	statsHandler     func() map[string]interface{}
	websocketHandler func(http.ResponseWriter, *http.Request)
	cctvManager      interface {
		GetCCTVs() map[string]cctv.CCTVStream
	}
}

// ServerConfig는 API 서버 설정
type ServerConfig struct {
	Port             int
	Production       bool
	Logger           *zap.Logger
	HealthHandler    func() map[string]interface{}
	StatsHandler     func() map[string]interface{}
	WebSocketHandler func(http.ResponseWriter, *http.Request)
	CCTVManager      interface {
		GetCCTVs() map[string]cctv.CCTVStream
	}
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
		statsHandler:     config.StatsHandler,
		websocketHandler: config.WebSocketHandler,
		cctvManager:      config.CCTVManager,
	}

	server.setupRoutes()

	return server
}

// setupRoutes는 라우트를 설정합니다
func (s *Server) setupRoutes() {
	// Health check
	s.router.GET("/health", s.handleHealth)

	// API v1 - simplified for external API integration
	v1 := s.router.Group("/api/v1")
	{
		v1.GET("/health", s.handleHealth)
		v1.GET("/stats", s.handleStats)
	}

	// API v3 - mediaMTX style endpoints
	v3 := s.router.Group("/v3/config/paths")
	{
		v3.GET("/list", s.handlePathsList)
		v3.POST("/add/:name", s.handlePathAdd)
		v3.DELETE("/delete/:name", s.handlePathDelete)
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

// handleStats는 서버 통계를 반환합니다
func (s *Server) handleStats(c *gin.Context) {
	var stats map[string]interface{}

	if s.statsHandler != nil {
		stats = s.statsHandler()
	} else {
		stats = map[string]interface{}{
			"uptime":  "0h 0m 0s",
			"streams": 0,
			"clients": 0,
		}
	}

	c.JSON(http.StatusOK, stats)
}

// handlePathsList는 mediaMTX 스타일의 paths 목록을 반환합니다
func (s *Server) handlePathsList(c *gin.Context) {
	if s.cctvManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "CCTV manager not available",
		})
		return
	}

	cctvs := s.cctvManager.GetCCTVs()

	// mediaMTX 스타일 응답으로 변환
	items := make([]gin.H, 0, len(cctvs))
	for _, cctv := range cctvs {
		items = append(items, gin.H{
			"name":   cctv.Name,
			"source": cctv.URL,
		})
	}

	response := gin.H{
		"pageCount": 1,
		"itemCount": len(items),
		"items":     items,
	}

	c.JSON(http.StatusOK, response)
}

// handlePathAdd는 새로운 path를 추가합니다
func (s *Server) handlePathAdd(c *gin.Context) {
	pathName := c.Param("name")

	if s.cctvManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "CCTV manager not available",
		})
		return
	}

	// 요청 바디 파싱
	var request struct {
		Source         string `json:"source" binding:"required"`
		SourceOnDemand bool   `json:"sourceOnDemand"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request body: " + err.Error(),
		})
		return
	}

	s.logger.Info("Path add requested",
		zap.String("name", pathName),
		zap.String("source", request.Source),
		zap.Bool("sourceOnDemand", request.SourceOnDemand),
	)

	// 현재는 외부 API 기반이므로 실제 추가는 지원하지 않음
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": "Path addition not supported in API-based mode",
	})
}

// handlePathDelete는 path를 삭제합니다
func (s *Server) handlePathDelete(c *gin.Context) {
	pathName := c.Param("name")

	if s.cctvManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "CCTV manager not available",
		})
		return
	}

	s.logger.Info("Path delete requested", zap.String("name", pathName))

	// 현재는 외부 API 기반이므로 실제 삭제는 지원하지 않음
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": "Path deletion not supported in API-based mode",
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
