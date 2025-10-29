package core

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config는 전체 애플리케이션 설정을 담는 구조체
type Config struct {
	Server      ServerConfig      `yaml:"server"`
	RTSP        RTSPConfig        `yaml:"rtsp"`
	WebRTC      WebRTCConfig      `yaml:"webrtc"`
	Media       MediaConfig       `yaml:"media"`
	Logging     LoggingConfig     `yaml:"logging"`
	Metrics     MetricsConfig     `yaml:"metrics"`
	Performance PerformanceConfig `yaml:"performance"`
}

type ServerConfig struct {
	HTTPPort   int  `yaml:"http_port"`
	WSPort     int  `yaml:"ws_port"`
	Production bool `yaml:"production"`
}

type RTSPConfig struct {
	TestStream TestStreamConfig `yaml:"test_stream"`
	Client     RTSPClientConfig `yaml:"client"`
	Pool       RTSPPoolConfig   `yaml:"pool"`
}

type TestStreamConfig struct {
	URL  string `yaml:"url"`
	Name string `yaml:"name"`
}

type RTSPClientConfig struct {
	Timeout       int  `yaml:"timeout"`
	RetryCount    int  `yaml:"retry_count"`
	RetryDelay    int  `yaml:"retry_delay"`
	TCPTransport  bool `yaml:"tcp_transport"`
}

type RTSPPoolConfig struct {
	MaxStreams          int `yaml:"max_streams"`
	HealthCheckInterval int `yaml:"health_check_interval"`
}

type WebRTCConfig struct {
	ICEServers []ICEServerConfig `yaml:"ice_servers"`
	Settings   WebRTCSettings    `yaml:"settings"`
}

type ICEServerConfig struct {
	URLs []string `yaml:"urls"`
}

type WebRTCSettings struct {
	MaxPeers    int      `yaml:"max_peers"`
	VideoCodecs []string `yaml:"video_codecs"`
	AudioCodecs []string `yaml:"audio_codecs"`
}

type MediaConfig struct {
	Buffer BufferConfig `yaml:"buffer"`
	Codec  CodecConfig  `yaml:"codec"`
}

type BufferConfig struct {
	VideoBufferSize int `yaml:"video_buffer_size"`
	AudioBufferSize int `yaml:"audio_buffer_size"`
}

type CodecConfig struct {
	H264Profile string `yaml:"h264_profile"`
	MaxBitrate  int    `yaml:"max_bitrate"`
}

type LoggingConfig struct {
	Level      string `yaml:"level"`
	Output     string `yaml:"output"`
	FilePath   string `yaml:"file_path"`
	MaxSize    int    `yaml:"max_size"`
	MaxBackups int    `yaml:"max_backups"`
	MaxAge     int    `yaml:"max_age"`
}

type MetricsConfig struct {
	Enabled  bool `yaml:"enabled"`
	Port     int  `yaml:"port"`
	Interval int  `yaml:"interval"`
}

type PerformanceConfig struct {
	WorkerPoolSize  int `yaml:"worker_pool_size"`
	ReadBufferSize  int `yaml:"read_buffer_size"`
	WriteBufferSize int `yaml:"write_buffer_size"`
	GCPercent       int `yaml:"gc_percent"`
}

// LoadConfig는 YAML 파일에서 설정을 로드합니다
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// 설정 검증
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &config, nil
}

// Validate는 설정값의 유효성을 검증합니다
func (c *Config) Validate() error {
	if c.Server.HTTPPort <= 0 || c.Server.HTTPPort > 65535 {
		return fmt.Errorf("invalid http_port: %d", c.Server.HTTPPort)
	}

	if c.Server.WSPort <= 0 || c.Server.WSPort > 65535 {
		return fmt.Errorf("invalid ws_port: %d", c.Server.WSPort)
	}

	if c.RTSP.Pool.MaxStreams <= 0 {
		return fmt.Errorf("max_streams must be positive")
	}

	if c.WebRTC.Settings.MaxPeers <= 0 {
		return fmt.Errorf("max_peers must be positive")
	}

	return nil
}
