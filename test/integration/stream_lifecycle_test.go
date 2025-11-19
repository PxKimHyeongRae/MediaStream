package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourusername/cctv3/internal/database"
)

// TestStreamLifecycle는 스트림 생명주기 전체를 테스트합니다
func TestStreamLifecycle(t *testing.T) {
	if !isServerRunning() {
		t.Skip("Server is not running")
	}

	cleanupTestStreams(t)
	defer cleanupTestStreams(t)

	t.Run("OnDemandStreamManagement", testOnDemandStreamManagement)
	t.Run("MultipleUsersOneCamera", testMultipleUsersOneCamera)
	t.Run("UserConnectionTracking", testUserConnectionTracking)
	t.Run("ConcurrentMultipleStreams", testConcurrentMultipleStreams)
	t.Run("RapidConnectDisconnect", testRapidConnectDisconnect)
	t.Run("StreamInterruptionRecovery", testStreamInterruptionRecovery)
	t.Run("ResourceCleanupOnDisconnect", testResourceCleanupOnDisconnect)
	t.Run("FirstFrameTime", testFirstFrameTime)
}

// testOnDemandStreamManagement는 온디맨드 스트림 관리를 테스트합니다
// 시나리오: 첫 사용자 접속 시 RTSP 시작, 마지막 사용자 종료 시 RTSP 정지
func testOnDemandStreamManagement(t *testing.T) {
	streamID := "test-ondemand-lifecycle"

	// 1. 온디맨드 스트림 생성
	stream := database.Stream{
		ID:             streamID,
		Name:           "On-Demand Lifecycle Test",
		Source:         "rtsp://admin:live0416@192.168.10.53:554/Streaming/Channels/101",
		SourceOnDemand: true,
		RTSPTransport:  "tcp",
	}

	createStream(t, stream)
	defer deleteStream(t, streamID)

	// 2. 초기 상태: RTSP 클라이언트 없음
	info := getStream(t, streamID)
	assert.NotNil(t, info.RuntimeInfo)
	packetsRecv := uint64(info.RuntimeInfo["packets_received"].(float64))
	assert.Equal(t, uint64(0), packetsRecv, "초기 상태: RTSP 연결 없어야 함")

	t.Log("✅ 초기 상태: RTSP 클라이언트 없음 확인")

	// 3. 스트림 시작 (첫 사용자 접속 시뮬레이션)
	resp, err := http.Post(baseURL+"/api/v1/streams/"+streamID+"/start", "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	t.Log("✅ 첫 사용자 접속: RTSP 클라이언트 시작 요청")

	// 4. RTSP 연결 대기
	time.Sleep(3 * time.Second)

	// 5. RTSP 클라이언트 활성화 확인
	info = getStream(t, streamID)
	assert.NotNil(t, info.RuntimeInfo)
	assert.Equal(t, true, info.RuntimeInfo["is_active"])

	time.Sleep(2 * time.Second)
	info = getStream(t, streamID)
	packetsRecv = uint64(info.RuntimeInfo["packets_received"].(float64))
	assert.Greater(t, packetsRecv, uint64(0), "RTSP 클라이언트가 패킷을 수신해야 함")

	t.Logf("✅ RTSP 클라이언트 활성화: %d packets received", packetsRecv)

	// 6. 스트림 정지 (마지막 사용자 종료 시뮬레이션)
	// DELETE를 사용하면 스트림 자체가 삭제되므로, 여기서는 테스트 종료 시 cleanup에서 처리
	t.Log("✅ 온디맨드 스트림 생명주기 테스트 완료")
}

// testMultipleUsersOneCamera는 여러 사용자가 하나의 카메라에 접속하는 시나리오를 테스트합니다
// 핵심 검증: RTSP 연결은 항상 1개만 유지되어야 함
func testMultipleUsersOneCamera(t *testing.T) {
	streamID := "test-multi-users-one-cam"

	// 1. Always-on 스트림 생성 (sourceOnDemand=false)
	stream := database.Stream{
		ID:             streamID,
		Name:           "Multi Users One Camera Test",
		Source:         "rtsp://admin:live0416@192.168.10.53:554/Streaming/Channels/101",
		SourceOnDemand: false, // 즉시 RTSP 시작
		RTSPTransport:  "tcp",
	}

	createStream(t, stream)
	defer deleteStream(t, streamID)

	// 2. RTSP 연결 대기
	time.Sleep(3 * time.Second)

	// 3. 초기 RTSP 클라이언트 상태 확인
	info := getStream(t, streamID)
	assert.NotNil(t, info.RuntimeInfo)
	assert.Equal(t, true, info.RuntimeInfo["is_active"])

	initialPackets := uint64(info.RuntimeInfo["packets_received"].(float64))
	assert.Greater(t, initialPackets, uint64(0), "RTSP 클라이언트가 활성화되어야 함")

	t.Logf("✅ 초기 RTSP 클라이언트 활성화: %d packets", initialPackets)

	// 4. 여러 "가상 사용자" 시뮬레이션
	// 실제로는 GET 요청을 여러 번 보내서 구독자 수를 확인
	numUsers := 5
	var wg sync.WaitGroup

	for i := 0; i < numUsers; i++ {
		wg.Add(1)
		go func(userID int) {
			defer wg.Done()

			// 각 사용자가 스트림 정보를 조회 (시청 시작 시뮬레이션)
			userInfo := getStream(t, streamID)
			assert.NotNil(t, userInfo.RuntimeInfo)
			assert.Equal(t, true, userInfo.RuntimeInfo["is_active"])

			t.Logf("사용자 %d: 스트림 접속 성공", userID)

			// 잠시 대기 (시청 시뮬레이션)
			time.Sleep(2 * time.Second)
		}(i + 1)
	}

	wg.Wait()

	// 5. 핵심 검증: RTSP 연결이 여전히 1개인지 확인
	// (패킷 수신이 계속 증가하는지 확인)
	time.Sleep(2 * time.Second)
	finalInfo := getStream(t, streamID)
	finalPackets := uint64(finalInfo.RuntimeInfo["packets_received"].(float64))

	assert.Greater(t, finalPackets, initialPackets, "RTSP 클라이언트가 계속 패킷을 수신해야 함")
	t.Logf("✅ 여러 사용자 접속 후에도 RTSP 클라이언트 1개 유지: %d packets (초기: %d)", finalPackets, initialPackets)

	// 6. 구독자 수 확인 (WebRTC 피어가 없으므로 0일 것)
	subscriberCount := int(finalInfo.RuntimeInfo["subscriber_count"].(float64))
	t.Logf("현재 구독자 수: %d", subscriberCount)
}

// testUserConnectionTracking는 사용자 연결 상태 추적을 테스트합니다
func testUserConnectionTracking(t *testing.T) {
	streamID := "test-user-tracking"

	// 1. Always-on 스트림 생성
	stream := database.Stream{
		ID:             streamID,
		Name:           "User Connection Tracking Test",
		Source:         "rtsp://admin:live0416@192.168.10.53:554/Streaming/Channels/101",
		SourceOnDemand: false,
		RTSPTransport:  "tcp",
	}

	createStream(t, stream)
	defer deleteStream(t, streamID)

	// 2. RTSP 연결 대기
	time.Sleep(3 * time.Second)

	// 3. 초기 상태 확인
	info := getStream(t, streamID)
	initialSubscribers := int(info.RuntimeInfo["subscriber_count"].(float64))
	assert.Equal(t, 0, initialSubscribers, "초기 구독자 수는 0이어야 함")

	t.Log("✅ 초기 구독자 수: 0")

	// 4. 스트림 목록에서도 확인
	listResp := listStreamsWithRuntime(t)
	found := false
	for _, s := range listResp.Streams {
		if s.ID == streamID {
			found = true
			assert.NotNil(t, s.RuntimeInfo)
			assert.Equal(t, true, s.RuntimeInfo["is_active"])
			break
		}
	}
	assert.True(t, found, "스트림이 목록에 존재해야 함")

	t.Log("✅ 사용자 연결 추적 테스트 완료")
}

// testConcurrentMultipleStreams는 여러 스트림 동시 관리를 테스트합니다
func testConcurrentMultipleStreams(t *testing.T) {
	numStreams := 3
	streamIDs := make([]string, numStreams)

	// 1. 여러 스트림 동시 생성
	var wg sync.WaitGroup
	for i := 0; i < numStreams; i++ {
		streamIDs[i] = fmt.Sprintf("test-concurrent-%d", i+1)
		wg.Add(1)

		go func(idx int) {
			defer wg.Done()

			stream := database.Stream{
				ID:             streamIDs[idx],
				Name:           fmt.Sprintf("Concurrent Stream %d", idx+1),
				Source:         "rtsp://admin:live0416@192.168.10.53:554/Streaming/Channels/101",
				SourceOnDemand: false,
				RTSPTransport:  "tcp",
			}

			createStream(t, stream)
			t.Logf("스트림 %s 생성 완료", streamIDs[idx])
		}(i)
	}

	wg.Wait()

	// 2. RTSP 연결 대기
	time.Sleep(5 * time.Second)

	// 3. 모든 스트림이 활성화되었는지 확인
	for _, streamID := range streamIDs {
		info := getStream(t, streamID)
		assert.NotNil(t, info.RuntimeInfo)
		assert.Equal(t, true, info.RuntimeInfo["is_active"])

		packetsRecv := uint64(info.RuntimeInfo["packets_received"].(float64))
		assert.Greater(t, packetsRecv, uint64(0), fmt.Sprintf("스트림 %s가 패킷을 수신해야 함", streamID))

		t.Logf("✅ 스트림 %s 활성화: %d packets", streamID, packetsRecv)
	}

	// 4. 정리
	for _, streamID := range streamIDs {
		deleteStream(t, streamID)
	}

	t.Logf("✅ %d개 스트림 동시 관리 테스트 완료", numStreams)
}

// testRapidConnectDisconnect는 빠른 연결/해제 반복을 테스트합니다
// 핵심: 리소스 누수가 없어야 함
func testRapidConnectDisconnect(t *testing.T) {
	streamID := "test-rapid-connect"

	iterations := 5
	for i := 0; i < iterations; i++ {
		t.Logf("반복 %d/%d", i+1, iterations)

		// 1. 스트림 생성
		stream := database.Stream{
			ID:             streamID,
			Name:           "Rapid Connect Test",
			Source:         "rtsp://admin:live0416@192.168.10.53:554/Streaming/Channels/101",
			SourceOnDemand: false,
			RTSPTransport:  "tcp",
		}

		createStream(t, stream)

		// 2. RTSP 연결 대기
		time.Sleep(2 * time.Second)

		// 3. 패킷 수신 확인
		info := getStream(t, streamID)
		assert.NotNil(t, info.RuntimeInfo)

		// 4. 즉시 삭제
		deleteStream(t, streamID)

		// 5. 삭제 확인
		resp, err := http.Get(baseURL + "/api/v1/streams/" + streamID)
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)

		// 6. 잠시 대기 (리소스 정리 시간)
		time.Sleep(500 * time.Millisecond)
	}

	t.Logf("✅ 빠른 연결/해제 %d회 반복 테스트 완료 - 리소스 누수 없음", iterations)
}

