package rtsp

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/pion/rtp"
	"go.uber.org/zap"
)

// Client는 RTSP 스트림 클라이언트
type Client struct {
	url            string
	username       string
	password       string
	transport      string // "tcp" or "udp"
	timeout        time.Duration
	retryCount     int
	retryDelay     time.Duration

	// 상태
	ctx           context.Context
	ctxCancel     context.CancelFunc
	logger        *zap.Logger
	connected     bool
	reconnecting  bool
	mutex         sync.RWMutex

	// gortsplib
	client *gortsplib.Client

	// 콜백
	onPacket      func(packet *rtp.Packet)
	onConnect     func()
	onDisconnect  func(err error)

	// 통계
	packetsReceived uint64
	bytesReceived   uint64
}

// ClientConfig는 RTSP 클라이언트 설정
type ClientConfig struct {
	URL          string
	Username     string
	Password     string
	Transport    string        // "tcp" or "udp"
	Timeout      time.Duration
	RetryCount   int
	RetryDelay   time.Duration
	Logger       *zap.Logger
	OnPacket     func(*rtp.Packet)
	OnConnect    func()
	OnDisconnect func(error)
}

// NewClient는 새로운 RTSP 클라이언트를 생성합니다
func NewClient(config ClientConfig) (*Client, error) {
	// URL 검증
	if _, err := url.Parse(config.URL); err != nil {
		return nil, fmt.Errorf("invalid RTSP URL: %w", err)
	}

	// 기본값 설정
	if config.Transport == "" {
		config.Transport = "tcp"
	}
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}
	if config.RetryCount == 0 {
		config.RetryCount = 3
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = 5 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	client := &Client{
		url:          config.URL,
		username:     config.Username,
		password:     config.Password,
		transport:    config.Transport,
		timeout:      config.Timeout,
		retryCount:   config.RetryCount,
		retryDelay:   config.RetryDelay,
		ctx:          ctx,
		ctxCancel:    cancel,
		logger:       config.Logger,
		onPacket:     config.OnPacket,
		onConnect:    config.OnConnect,
		onDisconnect: config.OnDisconnect,
	}

	return client, nil
}

// Start는 RTSP 스트림 수신을 시작합니다
func (c *Client) Start() error {
	c.logger.Info("Starting RTSP client",
		zap.String("url", c.maskURL()),
		zap.String("transport", c.transport),
	)

	go c.runWithRetry()

	return nil
}

// Stop은 RTSP 클라이언트를 종료합니다
func (c *Client) Stop() {
	c.logger.Info("Stopping RTSP client")
	c.ctxCancel()

	if c.client != nil {
		c.client.Close()
	}
}

// IsConnected는 연결 상태를 반환합니다
func (c *Client) IsConnected() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.connected
}

// GetStats는 통계 정보를 반환합니다
func (c *Client) GetStats() (packetsReceived, bytesReceived uint64) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.packetsReceived, c.bytesReceived
}

// runWithRetry는 재연결 로직과 함께 실행합니다
func (c *Client) runWithRetry() {
	attempt := 0

	for {
		select {
		case <-c.ctx.Done():
			c.logger.Info("RTSP client stopped")
			return
		default:
		}

		attempt++
		c.logger.Info("Connecting to RTSP stream",
			zap.Int("attempt", attempt),
			zap.Int("max_attempts", c.retryCount),
		)

		err := c.run()

		// 정상 종료
		if err == nil || c.ctx.Err() != nil {
			return
		}

		// 에러 발생
		c.logger.Error("RTSP connection failed",
			zap.Error(err),
			zap.Int("attempt", attempt),
		)

		c.setConnected(false)

		if c.onDisconnect != nil {
			c.onDisconnect(err)
		}

		// 최대 재시도 횟수 체크 (0이면 무한 재시도)
		if c.retryCount > 0 && attempt >= c.retryCount {
			c.logger.Error("Max retry attempts reached, giving up",
				zap.Int("attempts", attempt),
			)
			return
		}

		// 재연결 대기
		c.logger.Info("Retrying connection",
			zap.Duration("delay", c.retryDelay),
		)

		select {
		case <-time.After(c.retryDelay):
			continue
		case <-c.ctx.Done():
			return
		}
	}
}

