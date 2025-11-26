package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVideoStreaming은 실제 비디오 스트리밍을 테스트합니다
func TestVideoStreaming(t *testing.T) {
	// 테스트 타임아웃 설정
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 테스트 클라이언트 생성
	client := &TestClient{
		t:             t,
		serverURL:     "ws://localhost:8080/ws",
		videoReceived: false,
		packetCount:   0,
	}

	// 비디오 스트림 테스트 실행
	err := client.TestVideoStream(ctx)
	require.NoError(t, err, "Video streaming test failed")

	// 검증
	assert.True(t, client.videoReceived, "No video track received")
	assert.Greater(t, client.packetCount, 100, "Insufficient video packets received (expected > 100)")

	t.Logf("✅ Test passed! Received %d video packets", client.packetCount)
}

// TestClient는 테스트용 WebRTC 클라이언트입니다
type TestClient struct {
	t             *testing.T
	serverURL     string
	streamID      string // 선택할 스트림 ID
	ws            *websocket.Conn
	pc            *webrtc.PeerConnection
	videoReceived bool
	packetCount   int
	mu            sync.Mutex
}

// SignalingMessage는 시그널링 메시지 구조체입니다
type SignalingMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// OfferPayload는 Offer 페이로드를 나타냅니다
type OfferPayload struct {
	SDP      string `json:"sdp"`
	StreamID string `json:"streamId"`
}

// TestVideoStream은 비디오 스트림 테스트를 실행합니다
func (c *TestClient) TestVideoStream(ctx context.Context) error {
	c.t.Log("🔌 Connecting to WebSocket server...")

	// WebSocket 연결
	u, err := url.Parse(c.serverURL)
	if err != nil {
		return fmt.Errorf("failed to parse URL: %w", err)
	}

	c.ws, _, err = websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to connect to WebSocket: %w", err)
	}
	defer c.ws.Close()

	c.t.Log("✅ WebSocket connected")

	// PeerConnection 생성
	c.t.Log("🎬 Creating PeerConnection...")
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{
					"stun:stun.l.google.com:19302",
				},
			},
		},
	}

	c.pc, err = webrtc.NewPeerConnection(config)
	if err != nil {
		return fmt.Errorf("failed to create peer connection: %w", err)
	}
	defer c.pc.Close()

	// 비디오/오디오 트랜시버 추가
	if _, err = c.pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	}); err != nil {
		return fmt.Errorf("failed to add video transceiver: %w", err)
	}

	if _, err = c.pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	}); err != nil {
		return fmt.Errorf("failed to add audio transceiver: %w", err)
	}

	// 이벤트 핸들러 설정
	c.setupPeerConnectionHandlers()

	// Offer 생성
	c.t.Log("📤 Creating and sending offer...")
	offer, err := c.pc.CreateOffer(nil)
	if err != nil {
		return fmt.Errorf("failed to create offer: %w", err)
	}

	if err = c.pc.SetLocalDescription(offer); err != nil {
		return fmt.Errorf("failed to set local description: %w", err)
	}

	// Offer 전송 (streamID 포함)
	// streamID가 없으면 기본값 사용
	streamID := c.streamID
	if streamID == "" {
		streamID = "stream1" // 기본값
	}

	offerPayload := OfferPayload{
		SDP:      offer.SDP,
		StreamID: streamID,
	}

	offerPayloadJSON, err := json.Marshal(offerPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal offer payload: %w", err)
	}

	offerMsg := SignalingMessage{
		Type:    "offer",
		Payload: offerPayloadJSON,
	}
	if err = c.ws.WriteJSON(offerMsg); err != nil {
		return fmt.Errorf("failed to send offer: %w", err)
	}

	c.t.Logf("✅ Offer sent for stream: %s", streamID)

	// Answer 수신 대기
	c.t.Log("⏳ Waiting for answer...")
	done := make(chan error, 1)

	go func() {
		for {
			var msg SignalingMessage
			if err := c.ws.ReadJSON(&msg); err != nil {
				done <- fmt.Errorf("failed to read message: %w", err)
				return
			}

			switch msg.Type {
			case "answer":
				var answerSDP string
				if err := json.Unmarshal(msg.Payload, &answerSDP); err != nil {
					done <- fmt.Errorf("failed to unmarshal answer: %w", err)
					return
				}

				c.t.Log("📥 Answer received")
				answer := webrtc.SessionDescription{
					Type: webrtc.SDPTypeAnswer,
					SDP:  answerSDP,
				}

				if err := c.pc.SetRemoteDescription(answer); err != nil {
					done <- fmt.Errorf("failed to set remote description: %w", err)
					return
				}

				c.t.Log("✅ Remote description set")

			case "error":
				var errMsg string
				if err := json.Unmarshal(msg.Payload, &errMsg); err != nil {
					done <- fmt.Errorf("server error (failed to parse): %v", msg.Payload)
					return
				}
				done <- fmt.Errorf("server error: %s", errMsg)
				return
			}
		}
	}()

	// ICE 연결 및 비디오 수신 대기
	c.t.Log("⏳ Waiting for ICE connection and video packets...")

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	// 최대 대기 시간
	timeout := time.After(25 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled")

		case <-timeout:
			c.mu.Lock()
			received := c.videoReceived
			count := c.packetCount
			c.mu.Unlock()

			if !received {
				return fmt.Errorf("timeout: no video track received after 25s")
			}
			if count < 100 {
				return fmt.Errorf("timeout: insufficient packets received (%d < 100)", count)
			}
			return nil

		case err := <-done:
			return err

		case <-ticker.C:
			// 주기적으로 상태 체크
			c.mu.Lock()
			iceState := c.pc.ICEConnectionState()
			received := c.videoReceived
			count := c.packetCount
			c.mu.Unlock()

			c.t.Logf("📊 Status: ICE=%s, VideoTrack=%v, Packets=%d",
				iceState, received, count)

			// 성공 조건: 비디오 트랙 수신 + 100개 이상의 패킷
			if received && count >= 100 {
				c.t.Log("🎉 Success criteria met!")
				return nil
			}
		}
	}
}

