package cctv

import "github.com/yourusername/cctv3/internal/core"

// Provider는 CCTV 정보를 제공하는 인터페이스입니다
type Provider interface {
	// GetCCTVs returns all available CCTV streams
	GetCCTVs() map[string]CCTVStream

	// GetCCTV returns a specific CCTV stream by ID
	GetCCTV(streamID string) (CCTVStream, bool)

	// GetStreamConfig returns the stream configuration for the core system
	GetStreamConfig(streamID string) (*core.PathConfig, error)

	// Start initializes the CCTV provider
	Start() error

	// Stop stops the CCTV provider
	Stop()

	// ManualSync performs manual synchronization with external API
	ManualSync() error
}