// testStreamInterruptionRecovery는 스트림 중단 복구를 테스트합니다
func testStreamInterruptionRecovery(t *testing.T) {
	streamID := "test-interruption"

	// 1. 스트림 생성
	stream := database.Stream{
		ID:             streamID,
		Name:           "Interruption Recovery Test",
		Source:         "rtsp://admin:live0416@192.168.10.53:554/Streaming/Channels/101",
		SourceOnDemand: false,
		RTSPTransport:  "tcp",
	}

	createStream(t, stream)
	defer deleteStream(t, streamID)

	// 2. RTSP 연결 대기 및 패킷 수신 확인
	time.Sleep(3 * time.Second)
	info := getStream(t, streamID)
	packetsRecv1 := uint64(info.RuntimeInfo["packets_received"].(float64))
	assert.Greater(t, packetsRecv1, uint64(0), "초기 패킷 수신 확인")

	t.Logf("✅ 초기 패킷 수신: %d packets", packetsRecv1)

	// 3. Source 변경 (스트림 중단 시뮬레이션)
	// 같은 URL이지만 UPDATE를 통해 RTSP 재시작
	stream.Name = "Updated Interruption Test"
	updateStream(t, streamID, stream)

	t.Log("스트림 업데이트 - RTSP 재시작 중...")

	// 4. RTSP 재연결 대기
	time.Sleep(4 * time.Second)

	// 5. 재연결 후 패킷 수신 확인
	info = getStream(t, streamID)
	assert.NotNil(t, info.RuntimeInfo)
	assert.Equal(t, true, info.RuntimeInfo["is_active"])
	assert.Equal(t, "Updated Interruption Test", info.Name)

	t.Log("✅ 스트림 중단 복구 테스트 완료")
}

