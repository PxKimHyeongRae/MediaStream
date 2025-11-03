package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/cctv3/internal/core"
	"github.com/yourusername/cctv3/internal/process"
	"go.uber.org/zap"
)

// PathsHandler는 /api/v1/paths 엔드포인트를 처리합니다
type PathsHandler struct {
	configStore    *core.ConfigStore
	streamManager  interface{} // StreamManager 인터페이스 (추후 통합)
	processManager *process.Manager
	logger         *zap.Logger
}

// NewPathsHandler는 새로운 PathsHandler를 생성합니다
func NewPathsHandler(
	configStore *core.ConfigStore,
	streamManager interface{},
	processManager *process.Manager,
	logger *zap.Logger,
) *PathsHandler {
	return &PathsHandler{
		configStore:    configStore,
		streamManager:  streamManager,
		processManager: processManager,
		logger:         logger,
	}
}

// PathsResponse는 경로 목록 응답
type PathsResponse struct {
	Paths map[string]core.PathConfig `json:"paths"`
	Count int                         `json:"count"`
}

// PathResponse는 단일 경로 응답
type PathResponse struct {
	ID      string             `json:"id"`
	Config  core.PathConfig    `json:"config"`
	IsYAML  bool               `json:"isYaml"`
	Running bool               `json:"running,omitempty"`
}

// ErrorResponse는 에러 응답
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// GetAllPaths는 모든 경로를 가져옵니다
// GET /api/v1/paths
func (h *PathsHandler) GetAllPaths(c *gin.Context) {
	paths := h.configStore.GetAllPaths()

	c.JSON(http.StatusOK, PathsResponse{
		Paths: paths,
		Count: len(paths),
	})
}

// GetPath는 특정 경로를 가져옵니다
// GET /api/v1/paths/:id
func (h *PathsHandler) GetPath(c *gin.Context) {
	id := c.Param("id")

	config, exists := h.configStore.GetPath(id)
	if !exists {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "Path not found",
		})
		return
	}

	isYAML := h.configStore.IsYAMLPath(id)
	running := false
	if config.RunOnDemand != "" {
		running = h.processManager.IsRunning(id)
	}

	c.JSON(http.StatusOK, PathResponse{
		ID:      id,
		Config:  config,
		IsYAML:  isYAML,
		Running: running,
	})
}

// AddPaths는 하나 이상의 경로를 추가합니다
// POST /api/v1/paths
// Body: { "path_id": { config }, "path_id2": { config2 }, ... }
func (h *PathsHandler) AddPaths(c *gin.Context) {
	var paths map[string]core.PathConfig
	if err := c.ShouldBindJSON(&paths); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_json",
			Message: err.Error(),
		})
		return
	}

	if len(paths) == 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "empty_request",
			Message: "At least one path must be provided",
		})
		return
	}

	added := []string{}
	errors := make(map[string]string)

	for id, config := range paths {
		if err := h.configStore.AddPath(id, config); err != nil {
			errors[id] = err.Error()
			continue
		}
		added = append(added, id)

		// runOnDemand는 요청 시 시작하므로 여기서는 스트림만 생성하지 않음
		h.logger.Info("Path added via API", zap.String("id", id))
	}

	response := gin.H{
		"added": added,
		"count": len(added),
	}

	if len(errors) > 0 {
		response["errors"] = errors
	}

	statusCode := http.StatusCreated
	if len(added) == 0 {
		statusCode = http.StatusBadRequest
	} else if len(errors) > 0 {
		statusCode = http.StatusMultiStatus
	}

	c.JSON(statusCode, response)
}

// UpdatePath는 경로를 수정합니다
// PUT /api/v1/paths/:id
func (h *PathsHandler) UpdatePath(c *gin.Context) {
	id := c.Param("id")

	var config core.PathConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_json",
			Message: err.Error(),
		})
		return
	}

	if err := h.configStore.UpdatePath(id, config); err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "path "+id+" not found" {
			statusCode = http.StatusNotFound
		} else if err.Error() == "cannot update YAML-defined path "+id+", use API-only paths" {
			statusCode = http.StatusForbidden
		}

		c.JSON(statusCode, ErrorResponse{
			Error:   "update_failed",
			Message: err.Error(),
		})
		return
	}

	h.logger.Info("Path updated via API", zap.String("id", id))

	c.JSON(http.StatusOK, gin.H{
		"message": "Path updated successfully",
		"id":      id,
	})
}

