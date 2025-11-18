package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	// "github.com/yourusername/cctv3/internal/cctv" // AIOT API 관련 - 향후 재사용을 위해 주석 처리
	"github.com/yourusername/cctv3/internal/core"
	"github.com/yourusername/cctv3/internal/database"
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
	stopStreamHandler  func(streamID string) error // 스트림 정지 콜백

	// Database repository for CRUD operations
	streamRepo *database.StreamRepository

	// Stream manager for runtime streams
	streamManager *core.StreamManager

	// AIOT API 관련 (향후 재사용을 위해 주석 처리)
	// cctvManager cctv.Provider

	hlsManager *hls.Manager
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
	StopStreamHandler  func(streamID string) error // 스트림 정지 콜백

	// Database repository for CRUD operations
	StreamRepository *database.StreamRepository

	// Stream manager for runtime streams
	StreamManager *core.StreamManager

	// AIOT API 관련 (향후 재사용을 위해 주석 처리)
	// CCTVManager cctv.Provider

	HLSManager *hls.Manager
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
		stopStreamHandler:  config.StopStreamHandler,
		streamRepo:         config.StreamRepository,
		streamManager:      config.StreamManager,
		// cctvManager:        config.CCTVManager, // AIOT API 관련 - 향후 재사용을 위해 주석 처리
		hlsManager: config.HLSManager,
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

		// AIOT API 관련 - 향후 재사용을 위해 주석 처리
		// v1.POST("/sync", s.handleSync)

		// Stream CRUD endpoints
		streams := v1.Group("/streams")
		{
			streams.POST("", s.handleCreateStream)          // Create stream
			streams.GET("", s.handleListStreams)            // List all streams
			streams.GET("/:id", s.handleGetStream)          // Get single stream
			streams.PUT("/:id", s.handleUpdateStream)       // Update stream
			streams.DELETE("/:id", s.handleDeleteStream)    // Delete stream (stop RTSP client)
			streams.POST("/:id/start", s.handleStartStream) // Start on-demand stream
		}

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

// AIOT API 관련 핸들러 - 향후 재사용을 위해 주석 처리
// handleSync는 CCTV 목록 수동 동기화를 처리합니다
// func (s *Server) handleSync(c *gin.Context) {
// 	if s.cctvManager == nil {
// 		c.JSON(http.StatusServiceUnavailable, gin.H{
// 			"error": "CCTV manager not available",
// 		})
// 		return
// 	}
//
// 	s.logger.Info("Manual sync requested")
//
// 	if err := s.cctvManager.ManualSync(); err != nil {
// 		s.logger.Error("Manual sync failed", zap.Error(err))
// 		c.JSON(http.StatusInternalServerError, gin.H{
// 			"error": "Sync failed: " + err.Error(),
// 		})
// 		return
// 	}
//
// 	// 동기화 완료 후 업데이트된 CCTV 목록 반환
// 	cctvs := s.cctvManager.GetCCTVs()
//
// 	c.JSON(http.StatusOK, gin.H{
// 		"status":  "success",
// 		"message": "CCTV list synchronized successfully",
// 		"count":   len(cctvs),
// 	})
// }

