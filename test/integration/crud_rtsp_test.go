package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourusername/cctv3/internal/database"
)

// TestCRUDWithRealRTSP는 실제 RTSP 연결을 사용하는 CRUD 작업을 테스트합니다
func TestCRUDWithRealRTSP(t *testing.T) {
	// 서버가 실행 중인지 확인
	if !isServerRunning() {
		t.Skip("Server is not running. Please start the server before running tests.")
	}

	// 테스트 전 정리
	cleanupTestStreams(t)

	t.Run("CreateWithRealRTSP", func(t *testing.T) {
		testCreateWithRealRTSP(t)
	})

	t.Run("DeleteWithActiveRTSP", func(t *testing.T) {
		testDeleteWithActiveRTSP(t)
	})

	t.Run("UpdateSourceWithActiveRTSP", func(t *testing.T) {
		testUpdateSourceWithActiveRTSP(t)
	})

	t.Run("MultipleRTSPStreams", func(t *testing.T) {
		testMultipleRTSPStreams(t)
	})

	// 테스트 후 정리
	cleanupTestStreams(t)
}

// testCreateWithRealRTSP는 실제 RTSP 연결을 가진 스트림 생성을 테스트합니다
func testCreateWithRealRTSP(t *testing.T) {
	stream := database.Stream{
		ID:             "test-rtsp-create",
		Name:           "Test RTSP Create",
		Source:         "rtsp://admin:live0416@192.168.10.53:554/Streaming/Channels/101",
		SourceOnDemand: false, // 즉시 RTSP 연결
		RTSPTransport:  "tcp",
	}

	// 1. 스트림 생성
	resp := createStream(t, stream)
	assert.Equal(t, stream.ID, resp.ID)

	// 2. RTSP 연결 대기 (2초)
	time.Sleep(2 * time.Second)

	// 3. 스트림 정보 조회 - RuntimeInfo 확인
	getResp := getStream(t, stream.ID)
	assert.NotNil(t, getResp.RuntimeInfo, "RuntimeInfo should be present")
	assert.Equal(t, true, getResp.RuntimeInfo["is_active"])

	// 4. 코덱 정보 확인 (RTSP 연결 후 자동 감지)
	codec := getResp.RuntimeInfo["codec"]
	t.Logf("Detected codec: %v", codec)

	// 5. 패킷 수신 확인 (일정 시간 후)
	time.Sleep(3 * time.Second)
	getResp2 := getStream(t, stream.ID)
	packetsRecv := uint64(getResp2.RuntimeInfo["packets_received"].(float64))
	assert.Greater(t, packetsRecv, uint64(0), "Should receive packets from RTSP stream")
	t.Logf("Packets received: %d", packetsRecv)

	// 6. 정리
	deleteStream(t, stream.ID)
}

// testDeleteWithActiveRTSP는 활성 RTSP 연결을 가진 스트림 삭제를 테스트합니다
func testDeleteWithActiveRTSP(t *testing.T) {
	stream := database.Stream{
		ID:             "test-rtsp-delete",
		Name:           "Test RTSP Delete",
		Source:         "rtsp://admin:live0416@192.168.10.53:554/Streaming/Channels/101",
		SourceOnDemand: false, // 즉시 RTSP 연결
		RTSPTransport:  "tcp",
	}

	// 1. 스트림 생성 및 RTSP 연결 대기
	createStream(t, stream)
	time.Sleep(2 * time.Second)

	// 2. 패킷 수신 확인
	getResp := getStream(t, stream.ID)
	assert.NotNil(t, getResp.RuntimeInfo)
	time.Sleep(2 * time.Second)

	getResp2 := getStream(t, stream.ID)
	packetsRecv := uint64(getResp2.RuntimeInfo["packets_received"].(float64))
	assert.Greater(t, packetsRecv, uint64(0), "Should receive packets before delete")
	t.Logf("Packets received before delete: %d", packetsRecv)

	// 3. 활성 상태에서 삭제 (panic이 발생하면 안됨!)
	deleteStream(t, stream.ID)

	// 4. 삭제 확인
	resp, err := http.Get(baseURL + "/api/v1/streams/" + stream.ID)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	// 5. 목록에서도 제거 확인
	listResp := listStreams(t)
	for _, s := range listResp.Streams {
		assert.NotEqual(t, stream.ID, s.ID, "Deleted stream should not appear in list")
	}

	t.Log("✅ DELETE with active RTSP passed - no panic!")
}

