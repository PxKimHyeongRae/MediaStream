package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"
)

// StressTestConfig 스트레스 테스트 설정
type StressTestConfig struct {
	NumStreams        int           // 스트림 수
	ClientsPerStream  int           // 스트림당 클라이언트 수
	RampUpDuration    time.Duration // 점진적 증가 시간
	TestDuration      time.Duration // 테스트 실행 시간
	MinPackets        int           // 최소 패킷 수
}

// StressTestResult 스트레스 테스트 결과
type StressTestResult struct {
	TotalClients      int
	SuccessfulClients int
	FailedClients     int
	TotalPackets      uint64
	Duration          time.Duration
	ConnectionTime    time.Duration
	AvgPacketsPerClient float64
	Throughput        float64 // packets/second
}

// TestStressLoad는 대규모 동시 접속 스트레스 테스트입니다
func TestStressLoad(t *testing.T) {
	config := StressTestConfig{
		NumStreams:       3,
		ClientsPerStream: 1000, // 3 * 1000 = 3,000 clients
		RampUpDuration:   60 * time.Second,  // 1분: 3K 클라이언트 점진적 연결
		TestDuration:     60 * time.Second,  // 1분: 테스트 시간
		MinPackets:       30,                // 빠른 성공 판정
	}

	totalClients := config.NumStreams * config.ClientsPerStream
	t.Logf("🚀 스트레스 테스트 시작")
	t.Logf("📊 설정:")
	t.Logf("  - 스트림 수: %d", config.NumStreams)
	t.Logf("  - 스트림당 클라이언트: %d", config.ClientsPerStream)
	t.Logf("  - 총 클라이언트: %d", totalClients)
	t.Logf("  - 램프업 시간: %v", config.RampUpDuration)
	t.Logf("  - 테스트 시간: %v", config.TestDuration)

	result := runStressTest(t, config)

	t.Logf("\n📈 테스트 결과:")
	t.Logf("  - 총 클라이언트: %d", result.TotalClients)
	t.Logf("  - 성공: %d (%.2f%%)", result.SuccessfulClients,
		float64(result.SuccessfulClients)/float64(result.TotalClients)*100)
	t.Logf("  - 실패: %d (%.2f%%)", result.FailedClients,
		float64(result.FailedClients)/float64(result.TotalClients)*100)
	t.Logf("  - 총 패킷 수: %d", result.TotalPackets)
	t.Logf("  - 평균 패킷/클라이언트: %.2f", result.AvgPacketsPerClient)
	t.Logf("  - 처리량: %.2f packets/sec", result.Throughput)
	t.Logf("  - 연결 시간: %v", result.ConnectionTime)
	t.Logf("  - 총 테스트 시간: %v", result.Duration)

	// 성공률 검증 (최소 90% 성공)
	successRate := float64(result.SuccessfulClients) / float64(result.TotalClients)
	require.Greater(t, successRate, 0.9, "Success rate should be > 90%%")
}

// runStressTest 스트레스 테스트 실행
func runStressTest(t *testing.T, config StressTestConfig) StressTestResult {
	startTime := time.Now()

	var (
		successCount atomic.Int32
		failCount    atomic.Int32
		totalPackets atomic.Uint64
		wg           sync.WaitGroup
	)

	totalClients := config.NumStreams * config.ClientsPerStream
	ctx, cancel := context.WithTimeout(context.Background(),
		config.RampUpDuration + config.TestDuration + 120*time.Second) // 2분 추가 마진
	defer cancel()

	// 램프업: 점진적으로 클라이언트 추가
	clientDelay := config.RampUpDuration / time.Duration(totalClients)
	t.Logf("⏱️  클라이언트당 지연: %v", clientDelay)

	connectionStart := time.Now()
	clientID := 0

	for streamIdx := 0; streamIdx < config.NumStreams; streamIdx++ {
		streamID := fmt.Sprintf("stream%d", streamIdx+1)

		for clientNum := 0; clientNum < config.ClientsPerStream; clientNum++ {
			wg.Add(1)
			clientID++
			id := clientID

			go func(streamID string, id int) {
				defer wg.Done()

				client := &TestClient{
					t:             t,
					serverURL:     "ws://localhost:8080/ws",
					streamID:      streamID,
					videoReceived: false,
					packetCount:   0,
				}

				// 짧은 테스트: 연결 → 패킷 수신 확인 → 종료
				testCtx, testCancel := context.WithTimeout(ctx, config.TestDuration)
				defer testCancel()

				err := client.TestVideoStreamQuick(testCtx, config.MinPackets)
				if err != nil {
					failCount.Add(1)
					if id <= 10 { // 처음 10개만 로그
						t.Logf("❌ Client %d (%s) failed: %v", id, streamID, err)
					}
				} else {
					successCount.Add(1)
					totalPackets.Add(uint64(client.packetCount))
					if id <= 10 { // 처음 10개만 로그
						t.Logf("✅ Client %d (%s) success: %d packets", id, streamID, client.packetCount)
					}
				}
			}(streamID, id)

			// 램프업 지연
			time.Sleep(clientDelay)
		}
	}

	connectionTime := time.Since(connectionStart)
	t.Logf("🔗 모든 클라이언트 연결 시작 완료: %v", connectionTime)

	// 진행 상황 모니터링
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			goto finished
		case <-ticker.C:
			t.Logf("📊 진행: 성공=%d, 실패=%d, 패킷=%d",
				successCount.Load(), failCount.Load(), totalPackets.Load())
		case <-ctx.Done():
			t.Log("⚠️ Timeout reached")
			goto finished
		}
	}