// DeletePath는 경로를 삭제합니다
// DELETE /api/v1/paths/:id
func (h *PathsHandler) DeletePath(c *gin.Context) {
	id := c.Param("id")

	// 실행 중인 프로세스가 있으면 먼저 중지
	if h.processManager.IsRunning(id) {
		if err := h.processManager.Stop(id); err != nil {
			h.logger.Error("Failed to stop process before deletion",
				zap.String("id", id),
				zap.Error(err),
			)
		}
	}

	if err := h.configStore.DeletePath(id); err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "path "+id+" not found" {
			statusCode = http.StatusNotFound
		} else if err.Error() == "cannot delete YAML-defined path "+id {
			statusCode = http.StatusForbidden
		}

		c.JSON(statusCode, ErrorResponse{
			Error:   "delete_failed",
			Message: err.Error(),
		})
		return
	}

	h.logger.Info("Path deleted via API", zap.String("id", id))

	c.JSON(http.StatusOK, gin.H{
		"message": "Path deleted successfully",
		"id":      id,
	})
}

// GetConfigSchema는 설정 가능한 옵션 스키마를 반환합니다
// GET /api/v1/config/schema
func (h *PathsHandler) GetConfigSchema(c *gin.Context) {
	schema := map[string]interface{}{
		"type":        "object",
		"description": "Path configuration schema",
		"properties": map[string]interface{}{
			"source": map[string]interface{}{
				"type":        "string",
				"description": "RTSP source URL (mutually exclusive with runOnDemand)",
				"example":     "rtsp://admin:password@192.168.1.100:554/stream",
			},
			"sourceOnDemand": map[string]interface{}{
				"type":        "boolean",
				"description": "Start RTSP source only when clients request",
				"default":     true,
			},
			"rtspTransport": map[string]interface{}{
				"type":        "string",
				"description": "RTSP transport protocol",
				"enum":        []string{"tcp", "udp"},
				"default":     "tcp",
			},
			"runOnDemand": map[string]interface{}{
				"type":        "string",
				"description": "External command to run on demand (mutually exclusive with source). Typically used for ffmpeg transcoding.",
				"example":     "ffmpeg -i rtsp://source -c:v libx264 -f rtsp rtsp://127.0.0.1:8554/output",
			},
			"runOnDemandRestart": map[string]interface{}{
				"type":        "boolean",
				"description": "Automatically restart the process if it crashes",
				"default":     false,
			},
			"runOnDemandCloseAfter": map[string]interface{}{
				"type":        "string",
				"description": "Duration to wait before closing the process after last client disconnects. Examples: '10s', '1m', '5m'",
				"example":     "15s",
				"default":     "10s",
			},
		},
		"oneOf": []interface{}{
			map[string]interface{}{
				"required": []string{"source"},
			},
			map[string]interface{}{
				"required": []string{"runOnDemand"},
			},
		},
		"examples": []interface{}{
			map[string]interface{}{
				"description": "RTSP source example",
				"config": map[string]interface{}{
					"source":         "rtsp://admin:pass@192.168.1.100:554/stream",
					"sourceOnDemand": true,
					"rtspTransport":  "tcp",
				},
			},
			map[string]interface{}{
				"description": "ffmpeg transcoding example (H.265 to H.264)",
				"config": map[string]interface{}{
					"runOnDemand":           "ffmpeg -rtsp_transport tcp -i rtsp://127.0.0.1:8554/source -c:v libx264 -preset veryfast -f rtsp rtsp://127.0.0.1:8554/output",
					"runOnDemandRestart":    true,
					"runOnDemandCloseAfter": "15s",
				},
			},
		},
	}

	c.JSON(http.StatusOK, schema)
}

// parseDuration은 문자열을 time.Duration으로 파싱합니다
func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 10 * time.Second, nil // 기본값
	}
	return time.ParseDuration(s)
}