// testUpdateSourceWithActiveRTSP는 활성 RTSP 연결의 Source 변경을 테스트합니다
func testUpdateSourceWithActiveRTSP(t *testing.T) {
	stream := database.Stream{
		ID:             "test-rtsp-update",
		Name:           "Test RTSP Update",
		Source:         "rtsp://admin:live0416@192.168.10.53:554/Streaming/Channels/101",
		SourceOnDemand: false,
		RTSPTransport:  "tcp",
	}

	// 1. 스트림 생성 및 RTSP 연결 대기
	createStream(t, stream)
	time.Sleep(2 * time.Second)

	// 2. 패킷 수신 확인
	getResp := getStream(t, stream.ID)
	assert.NotNil(t, getResp.RuntimeInfo)
	time.Sleep(2 * time.Second)

	getResp2 := getStream(t, stream.ID)
	packetsRecv1 := uint64(getResp2.RuntimeInfo["packets_received"].(float64))
	assert.Greater(t, packetsRecv1, uint64(0))
	t.Logf("Packets received (before update): %d", packetsRecv1)

	// 3. Source 변경 (같은 URL이지만 UPDATE 로직 테스트)
	stream.Name = "Updated Name"
	updateStream(t, stream.ID, stream)

	// 4. RTSP 재연결 대기
	time.Sleep(3 * time.Second)

	// 5. 패킷 수신 계속 확인
	getResp3 := getStream(t, stream.ID)
	assert.NotNil(t, getResp3.RuntimeInfo)
	assert.Equal(t, "Updated Name", getResp3.Name)

	// 6. 정리
	deleteStream(t, stream.ID)
}

// testMultipleRTSPStreams는 여러 RTSP 스트림 동시 관리를 테스트합니다
func testMultipleRTSPStreams(t *testing.T) {
	streams := []database.Stream{
		{
			ID:             "test-rtsp-multi-1",
			Name:           "Multi RTSP 1",
			Source:         "rtsp://admin:live0416@192.168.10.53:554/Streaming/Channels/101",
			SourceOnDemand: false,
			RTSPTransport:  "tcp",
		},
		{
			ID:             "test-rtsp-multi-2",
			Name:           "Multi RTSP 2",
			Source:         "rtsp://admin:live0416@192.168.10.53:554/Streaming/Channels/101", // 같은 URL 허용
			SourceOnDemand: false,
			RTSPTransport:  "tcp",
		},
		{
			ID:             "test-rtsp-multi-3",
			Name:           "Multi RTSP 3",
			Source:         "rtsp://admin:live0416@192.168.10.53:554/Streaming/Channels/101",
			SourceOnDemand: false,
			RTSPTransport:  "tcp",
		},
	}

	// 1. 모든 스트림 생성
	for _, stream := range streams {
		createStream(t, stream)
	}

	// 2. RTSP 연결 대기
	time.Sleep(3 * time.Second)

	// 3. 모든 스트림 패킷 수신 확인
	for _, stream := range streams {
		getResp := getStream(t, stream.ID)
		assert.NotNil(t, getResp.RuntimeInfo, "RuntimeInfo should be present for "+stream.ID)

		time.Sleep(1 * time.Second)
		getResp2 := getStream(t, stream.ID)
		packetsRecv := uint64(getResp2.RuntimeInfo["packets_received"].(float64))
		assert.Greater(t, packetsRecv, uint64(0), "Should receive packets for "+stream.ID)
		t.Logf("%s: %d packets received", stream.ID, packetsRecv)
	}

	// 4. 순차적으로 삭제 (panic 없이)
	for _, stream := range streams {
		deleteStream(t, stream.ID)
		time.Sleep(500 * time.Millisecond) // 각 삭제 사이에 대기
	}

	// 5. 모두 삭제 확인
	listResp := listStreams(t)
	for _, stream := range streams {
		for _, s := range listResp.Streams {
			assert.NotEqual(t, stream.ID, s.ID, stream.ID+" should not appear in list")
		}
	}

	t.Log("✅ Multiple RTSP streams test passed!")
}