// handlePathsList는 mediaMTX 스타일의 paths 목록을 반환합니다
func (s *Server) handlePathsList(c *gin.Context) {
	// Database에서 모든 스트림 가져오기 (Single Source of Truth)
	dbStreams, err := s.streamRepo.List()
	if err != nil {
		s.logger.Error("Failed to list streams from database", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to list streams: " + err.Error(),
		})
		return
	}

	// mediaMTX 호환 형식으로 변환
	items := make([]gin.H, 0, len(dbStreams))
	for _, stream := range dbStreams {
		items = append(items, gin.H{
			"name":   stream.ID, // mediaMTX는 name 필드에 ID 사용
			"source": stream.Source,
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

	// 요청 바디 파싱
	var request struct {
		Source         string `json:"source" binding:"required"`
		SourceOnDemand bool   `json:"sourceOnDemand"`
		RTSPTransport  string `json:"rtspTransport"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request body: " + err.Error(),
		})
		return
	}

	// 기본값 설정
	if request.RTSPTransport == "" {
		request.RTSPTransport = "tcp"
	}

	s.logger.Info("Path add requested",
		zap.String("name", pathName),
		zap.String("source", request.Source),
		zap.Bool("sourceOnDemand", request.SourceOnDemand),
	)

	// 데이터베이스에 저장
	stream := &database.Stream{
		ID:             pathName,
		Name:           pathName,
		Source:         request.Source,
		SourceOnDemand: request.SourceOnDemand,
		RTSPTransport:  request.RTSPTransport,
	}

	if err := s.streamRepo.Create(stream); err != nil {
		s.logger.Error("Failed to create stream", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to create stream: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Path added successfully",
		"name":    pathName,
	})
}

// handlePathDelete는 path를 삭제합니다
func (s *Server) handlePathDelete(c *gin.Context) {
	pathName := c.Param("name")

	s.logger.Info("Path delete requested", zap.String("name", pathName))

	if err := s.streamRepo.Delete(pathName); err != nil {
		s.logger.Error("Failed to delete stream", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to delete stream: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Path deleted successfully",
		"name":    pathName,
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

// ====================================================================================
// CRUD API Handlers
// ====================================================================================

// handleCreateStream은 새로운 스트림을 생성합니다
func (s *Server) handleCreateStream(c *gin.Context) {
	var request database.Stream

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request body: " + err.Error(),
		})
		return
	}

	// ID가 없으면 Name을 ID로 사용
	if request.ID == "" {
		request.ID = request.Name
	}

	// 기본값 설정
	if request.RTSPTransport == "" {
		request.RTSPTransport = "tcp"
	}

	s.logger.Info("Creating new stream",
		zap.String("id", request.ID),
		zap.String("name", request.Name),
		zap.Bool("source_on_demand", request.SourceOnDemand),
	)

	// 1. Database에 저장 (Single Source of Truth)
	if err := s.streamRepo.Create(&request); err != nil {
		s.logger.Error("Failed to create stream", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to create stream: " + err.Error(),
		})
		return
	}

	// 2. StreamManager에 Stream 객체 생성 (필수!)
	if s.streamManager != nil {
		if _, err := s.streamManager.CreateStream(request.ID, request.Name); err != nil {
			s.logger.Error("Failed to create stream in manager", zap.Error(err))
			// Database는 이미 저장되었으므로 계속 진행
		}
	}

	// 3. sourceOnDemand=false면 즉시 RTSP 클라이언트 시작
	if !request.SourceOnDemand && s.startStreamHandler != nil {
		if err := s.startStreamHandler(request.ID); err != nil {
			s.logger.Error("Failed to start RTSP client for always-on stream",
				zap.String("id", request.ID),
				zap.Error(err),
			)
			// 에러가 나도 CREATE는 성공했으므로 201 반환
		} else {
			s.logger.Info("RTSP client started for always-on stream", zap.String("id", request.ID))
		}
	}

	s.logger.Info("Stream created successfully", zap.String("id", request.ID))

	c.JSON(http.StatusCreated, request)
}

// handleListStreams는 모든 스트림 목록을 반환합니다
func (s *Server) handleListStreams(c *gin.Context) {
	// Database에서 모든 스트림 가져오기 (Single Source of Truth)
	dbStreams, err := s.streamRepo.List()
	if err != nil {
		s.logger.Error("Failed to list streams from database", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to list streams: " + err.Error(),
		})
		return
	}

	// 응답 데이터 구성 (runtime_info 추가)
	streams := make([]gin.H, 0, len(dbStreams))
	for _, dbStream := range dbStreams {
		streamData := gin.H{
			"id":               dbStream.ID,
			"name":             dbStream.Name,
			"source":           dbStream.Source,
			"source_on_demand": dbStream.SourceOnDemand,
			"rtsp_transport":   dbStream.RTSPTransport,
			"created_at":       dbStream.CreatedAt,
			"updated_at":       dbStream.UpdatedAt,
		}

		// StreamManager에서 runtime 정보 가져오기 (있으면)
		if s.streamManager != nil {
			if runtimeStream, err := s.streamManager.GetStream(dbStream.ID); err == nil {
				packetsRecv, packetsSent, bytesRecv, bytesSent := runtimeStream.GetStats()
				streamData["runtime_info"] = gin.H{
					"is_active":        true,
					"codec":            runtimeStream.GetVideoCodec(),
					"subscriber_count": runtimeStream.GetSubscriberCount(),
					"packets_received": packetsRecv,
					"packets_sent":     packetsSent,
					"bytes_received":   bytesRecv,
					"bytes_sent":       bytesSent,
				}
			}
		}

		streams = append(streams, streamData)
	}

	c.JSON(http.StatusOK, gin.H{
		"streams": streams,
		"count":   len(streams),
	})
}

// handleGetStream은 특정 스트림 정보를 반환합니다
func (s *Server) handleGetStream(c *gin.Context) {
	streamID := c.Param("id")

	// Database에서 찾기 (Single Source of Truth)
	dbStream, err := s.streamRepo.Get(streamID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("Stream %s not found", streamID),
		})
		return
	}

	// 응답 데이터 구성
	response := gin.H{
		"id":               dbStream.ID,
		"name":             dbStream.Name,
		"source":           dbStream.Source,
		"source_on_demand": dbStream.SourceOnDemand,
		"rtsp_transport":   dbStream.RTSPTransport,
		"created_at":       dbStream.CreatedAt,
		"updated_at":       dbStream.UpdatedAt,
	}

	// StreamManager에서 runtime 정보 추가 (있으면)
	if s.streamManager != nil {
		if runtimeStream, err := s.streamManager.GetStream(streamID); err == nil {
			packetsRecv, packetsSent, bytesRecv, bytesSent := runtimeStream.GetStats()
			response["runtime_info"] = gin.H{
				"is_active":        true,
				"codec":            runtimeStream.GetVideoCodec(),
				"subscriber_count": runtimeStream.GetSubscriberCount(),
				"packets_received": packetsRecv,
				"packets_sent":     packetsSent,
				"bytes_received":   bytesRecv,
				"bytes_sent":       bytesSent,
			}
		}
	}

	c.JSON(http.StatusOK, response)
}

// handleUpdateStream은 스트림 정보를 업데이트합니다
func (s *Server) handleUpdateStream(c *gin.Context) {
	streamID := c.Param("id")

	var request database.Stream
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request body: " + err.Error(),
		})
		return
	}

	// ID는 URL 파라미터로부터 사용
	request.ID = streamID

	s.logger.Info("Updating stream",
		zap.String("id", streamID),
		zap.String("name", request.Name),
		zap.String("source", request.Source),
	)

	// 1. Database 업데이트
	if err := s.streamRepo.Update(&request); err != nil {
		s.logger.Error("Failed to update stream", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to update stream: " + err.Error(),
		})
		return
	}

	// 2. 실행 중인 RTSP 클라이언트 재시작 (source가 변경되었을 경우)
	// 먼저 정지
	if s.stopStreamHandler != nil {
		if err := s.stopStreamHandler(streamID); err != nil {
			s.logger.Warn("Failed to stop stream for restart (may not be running)",
				zap.String("id", streamID),
				zap.Error(err),
			)
		}
	}

	// sourceOnDemand=false면 즉시 재시작
	if !request.SourceOnDemand && s.startStreamHandler != nil {
		if err := s.startStreamHandler(streamID); err != nil {
			s.logger.Error("Failed to restart stream after update",
				zap.String("id", streamID),
				zap.Error(err),
			)
			// 에러가 나도 UPDATE는 성공했으므로 200 반환
		} else {
			s.logger.Info("Stream restarted successfully after update", zap.String("id", streamID))
		}
	}

	c.JSON(http.StatusOK, request)
}

// handleDeleteStream은 스트림을 삭제합니다
func (s *Server) handleDeleteStream(c *gin.Context) {
	streamID := c.Param("id")

	s.logger.Info("Deleting stream", zap.String("id", streamID))

	// 1. Database에서 삭제
	if err := s.streamRepo.Delete(streamID); err != nil {
		s.logger.Error("Failed to delete stream from database", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to delete stream: " + err.Error(),
		})
		return
	}

	// 2. 실행 중인 RTSP 클라이언트 정지 (stopStreamHandler 사용)
	if s.stopStreamHandler != nil {
		if err := s.stopStreamHandler(streamID); err != nil {
			s.logger.Warn("Failed to stop stream (may not be running)",
				zap.String("id", streamID),
				zap.Error(err),
			)
		} else {
			s.logger.Info("Stream stopped successfully", zap.String("id", streamID))
		}
	}

	// 3. StreamManager에서 제거
	if s.streamManager != nil {
		if err := s.streamManager.RemoveStream(streamID); err != nil {
			s.logger.Warn("Failed to remove stream from manager",
				zap.String("id", streamID),
				zap.Error(err),
			)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Stream deleted successfully",
		"id":      streamID,
	})
}

// handleStartStream은 온디맨드 스트림을 시작합니다
func (s *Server) handleStartStream(c *gin.Context) {
	streamID := c.Param("id")

	s.logger.Info("Starting stream", zap.String("id", streamID))

	if s.startStreamHandler == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Start stream handler not configured",
		})
		return
	}

	if err := s.startStreamHandler(streamID); err != nil {
		s.logger.Error("Failed to start stream", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to start stream: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Stream started successfully",
		"id":      streamID,
	})
}
