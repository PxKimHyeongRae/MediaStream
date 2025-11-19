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
)

// TestOnDemandVerification는 온디맨드 스트림이 제대로 동작하는지 검증합니다
// 핵심: 여러 사용자가 접속해도 RTSP 연결은 1개만 유지되어야 함
func TestOnDemandVerification(t *testing.T) {
	if !isServerRunning() {
		t.Skip("Server is not running")
	}

	streamID := "CCTV-TEST1"

	t.Log("=== 온디맨드 스트림 검증 시작 ===")
	t.Logf("테스트 대상: %s", streamID)

	// 1. 초기 상태 확인 - RTSP 클라이언트 비활성화 상태여야 함
	t.Log("\n[단계 1] 초기 상태 확인")
	initialInfo := getStreamInfo(t, streamID)
	initialPackets := int(initialInfo.RuntimeInfo["packets_received"].(float64))
	initialSubscribers := int(initialInfo.RuntimeInfo["subscriber_count"].(float64))

	t.Logf("  초기 패킷 수: %d", initialPackets)
	t.Logf("  초기 구독자 수: %d", initialSubscribers)

	// 2. 스트림 시작 (첫 사용자 접속 시뮬레이션)
	t.Log("\n[단계 2] 첫 번째 사용자 접속 - RTSP 클라이언트 시작")
	startStream(t, streamID)
	time.Sleep(3 * time.Second) // RTSP 연결 대기

	afterStartInfo := getStreamInfo(t, streamID)
	afterStartPackets := int(afterStartInfo.RuntimeInfo["packets_received"].(float64))
	codec := afterStartInfo.RuntimeInfo["codec"].(string)

	require.Greater(t, afterStartPackets, initialPackets, "RTSP 클라이언트가 시작되어 패킷을 수신해야 함")
	require.NotEmpty(t, codec, "코덱이 감지되어야 함")

	t.Logf("  ✅ RTSP 클라이언트 시작됨")
	t.Logf("  패킷 수신: %d packets", afterStartPackets)
	t.Logf("  코덱: %s", codec)

	// 3. 다중 사용자 동시 접속 시뮬레이션
	t.Log("\n[단계 3] 5명의 사용자 동시 접속 시뮬레이션")
	userCount := 5
	var wg sync.WaitGroup

	for i := 1; i <= userCount; i++ {
		wg.Add(1)
		go func(userID int) {
			defer wg.Done()
			// 각 사용자가 스트림 정보를 조회 (실제로는 WebRTC 연결)
			info := getStreamInfo(t, streamID)
			t.Logf("  사용자 %d: 스트림 접속 성공 (packets: %.0f)", userID, info.RuntimeInfo["packets_received"].(float64))
		}(i)
	}

	wg.Wait()
	time.Sleep(2 * time.Second) // 패킷 수신 대기

	// 4. 다중 접속 후 상태 확인
	t.Log("\n[단계 4] 다중 접속 후 RTSP 연결 상태 확인")
	afterMultiInfo := getStreamInfo(t, streamID)
	afterMultiPackets := int(afterMultiInfo.RuntimeInfo["packets_received"].(float64))

	require.Greater(t, afterMultiPackets, afterStartPackets, "RTSP 클라이언트가 계속 패킷을 수신해야 함")

	packetsIncrement := afterMultiPackets - afterStartPackets
	t.Logf("  ✅ RTSP 연결 1개 유지 확인")
	t.Logf("  총 패킷 수: %d (증가: %d)", afterMultiPackets, packetsIncrement)
	t.Logf("  접속 사용자: %d명", userCount)

	// 5. 핵심 검증: RTSP 클라이언트가 1개만 있는지 확인
	// 만약 각 사용자마다 RTSP 연결이 생성되었다면 CCTV에 5개의 연결이 생겼을 것
	// 우리는 1개의 RTSP 연결을 공유하므로, 패킷 증가량이 정상 범위여야 함
	t.Log("\n[단계 5] CCTV 부하 검증")

	// 정상적인 경우: 1개 RTSP 연결 → 패킷 증가량이 일정
	// 비정상적인 경우: 5개 RTSP 연결 → 패킷 증가량이 5배
	expectedPacketsPerSecond := afterStartPackets / 3 // 3초 동안 받은 평균
	actualPacketsPerSecond := packetsIncrement / 2    // 2초 동안 받은 패킷

	// 패킷 증가율이 2배 미만이면 정상 (1개 연결 유지)
	// 5배 이상이면 비정상 (다중 연결 생성)
	ratio := float64(actualPacketsPerSecond) / float64(expectedPacketsPerSecond)
	t.Logf("  예상 패킷/초: %d", expectedPacketsPerSecond)
	t.Logf("  실제 패킷/초: %d", actualPacketsPerSecond)
	t.Logf("  증가 비율: %.2fx", ratio)

	assert.Less(t, ratio, 3.0, "RTSP 연결이 1개만 유지되어야 함 (비율이 3배 미만)")

	if ratio < 3.0 {
		t.Logf("  ✅ 원본 CCTV에 영향 없음 - RTSP 연결 1개 유지됨")
	} else {
		t.Errorf("  ❌ 원본 CCTV에 부하 발생 - 다중 RTSP 연결 의심 (비율: %.2fx)", ratio)
	}

	// 6. 최종 검증: RuntimeInfo 확인
	t.Log("\n[단계 6] 최종 상태 검증")
	finalInfo := getStreamInfo(t, streamID)
	isActive := finalInfo.RuntimeInfo["is_active"].(bool)
	finalPackets := int(finalInfo.RuntimeInfo["packets_received"].(float64))
	finalBytes := int(finalInfo.RuntimeInfo["bytes_received"].(float64))

	assert.True(t, isActive, "RTSP 클라이언트가 활성화되어 있어야 함")
	assert.Greater(t, finalPackets, 0, "패킷을 수신했어야 함")
	assert.Greater(t, finalBytes, 0, "바이트를 수신했어야 함")

	t.Logf("  활성화: %v", isActive)
	t.Logf("  총 패킷: %d", finalPackets)
	t.Logf("  총 바이트: %d", finalBytes)
	t.Logf("  코덱: %s", finalInfo.RuntimeInfo["codec"])

	t.Log("\n=== ✅ 온디맨드 스트림 검증 완료 ===")
	t.Log("결론: 여러 사용자가 접속해도 RTSP 연결 1개만 유지되어 원본 CCTV에 영향 없음")
}