// setupPeerConnectionHandlers는 PeerConnection 이벤트 핸들러를 설정합니다
func (c *TestClient) setupPeerConnectionHandlers() {
	// ICE 연결 상태 변경
	c.pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		c.t.Logf("🧊 ICE connection state: %s", state)
	})

	// 연결 상태 변경
	c.pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		c.t.Logf("🔗 Connection state: %s", state)
	})

	// 트랙 수신
	c.pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		c.t.Logf("🎥 Track received: %s (codec: %s)", track.Kind(), track.Codec().MimeType)

		if track.Kind() == webrtc.RTPCodecTypeVideo {
			c.mu.Lock()
			c.videoReceived = true
			c.mu.Unlock()

			// 비디오 패킷 읽기
			go func() {
				for {
					_, _, err := track.ReadRTP()
					if err != nil {
						c.t.Logf("⚠️ RTP read error: %v", err)
						return
					}

					c.mu.Lock()
					c.packetCount++
					count := c.packetCount
					c.mu.Unlock()

					// 100개 단위로 로그
					if count%100 == 0 {
						c.t.Logf("📦 Received %d video packets", count)
					}
				}
			}()
		}
	})
}

// TestMultipleClients는 여러 클라이언트 동시 접속을 테스트합니다
func TestMultipleClients(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	numClients := 3
	t.Logf("🚀 Testing %d simultaneous clients...", numClients)

	var wg sync.WaitGroup
	errors := make(chan error, numClients)

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		clientID := i + 1

		go func(id int) {
			defer wg.Done()

			client := &TestClient{
				t:             t,
				serverURL:     "ws://localhost:8080/ws",
				videoReceived: false,
				packetCount:   0,
			}

			t.Logf("👤 Client %d: Starting test...", id)

			if err := client.TestVideoStream(ctx); err != nil {
				errors <- fmt.Errorf("client %d failed: %w", id, err)
				return
			}

			t.Logf("✅ Client %d: Success! (%d packets)", id, client.packetCount)
		}(clientID)
	}

	// 모든 클라이언트 완료 대기
	wg.Wait()
	close(errors)

	// 에러 확인
	var failedClients []error
	for err := range errors {
		failedClients = append(failedClients, err)
	}

	require.Empty(t, failedClients, "Some clients failed: %v", failedClients)
	t.Logf("🎉 All %d clients succeeded!", numClients)
}

// TestMultiStreamMultiClient는 여러 스트림을 여러 클라이언트가 동시에 시청하는 시나리오를 테스트합니다
func TestMultiStreamMultiClient(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 테스트 시나리오: 3개 스트림, 각 스트림당 2명의 클라이언트 (총 6명)
	streams := []string{"stream1", "stream2", "stream3"}
	clientsPerStream := 2

	totalClients := len(streams) * clientsPerStream
	t.Logf("🚀 Testing %d streams with %d clients per stream (total: %d clients)...",
		len(streams), clientsPerStream, totalClients)

	var wg sync.WaitGroup
	errors := make(chan error, totalClients)
	results := make(chan struct {
		streamID    string
		clientNum   int
		packetCount int
	}, totalClients)

	clientID := 0
	for _, streamID := range streams {
		for clientNum := 1; clientNum <= clientsPerStream; clientNum++ {
			wg.Add(1)
			clientID++

			// 클로저 내부에서 사용할 변수들을 복사
			stream := streamID
			num := clientNum
			id := clientID

			go func(streamID string, clientNum int, globalID int) {
				defer wg.Done()

				client := &TestClient{
					t:             t,
					serverURL:     "ws://localhost:8080/ws",
					streamID:      streamID,
					videoReceived: false,
					packetCount:   0,
				}

				t.Logf("👤 Client %d (Stream %s, Client #%d): Starting test...",
					globalID, streamID, clientNum)

				if err := client.TestVideoStream(ctx); err != nil {
					errors <- fmt.Errorf("client %d (stream %s, client #%d) failed: %w",
						globalID, streamID, clientNum, err)
					return
				}

				t.Logf("✅ Client %d (Stream %s, Client #%d): Success! (%d packets)",
					globalID, streamID, clientNum, client.packetCount)

				results <- struct {
					streamID    string
					clientNum   int
					packetCount int
				}{streamID, clientNum, client.packetCount}
			}(stream, num, id)
		}
	}

	// 모든 클라이언트 완료 대기
	wg.Wait()
	close(errors)
	close(results)

	// 에러 확인
	var failedClients []error
	for err := range errors {
		failedClients = append(failedClients, err)
	}

	require.Empty(t, failedClients, "Some clients failed: %v", failedClients)

	// 결과 집계
	streamStats := make(map[string]int)
	for result := range results {
		streamStats[result.streamID]++
		t.Logf("📊 Stream %s: Client received %d packets", result.streamID, result.packetCount)
	}

	// 각 스트림별로 클라이언트 수 검증
	for _, streamID := range streams {
		count := streamStats[streamID]
		assert.Equal(t, clientsPerStream, count,
			"Stream %s should have %d successful clients, got %d",
			streamID, clientsPerStream, count)
	}

	t.Logf("🎉 All %d clients across %d streams succeeded!", totalClients, len(streams))
	t.Log("📊 Final stats:")
	for streamID, count := range streamStats {
		t.Logf("  - %s: %d clients", streamID, count)
	}
}