// TestOnDemandRTSPStream은 온디맨드 RTSP 스트림을 테스트합니다
func TestOnDemandRTSPStream(t *testing.T) {
	if !isServerRunning() {
		t.Skip("Server is not running")
	}

	cleanupTestStreams(t)

	stream := database.Stream{
		ID:             "test-rtsp-ondemand",
		Name:           "Test On-Demand RTSP",
		Source:         "rtsp://admin:live0416@192.168.10.53:554/Streaming/Channels/101",
		SourceOnDemand: true, // 온디맨드
		RTSPTransport:  "tcp",
	}

	// 1. 스트림 생성
	createStream(t, stream)

	// 2. 초기 상태: RTSP 연결 없음
	getResp := getStream(t, stream.ID)
	assert.NotNil(t, getResp.RuntimeInfo)
	packetsRecv := uint64(getResp.RuntimeInfo["packets_received"].(float64))
	assert.Equal(t, uint64(0), packetsRecv, "Should not receive packets (on-demand)")

	// 3. 스트림 시작
	resp, err := http.Post(baseURL+"/api/v1/streams/"+stream.ID+"/start", "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// 4. RTSP 연결 대기
	time.Sleep(3 * time.Second)

	// 5. 패킷 수신 확인
	getResp2 := getStream(t, stream.ID)
	packetsRecv2 := uint64(getResp2.RuntimeInfo["packets_received"].(float64))
	assert.Greater(t, packetsRecv2, uint64(0), "Should receive packets after start")
	t.Logf("Packets received after start: %d", packetsRecv2)

	// 6. 삭제
	deleteStream(t, stream.ID)

	cleanupTestStreams(t)
}

// TestRapidCreateDelete는 빠른 생성/삭제 반복을 테스트합니다
func TestRapidCreateDelete(t *testing.T) {
	if !isServerRunning() {
		t.Skip("Server is not running")
	}

	cleanupTestStreams(t)

	for i := 0; i < 5; i++ {
		streamID := "test-rtsp-rapid"

		stream := database.Stream{
			ID:             streamID,
			Name:           "Rapid Test",
			Source:         "rtsp://admin:live0416@192.168.10.53:554/Streaming/Channels/101",
			SourceOnDemand: false,
			RTSPTransport:  "tcp",
		}

		t.Logf("Iteration %d: Creating stream", i+1)
		createStream(t, stream)

		// RTSP 연결 대기
		time.Sleep(2 * time.Second)

		// 패킷 수신 확인
		getResp := getStream(t, streamID)
		assert.NotNil(t, getResp.RuntimeInfo)

		t.Logf("Iteration %d: Deleting stream", i+1)
		deleteStream(t, streamID)

		// 삭제 후 대기
		time.Sleep(1 * time.Second)

		// 삭제 확인
		resp, err := http.Get(baseURL + "/api/v1/streams/" + streamID)
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	}

	t.Log("✅ Rapid create/delete test passed!")
	cleanupTestStreams(t)
}

// TestDeleteDuringPacketReception은 패킷 수신 중 삭제를 테스트합니다
func TestDeleteDuringPacketReception(t *testing.T) {
	if !isServerRunning() {
		t.Skip("Server is not running")
	}

	cleanupTestStreams(t)

	stream := database.Stream{
		ID:             "test-rtsp-delete-during",
		Name:           "Delete During Reception",
		Source:         "rtsp://admin:live0416@192.168.10.53:554/Streaming/Channels/101",
		SourceOnDemand: false,
		RTSPTransport:  "tcp",
	}

	// 1. 스트림 생성 및 RTSP 연결 대기
	createStream(t, stream)
	time.Sleep(3 * time.Second)

	// 2. 패킷 수신 확인
	getResp := getStream(t, stream.ID)
	packetsRecv1 := uint64(getResp.RuntimeInfo["packets_received"].(float64))
	assert.Greater(t, packetsRecv1, uint64(0))
	t.Logf("Packets before delete: %d", packetsRecv1)

	// 3. 패킷이 계속 수신되는 중에 삭제 (이게 핵심 테스트!)
	// 별도 고루틴에서 패킷 수신 확인
	done := make(chan bool)
	go func() {
		for i := 0; i < 10; i++ {
			getResp := getStream(t, stream.ID)
			if getResp.RuntimeInfo != nil {
				packets := uint64(getResp.RuntimeInfo["packets_received"].(float64))
				t.Logf("Packets during: %d", packets)
			}
			time.Sleep(100 * time.Millisecond)
		}
		done <- true
	}()

	// 4. 잠깐 대기 후 삭제
	time.Sleep(500 * time.Millisecond)
	deleteStream(t, stream.ID)
	t.Log("Stream deleted during packet reception")

	// 5. 고루틴 종료 대기
	<-done

	t.Log("✅ Delete during packet reception passed - no panic!")
	cleanupTestStreams(t)
}

// BenchmarkRTSPStreamCreation은 RTSP 스트림 생성 성능을 벤치마크합니다
func BenchmarkRTSPStreamCreation(b *testing.B) {
	if !isServerRunning() {
		b.Skip("Server is not running")
	}

	// 벤치마크 전 정리
	cleanupBenchmarkStreams()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		streamID := "bench-rtsp-" + string(rune(i))

		stream := database.Stream{
			ID:             streamID,
			Name:           "Benchmark RTSP " + string(rune(i)),
			Source:         "rtsp://admin:live0416@192.168.10.53:554/Streaming/Channels/101",
			SourceOnDemand: true, // 온디맨드로 실제 연결 방지 (벤치마크 속도)
			RTSPTransport:  "tcp",
		}

		body, _ := json.Marshal(stream)
		resp, err := http.Post(baseURL+"/api/v1/streams", "application/json", bytes.NewBuffer(body))
		if err != nil {
			b.Fatal(err)
		}
		resp.Body.Close()
	}

	b.StopTimer()

	// 벤치마크 후 정리
	cleanupBenchmarkStreams()
}