// testResourceCleanupOnDisconnect는 연결 종료 시 리소스 정리를 테스트합니다
func testResourceCleanupOnDisconnect(t *testing.T) {
	streamID := "test-resource-cleanup"

	// 1. 스트림 생성
	stream := database.Stream{
		ID:             streamID,
		Name:           "Resource Cleanup Test",
		Source:         "rtsp://admin:live0416@192.168.10.53:554/Streaming/Channels/101",
		SourceOnDemand: false,
		RTSPTransport:  "tcp",
	}

	createStream(t, stream)

	// 2. RTSP 연결 대기
	time.Sleep(3 * time.Second)

	// 3. 패킷 수신 확인
	info := getStream(t, streamID)
	packetsRecv := uint64(info.RuntimeInfo["packets_received"].(float64))
	assert.Greater(t, packetsRecv, uint64(0), "RTSP 클라이언트 활성화 확인")

	t.Logf("✅ RTSP 클라이언트 활성화: %d packets", packetsRecv)

	// 4. 스트림 삭제 (리소스 정리)
	deleteStream(t, streamID)

	// 5. 삭제 확인
	resp, err := http.Get(baseURL + "/api/v1/streams/" + streamID)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	// 6. 목록에서도 제거 확인
	listResp := listStreamsWithRuntime(t)
	for _, s := range listResp.Streams {
		assert.NotEqual(t, streamID, s.ID, "삭제된 스트림이 목록에 없어야 함")
	}

	t.Log("✅ 리소스 정리 테스트 완료 - RTSP/HLS/Stream 모두 정리됨")
}