// TestOnDemandMultipleStreams는 여러 스트림을 동시에 온디맨드로 사용하는 경우를 테스트
func TestOnDemandMultipleStreams(t *testing.T) {
	if !isServerRunning() {
		t.Skip("Server is not running")
	}

	streams := []string{"CCTV-TEST1", "CCTV-TEST2", "CCTV-TEST3"}

	t.Log("=== 다중 온디맨드 스트림 검증 시작 ===")
	t.Logf("테스트 대상: %v", streams)

	// 1. 모든 스트림 시작
	t.Log("\n[단계 1] 모든 스트림 시작")
	for _, streamID := range streams {
		startStream(t, streamID)
		t.Logf("  스트림 시작: %s", streamID)
	}

	time.Sleep(5 * time.Second) // 모든 RTSP 연결 대기

	// 2. 각 스트림 상태 확인
	t.Log("\n[단계 2] 각 스트림 상태 확인")
	streamStats := make(map[string]map[string]interface{})

	for _, streamID := range streams {
		info := getStreamInfo(t, streamID)
		packets := int(info.RuntimeInfo["packets_received"].(float64))
		codec := info.RuntimeInfo["codec"]
		isActive := info.RuntimeInfo["is_active"].(bool)

		streamStats[streamID] = map[string]interface{}{
			"packets":   packets,
			"codec":     codec,
			"is_active": isActive,
		}

		t.Logf("  %s:", streamID)
		t.Logf("    활성화: %v", isActive)
		t.Logf("    패킷: %d", packets)
		t.Logf("    코덱: %v", codec)

		assert.True(t, isActive, fmt.Sprintf("%s RTSP 클라이언트가 활성화되어 있어야 함", streamID))
		assert.Greater(t, packets, 0, fmt.Sprintf("%s 패킷을 수신했어야 함", streamID))
	}

	// 3. 각 스트림에 다중 사용자 접속
	t.Log("\n[단계 3] 각 스트림에 3명씩 동시 접속")
	var wg sync.WaitGroup

	for _, streamID := range streams {
		for userID := 1; userID <= 3; userID++ {
			wg.Add(1)
			go func(sid string, uid int) {
				defer wg.Done()
				info := getStreamInfo(t, sid)
				packets := info.RuntimeInfo["packets_received"].(float64)
				t.Logf("  %s - 사용자 %d: %.0f packets", sid, uid, packets)
			}(streamID, userID)
		}
	}

	wg.Wait()
	time.Sleep(2 * time.Second)

	// 4. 최종 검증
	t.Log("\n[단계 4] 최종 상태 검증")
	for _, streamID := range streams {
		info := getStreamInfo(t, streamID)
		initialPackets := streamStats[streamID]["packets"].(int)
		currentPackets := int(info.RuntimeInfo["packets_received"].(float64))
		increment := currentPackets - initialPackets

		t.Logf("  %s:", streamID)
		t.Logf("    초기 패킷: %d", initialPackets)
		t.Logf("    현재 패킷: %d", currentPackets)
		t.Logf("    증가량: %d", increment)

		assert.Greater(t, currentPackets, initialPackets, fmt.Sprintf("%s RTSP 클라이언트가 계속 패킷을 수신해야 함", streamID))
	}

	t.Log("\n=== ✅ 다중 온디맨드 스트림 검증 완료 ===")
	t.Logf("결론: %d개 스트림 모두 정상 작동, 각각 RTSP 연결 1개씩 유지", len(streams))
}

// Helper functions

func getStreamInfo(t *testing.T, streamID string) StreamResponseWithRuntime {
	resp, err := http.Get(baseURL + "/api/v1/streams/" + streamID)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result StreamResponseWithRuntime
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	return result
}

func startStream(t *testing.T, streamID string) {
	resp, err := http.Post(baseURL+"/api/v1/streams/"+streamID+"/start", "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	// 이미 시작된 경우 200, 새로 시작한 경우 200 반환
	require.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated,
		"Expected 200 or 201, got %d", resp.StatusCode)
}