finished:
	duration := time.Since(startTime)
	successful := int(successCount.Load())
	failed := int(failCount.Load())
	packets := totalPackets.Load()

	avgPackets := 0.0
	if successful > 0 {
		avgPackets = float64(packets) / float64(successful)
	}

	throughput := 0.0
	if duration.Seconds() > 0 {
		throughput = float64(packets) / duration.Seconds()
	}

	return StressTestResult{
		TotalClients:      totalClients,
		SuccessfulClients: successful,
		FailedClients:     failed,
		TotalPackets:      packets,
		Duration:          duration,
		ConnectionTime:    connectionTime,
		AvgPacketsPerClient: avgPackets,
		Throughput:        throughput,
	}
}

// TestVideoStreamQuick는 빠른 비디오 스트림 테스트 (패킷 수 확인)
func (c *TestClient) TestVideoStreamQuick(ctx context.Context, minPackets int) error {
	// WebSocket 연결
	var err error
	c.ws, _, err = websocket.DefaultDialer.Dial(c.serverURL, nil)
	if err != nil {
		return fmt.Errorf("websocket dial failed: %w", err)
	}
	defer c.ws.Close()

	// PeerConnection 생성
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	}

	c.pc, err = webrtc.NewPeerConnection(config)
	if err != nil {
		return fmt.Errorf("peer connection creation failed: %w", err)
	}
	defer c.pc.Close()

	// 트랜시버 추가
	if _, err = c.pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo,
		webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly}); err != nil {
		return fmt.Errorf("video transceiver failed: %w", err)
	}

	if _, err = c.pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio,
		webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly}); err != nil {
		return fmt.Errorf("audio transceiver failed: %w", err)
	}

	// 이벤트 핸들러
	c.setupPeerConnectionHandlers()

	// Offer 생성 및 전송
	offer, err := c.pc.CreateOffer(nil)
	if err != nil {
		return fmt.Errorf("offer creation failed: %w", err)
	}

	if err = c.pc.SetLocalDescription(offer); err != nil {
		return fmt.Errorf("set local description failed: %w", err)
	}

	// Offer 전송
	streamID := c.streamID
	if streamID == "" {
		streamID = "stream1"
	}

	offerPayload := OfferPayload{SDP: offer.SDP, StreamID: streamID}
	offerPayloadJSON, _ := json.Marshal(offerPayload)
	offerMsg := SignalingMessage{Type: "offer", Payload: offerPayloadJSON}

	if err = c.ws.WriteJSON(offerMsg); err != nil {
		return fmt.Errorf("send offer failed: %w", err)
	}

	// Answer 수신
	done := make(chan error, 1)
	go func() {
		for {
			var msg SignalingMessage
			if err := c.ws.ReadJSON(&msg); err != nil {
				done <- err
				return
			}

			switch msg.Type {
			case "answer":
				var answerSDP string
				json.Unmarshal(msg.Payload, &answerSDP)
				answer := webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: answerSDP}
				c.pc.SetRemoteDescription(answer)
			case "error":
				var errMsg string
				json.Unmarshal(msg.Payload, &errMsg)
				done <- fmt.Errorf("server error: %s", errMsg)
				return
			}
		}
	}()

	// 패킷 수신 대기
	timeout := time.After(90 * time.Second) // 3K 클라이언트를 위한 충분한 시간
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			c.mu.Lock()
			count := c.packetCount
			c.mu.Unlock()
			if count < minPackets {
				return fmt.Errorf("insufficient packets: %d < %d", count, minPackets)
			}
			return nil
		case err := <-done:
			return err
		case <-ticker.C:
			c.mu.Lock()
			count := c.packetCount
			c.mu.Unlock()
			if count >= minPackets {
				return nil
			}
		}
	}
}
