package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/cctv3/internal/cctv"
	"github.com/yourusername/cctv3/internal/hls"
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
	statsHandler       func() map[string]interface{}
	websocketHandler   func(http.ResponseWriter, *http.Request)
	startStreamHandler func(streamID string) error // 스트림 시작 콜백
	cctvManager        cctv.Provider
	hlsManager         *hls.Manager
}

// ServerConfig는 API 서버 설정
type ServerConfig struct {
	Port               int
	Production         bool
	Logger             *zap.Logger
	HealthHandler      func() map[string]interface{}
	StatsHandler       func() map[string]interface{}
	WebSocketHandler   func(http.ResponseWriter, *http.Request)
	StartStreamHandler func(streamID string) error // 스트림 시작 콜백
	CCTVManager        cctv.Provider
	HLSManager         *hls.Manager
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
		logger:             config.Logger,
		router:             router,
		port:               config.Port,
		healthHandler:      config.HealthHandler,
		statsHandler:       config.StatsHandler,
		websocketHandler:   config.WebSocketHandler,
		startStreamHandler: config.StartStreamHandler,
		cctvManager:        config.CCTVManager,
		hlsManager:         config.HLSManager,
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
		v1.POST("/sync", s.handleSync)

		// HLS API endpoints
		hlsGroup := v1.Group("/hls")
		{
			hlsGroup.GET("/streams", s.handleHLSStreamsList)
			hlsGroup.GET("/streams/:id", s.handleHLSStreamInfo)
			hlsGroup.GET("/streams/:id/stats", s.handleHLSStreamStats)
		}
	}

	// API v3 - mediaMTX style endpoints
	v3 := s.router.Group("/v3/config/paths")
	{
		v3.GET("/list", s.handlePathsList)
		v3.POST("/add/:name", s.handlePathAdd)
		v3.DELETE("/delete/:name", s.handlePathDelete)
	}

	// HLS 플레이리스트 및 세그먼트 서빙
	s.router.GET("/hls/:streamId/index.m3u8", s.handleHLSPlaylist)
	s.router.GET("/hls/:streamId/:segment", s.handleHLSSegment)

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

// handleSync는 CCTV 목록 수동 동기화를 처리합니다
func (s *Server) handleSync(c *gin.Context) {
	if s.cctvManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "CCTV manager not available",
		})
		return
	}

	s.logger.Info("Manual sync requested")

	if err := s.cctvManager.ManualSync(); err != nil {
		s.logger.Error("Manual sync failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Sync failed: " + err.Error(),
		})
		return
	}

	// 동기화 완료 후 업데이트된 CCTV 목록 반환
	cctvs := s.cctvManager.GetCCTVs()

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "CCTV list synchronized successfully",
		"count":   len(cctvs),
	})
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

// handleHLSStreamsList는 모든 HLS 스트림 목록을 반환합니다
func (s *Server) handleHLSStreamsList(c *gin.Context) {
	if s.hlsManager == nil || !s.hlsManager.IsEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "HLS is not enabled",
		})
		return
	}

	streams := s.hlsManager.GetAllStreams()

	c.JSON(http.StatusOK, gin.H{
		"streams": streams,
		"count":   len(streams),
	})
}

// handleHLSStreamInfo는 특정 HLS 스트림 정보를 반환합니다
func (s *Server) handleHLSStreamInfo(c *gin.Context) {
	streamID := c.Param("id")

	if s.hlsManager == nil || !s.hlsManager.IsEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "HLS is not enabled handleHLSStreamInfo",
		})
		return
	}

	info, err := s.hlsManager.GetStreamInfo(streamID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("Stream %s not found", streamID),
		})
		return
	}

	c.JSON(http.StatusOK, info)
}

// handleHLSStreamStats는 특정 HLS 스트림 통계를 반환합니다
func (s *Server) handleHLSStreamStats(c *gin.Context) {
	streamID := c.Param("id")

	if s.hlsManager == nil || !s.hlsManager.IsEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "HLS is not enabled",
		})
		return
	}

	stats, err := s.hlsManager.GetStats(streamID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("Stream %s not found", streamID),
		})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// handleHLSPlaylist는 M3U8 플레이리스트를 서빙합니다
func (s *Server) handleHLSPlaylist(c *gin.Context) {
	streamID := c.Param("streamId")

	if s.hlsManager == nil || !s.hlsManager.IsEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "HLS is not enabled handleHLSPlaylist" + fmt.Sprintf("%v", s.hlsManager) + fmt.Sprintf("%v", s.hlsManager.IsEnabled()),
		})
		return
	}

	muxer, exists := s.hlsManager.GetMuxer(streamID)
	if !exists {
		// Muxer가 없으면 스트림을 자동으로 시작
		s.logger.Info("HLS Muxer not found, attempting to start stream",
			zap.String("stream_id", streamID))

		if s.startStreamHandler != nil {
			if err := s.startStreamHandler(streamID); err != nil {
				s.logger.Error("Failed to start stream for HLS",
					zap.String("stream_id", streamID),
					zap.Error(err))
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"error": fmt.Sprintf("Failed to start stream %s: %v", streamID, err),
				})
				return
			}

			// Muxer 생성 대기 (retry 로직: 최대 5초, 0.5초 간격)
			maxRetries := 10
			retryInterval := 500 * time.Millisecond

			for i := 0; i < maxRetries; i++ {
				time.Sleep(retryInterval)

				muxer, exists = s.hlsManager.GetMuxer(streamID)
				if exists {
					s.logger.Info("HLS Muxer ready after retry",
						zap.String("stream_id", streamID),
						zap.Int("retry_count", i+1),
						zap.Duration("total_wait", time.Duration(i+1)*retryInterval))
					break
				}

				s.logger.Debug("Waiting for HLS Muxer to be ready",
					zap.String("stream_id", streamID),
					zap.Int("retry", i+1),
					zap.Int("max_retries", maxRetries))
			}

			// 최종 확인
			if !exists {
				s.logger.Error("HLS Muxer not ready after max retries",
					zap.String("stream_id", streamID),
					zap.Duration("total_wait", time.Duration(maxRetries)*retryInterval))
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"error": fmt.Sprintf("Stream %s started but HLS Muxer not ready after %.1f seconds. RTSP connection may be slow.",
						streamID, float64(maxRetries)*retryInterval.Seconds()),
				})
				return
			}
		} else {
			c.JSON(http.StatusNotFound, gin.H{
				"error": fmt.Sprintf("Stream %s not found", streamID),
			})
			return
		}
	}

	// gohlslib muxer의 Handle 메서드로 요청 전달
	muxer.Handle(c.Writer, c.Request)
}

// handleHLSSegment는 TS 세그먼트를 서빙합니다
func (s *Server) handleHLSSegment(c *gin.Context) {
	streamID := c.Param("streamId")

	if s.hlsManager == nil || !s.hlsManager.IsEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "HLS is not enabled",
		})
		return
	}

	muxer, exists := s.hlsManager.GetMuxer(streamID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("Stream %s not found", streamID),
		})
		return
	}

	// gohlslib muxer의 Handle 메서드로 요청 전달
	muxer.Handle(c.Writer, c.Request)
}