// run은 실제 RTSP 연결 및 스트림 수신을 처리합니다
func (c *Client) run() error {
	// RTSP URL 파싱
	u, err := url.Parse(c.url)
	if err != nil {
		return fmt.Errorf("failed to parse URL: %w", err)
	}

	// gortsplib 클라이언트 생성
	c.client = &gortsplib.Client{
		Transport:    c.getTransport(),
		ReadTimeout:  c.timeout,
		WriteTimeout: c.timeout,
	}

	// RTSP 서버 연결
	err = c.client.Start(u.Scheme, u.Host)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer c.client.Close()

	c.logger.Info("Connected to RTSP server")

	// base URL 생성
	baseURL, err := base.ParseURL(c.url)
	if err != nil {
		return fmt.Errorf("failed to parse base URL: %w", err)
	}

	// DESCRIBE: 스트림 정보 획득
	desc, _, err := c.client.Describe(baseURL)
	if err != nil {
		return fmt.Errorf("failed to describe: %w", err)
	}

	c.logger.Info("Stream description received",
		zap.Int("media_count", len(desc.Medias)),
	)

	// 미디어 정보 로깅
	for i, media := range desc.Medias {
		for j, forma := range media.Formats {
			c.logger.Info("Media format detected",
				zap.Int("media_index", i),
				zap.Int("format_index", j),
				zap.String("codec", forma.Codec()),
				zap.Uint8("payload_type", forma.PayloadType()),
			)
		}
	}

	// SETUP: 모든 미디어 트랙 설정
	err = c.client.SetupAll(baseURL, desc.Medias)
	if err != nil {
		return fmt.Errorf("failed to setup: %w", err)
	}

	c.logger.Info("All media tracks setup completed")

	// RTP 패킷 콜백 등록 (PLAY 호출 전에 등록해야 함)
	c.client.OnPacketRTPAny(func(medi *description.Media, forma format.Format, pkt *rtp.Packet) {
		// RTP 패킷 수신 시 호출됨
		c.handleRTPPacket(pkt)
	})

	c.logger.Info("RTP packet callback registered")

	// PLAY: 재생 시작
	_, err = c.client.Play(nil)
	if err != nil {
		return fmt.Errorf("failed to play: %w", err)
	}

	c.logger.Info("RTSP playback started")

	c.setConnected(true)

	if c.onConnect != nil {
		c.onConnect()
	}

	// 연결 유지 및 에러 대기
	// OnPacketRTPAny 콜백이 자동으로 RTP 패킷을 수신하므로
	// 별도의 readPackets 고루틴은 불필요
	return c.client.Wait()
}

// getTransport는 전송 프로토콜을 반환합니다
func (c *Client) getTransport() *gortsplib.Transport {
	if c.transport == "udp" {
		transport := gortsplib.TransportUDP
		return &transport
	}
	transport := gortsplib.TransportTCP
	return &transport
}

// setConnected는 연결 상태를 설정합니다
func (c *Client) setConnected(connected bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.connected = connected
}

// maskURL은 비밀번호를 마스킹한 URL을 반환합니다
func (c *Client) maskURL() string {
	u, err := url.Parse(c.url)
	if err != nil {
		return "***"
	}

	if u.User != nil {
		u.User = url.UserPassword("***", "***")
	}

	return u.String()
}

// handleRTPPacket은 RTP 패킷을 처리합니다
func (c *Client) handleRTPPacket(pkt *rtp.Packet) {
	c.mutex.Lock()
	c.packetsReceived++
	c.bytesReceived += uint64(len(pkt.Payload))
	c.mutex.Unlock()

	if c.onPacket != nil {
		c.onPacket(pkt)
	}
}