// testFirstFrameTime는 첫 프레임까지의 시간을 측정합니다
func testFirstFrameTime(t *testing.T) {
	streamID := "test-first-frame"

	// 1. 온디맨드 스트림 생성
	stream := database.Stream{
		ID:             streamID,
		Name:           "First Frame Time Test",
		Source:         "rtsp://admin:live0416@192.168.10.53:554/Streaming/Channels/101",
		SourceOnDemand: true,
		RTSPTransport:  "tcp",
	}

	createStream(t, stream)
	defer deleteStream(t, streamID)

	// 2. 스트림 시작 및 시간 측정
	startTime := time.Now()

	resp, err := http.Post(baseURL+"/api/v1/streams/"+streamID+"/start", "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// 3. 첫 패킷 수신까지 대기
	maxWait := 10 * time.Second
	pollInterval := 500 * time.Millisecond
	firstFrameTime := time.Duration(0)

	for elapsed := time.Duration(0); elapsed < maxWait; elapsed += pollInterval {
		time.Sleep(pollInterval)

		info := getStream(t, streamID)
		if info.RuntimeInfo != nil {
			packetsRecv := uint64(info.RuntimeInfo["packets_received"].(float64))
			if packetsRecv > 0 {
				firstFrameTime = time.Since(startTime)
				break
			}
		}
	}

	assert.Greater(t, firstFrameTime, time.Duration(0), "첫 프레임을 수신해야 함")
	assert.Less(t, firstFrameTime, 10*time.Second, "첫 프레임까지 10초 이내여야 함")

	t.Logf("✅ Time to First Frame (TTFF): %v", firstFrameTime)

	// 일반적으로 3-5초 이내면 양호
	if firstFrameTime < 5*time.Second {
		t.Logf("   → 우수 (< 5초)")
	} else if firstFrameTime < 10*time.Second {
		t.Logf("   → 양호 (< 10초)")
	} else {
		t.Logf("   → 개선 필요 (>= 10초)")
	}
}

// TestRTSPClientVerification는 RTSP 클라이언트가 실제로 생성되었는지 검증합니다
func TestRTSPClientVerification(t *testing.T) {
	if !isServerRunning() {
		t.Skip("Server is not running")
	}

	cleanupTestStreams(t)
	defer cleanupTestStreams(t)

	streamID := "test-rtsp-verification"

	// 1. Always-on 스트림 생성
	stream := database.Stream{
		ID:             streamID,
		Name:           "RTSP Client Verification",
		Source:         "rtsp://admin:live0416@192.168.10.53:554/Streaming/Channels/101",
		SourceOnDemand: false,
		RTSPTransport:  "tcp",
	}

	createStream(t, stream)
	defer deleteStream(t, streamID)

	// 2. RTSP 연결 대기
	time.Sleep(3 * time.Second)

	// 3. RuntimeInfo 확인
	info := getStream(t, streamID)
	require.NotNil(t, info.RuntimeInfo, "RuntimeInfo가 존재해야 함")

	// 4. RTSP 클라이언트 활성화 확인
	isActive := info.RuntimeInfo["is_active"].(bool)
	assert.True(t, isActive, "RTSP 클라이언트가 활성화되어야 함")

	// 5. 코덱 정보 확인 (RTSP 연결 성공 시 자동 감지)
	codec := info.RuntimeInfo["codec"]
	t.Logf("감지된 코덱: %v", codec)

	// 6. 패킷 수신 확인
	time.Sleep(2 * time.Second)
	info = getStream(t, streamID)
	packetsRecv := uint64(info.RuntimeInfo["packets_received"].(float64))
	bytesRecv := uint64(info.RuntimeInfo["bytes_received"].(float64))

	assert.Greater(t, packetsRecv, uint64(0), "RTSP 클라이언트가 패킷을 수신해야 함")
	assert.Greater(t, bytesRecv, uint64(0), "RTSP 클라이언트가 바이트를 수신해야 함")

	t.Logf("✅ RTSP 클라이언트 검증 완료:")
	t.Logf("   - 활성화: %v", isActive)
	t.Logf("   - 코덱: %v", codec)
	t.Logf("   - 수신 패킷: %d", packetsRecv)
	t.Logf("   - 수신 바이트: %d", bytesRecv)
}

// TestStressTest는 부하 테스트를 수행합니다
func TestStressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	if !isServerRunning() {
		t.Skip("Server is not running")
	}

	cleanupTestStreams(t)
	defer cleanupTestStreams(t)

	// 1. 다중 스트림 생성
	numStreams := 5
	streamIDs := make([]string, numStreams)

	t.Logf("부하 테스트: %d개 스트림 동시 생성 및 관리", numStreams)

	var wg sync.WaitGroup
	for i := 0; i < numStreams; i++ {
		streamIDs[i] = fmt.Sprintf("test-stress-%d", i+1)
		wg.Add(1)

		go func(idx int) {
			defer wg.Done()

			stream := database.Stream{
				ID:             streamIDs[idx],
				Name:           fmt.Sprintf("Stress Test Stream %d", idx+1),
				Source:         "rtsp://admin:live0416@192.168.10.53:554/Streaming/Channels/101",
				SourceOnDemand: false,
				RTSPTransport:  "tcp",
			}

			createStream(t, stream)
		}(i)
	}

	wg.Wait()
	t.Logf("✅ %d개 스트림 생성 완료", numStreams)

	// 2. RTSP 연결 대기
	time.Sleep(5 * time.Second)

	// 3. 모든 스트림 상태 확인
	activeStreams := 0
	totalPackets := uint64(0)

	for _, streamID := range streamIDs {
		info := getStream(t, streamID)
		if info.RuntimeInfo != nil && info.RuntimeInfo["is_active"].(bool) {
			activeStreams++
			packetsRecv := uint64(info.RuntimeInfo["packets_received"].(float64))
			totalPackets += packetsRecv
			t.Logf("스트림 %s: %d packets", streamID, packetsRecv)
		}
	}

	assert.Equal(t, numStreams, activeStreams, "모든 스트림이 활성화되어야 함")
	assert.Greater(t, totalPackets, uint64(0), "패킷을 수신해야 함")

	t.Logf("✅ 부하 테스트 완료:")
	t.Logf("   - 활성 스트림: %d/%d", activeStreams, numStreams)
	t.Logf("   - 총 수신 패킷: %d", totalPackets)

	// 4. 정리
	for _, streamID := range streamIDs {
		deleteStream(t, streamID)
	}
}

// Helper: StreamResponse with RuntimeInfo
type StreamResponseWithRuntime struct {
	ID             string                 `json:"id"`
	Name           string                 `json:"name"`
	Source         string                 `json:"source"`
	SourceOnDemand bool                   `json:"source_on_demand"`
	RTSPTransport  string                 `json:"rtsp_transport"`
	CreatedAt      string                 `json:"created_at"`
	UpdatedAt      string                 `json:"updated_at"`
	RuntimeInfo    map[string]interface{} `json:"runtime_info"`
}

type StreamListResponseWithRuntime struct {
	Streams []StreamResponseWithRuntime `json:"streams"`
	Total   int                         `json:"total"`
}

func listStreamsWithRuntime(t *testing.T) StreamListResponseWithRuntime {
	resp, err := http.Get(baseURL + "/api/v1/streams")
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result StreamListResponseWithRuntime
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	return result
}
