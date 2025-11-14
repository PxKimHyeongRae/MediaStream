package cctv

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/yourusername/cctv3/internal/client"
	"github.com/yourusername/cctv3/internal/core"
	"go.uber.org/zap"
)

// CCTVManager manages CCTV streams from external API
type CCTVManager struct {
	apiClient     *client.APIClient
	streamManager *core.StreamManager
	logger        *zap.Logger

	// Configuration
	apiURL   string
	username string
	password string

	// State
	cctvs map[string]CCTVStream
	mutex sync.RWMutex

	// Context for lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
}

// CCTVStream represents a CCTV stream configuration
type CCTVStream struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	URL            string `json:"url"`
	SourceOnDemand bool   `json:"sourceOnDemand"`
	RTSPTransport  string `json:"rtspTransport"`
}

// Config holds the configuration for CCTVManager
type Config struct {
	APIURL        string
	Username      string
	Password      string
	StreamManager *core.StreamManager
	Logger        *zap.Logger
}

// NewCCTVManager creates a new CCTV manager
func NewCCTVManager(config Config) *CCTVManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &CCTVManager{
		apiClient:     client.NewAPIClient(config.APIURL),
		streamManager: config.StreamManager,
		logger:        config.Logger,
		apiURL:        config.APIURL,
		username:      config.Username,
		password:      config.Password,
		cctvs:         make(map[string]CCTVStream),
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Start initializes the CCTV manager by authenticating and syncing data
func (m *CCTVManager) Start() error {
	m.logger.Info("Starting CCTV manager",
		zap.String("api_url", m.apiURL),
		zap.String("username", m.username),
	)

	// Step 1: Authenticate
	if err := m.authenticate(); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	m.logger.Info("Authentication successful")

	// Step 2: Sync CCTVs (disabled due to server-side issues)
	// if err := m.syncCCTVs(); err != nil {
	//	m.logger.Warn("CCTV sync failed, continuing with fetch", zap.Error(err))
	// } else {
	//	m.logger.Info("CCTV sync completed")
	// }
	m.logger.Info("CCTV sync skipped (server-side issues)")

	// Step 3: Fetch CCTV list
	if err := m.fetchCCTVs(); err != nil {
		return fmt.Errorf("failed to fetch CCTV list: %w", err)
	}

	m.logger.Info("CCTV manager started successfully",
		zap.Int("total_cctvs", len(m.cctvs)),
	)

	// Start periodic sync
	go m.periodicSync()

	return nil
}

// Stop stops the CCTV manager
func (m *CCTVManager) Stop() {
	m.logger.Info("Stopping CCTV manager")
	if m.cancel != nil {
		m.cancel()
	}
}

// authenticate performs authentication with the external API
func (m *CCTVManager) authenticate() error {
	ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
	defer cancel()

	return m.apiClient.SignIn(ctx, m.username, m.password)
}

// syncCCTVs triggers the CCTV synchronization process
func (m *CCTVManager) syncCCTVs() error {
	ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second) // Give more time for sync
	defer cancel()

	return m.apiClient.SyncCCTVs(ctx)
}

// fetchCCTVs retrieves and updates the CCTV list
func (m *CCTVManager) fetchCCTVs() error {
	ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
	defer cancel()

	cctvs, err := m.apiClient.GetCCTVs(ctx)
	if err != nil {
		return err
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Clear existing CCTVs
	m.cctvs = make(map[string]CCTVStream)

	// Convert API response to CCTVStream
	for _, cctv := range cctvs {
		streamID := fmt.Sprintf("%s", cctv.Name) // Generate stream ID from name

		m.cctvs[streamID] = CCTVStream{
			ID:             streamID,
			Name:           cctv.Name,
			URL:            cctv.URL,
			SourceOnDemand: true,  // Default to on-demand for external API sources
			RTSPTransport:  "tcp", // Default to TCP
		}

		m.logger.Info("CCTV added",
			zap.String("stream_id", streamID),
			zap.String("name", cctv.Name),
			zap.String("url", m.maskURL(cctv.URL)),
		)
	}

	return nil
}

// periodicSync runs periodic synchronization
func (m *CCTVManager) periodicSync() {
	ticker := time.NewTicker(5 * time.Minute) // Sync every 5 minutes
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.logger.Info("Starting periodic CCTV sync")

			// Re-authenticate if needed
			if err := m.authenticate(); err != nil {
				m.logger.Error("Periodic authentication failed", zap.Error(err))
				continue
			}

			// Sync CCTVs
			if err := m.syncCCTVs(); err != nil {
				m.logger.Error("Periodic CCTV sync failed", zap.Error(err))
				continue
			}

			// Fetch updated list
			if err := m.fetchCCTVs(); err != nil {
				m.logger.Error("Failed to fetch updated CCTV list", zap.Error(err))
				continue
			}

			m.logger.Info("Periodic sync completed",
				zap.Int("total_cctvs", len(m.cctvs)),
			)
		}
	}
}

// GetCCTVs returns the current list of CCTV streams
func (m *CCTVManager) GetCCTVs() map[string]CCTVStream {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Return a copy to prevent external modification
	result := make(map[string]CCTVStream)
	for k, v := range m.cctvs {
		result[k] = v
	}
	return result
}

// GetCCTV returns a specific CCTV by stream ID
func (m *CCTVManager) GetCCTV(streamID string) (CCTVStream, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	cctv, exists := m.cctvs[streamID]
	return cctv, exists
}

// GetStreamConfig returns the stream configuration for the core system
func (m *CCTVManager) GetStreamConfig(streamID string) (*core.PathConfig, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	cctv, exists := m.cctvs[streamID]
	if !exists {
		return nil, fmt.Errorf("CCTV stream %s not found", streamID)
	}

	return &core.PathConfig{
		Source:         cctv.URL,
		SourceOnDemand: cctv.SourceOnDemand,
		RTSPTransport:  cctv.RTSPTransport,
	}, nil
}

// maskURL masks sensitive information in URLs for logging
func (m *CCTVManager) maskURL(url string) string {
	// Simple masking - in production, you might want more sophisticated masking
	if len(url) > 20 {
		return url[:10] + "***" + url[len(url)-10:]
	}
	return "***"
}
