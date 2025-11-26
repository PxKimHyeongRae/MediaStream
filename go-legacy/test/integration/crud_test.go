package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourusername/cctv3/internal/database"
)

const (
	baseURL = "http://localhost:8107"
)

// StreamResponse는 API 응답 구조체
type StreamResponse struct {
	ID             string                 `json:"id"`
	Name           string                 `json:"name"`
	Source         string                 `json:"source"`
	SourceOnDemand bool                   `json:"source_on_demand"`
	RTSPTransport  string                 `json:"rtsp_transport"`
	CreatedAt      string                 `json:"created_at"`
	UpdatedAt      string                 `json:"updated_at"`
	RuntimeInfo    map[string]interface{} `json:"runtime_info,omitempty"`
}

// ListResponse는 스트림 목록 응답
type ListResponse struct {
	Streams []StreamResponse `json:"streams"`
	Count   int              `json:"count"`
}

// PathsListResponse는 mediaMTX 호환 응답
type PathsListResponse struct {
	Items     []PathItem `json:"items"`
	ItemCount int        `json:"itemCount"`
	PageCount int        `json:"pageCount"`
}

type PathItem struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

// ErrorResponse는 에러 응답
type ErrorResponse struct {
	Error string `json:"error"`
}

// SuccessResponse는 성공 응답
type SuccessResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	ID      string `json:"id"`
}

// TestCRUDOperations는 CRUD 작업의 모든 경우의 수를 테스트합니다
func TestCRUDOperations(t *testing.T) {
	// 서버가 실행 중인지 확인
	if !isServerRunning() {
		t.Skip("Server is not running. Please start the server before running tests.")
	}

	// 테스트 전 정리
	cleanupTestStreams(t)

	t.Run("Create", func(t *testing.T) {
		testCreate(t)
	})

	t.Run("Read", func(t *testing.T) {
		testRead(t)
	})

	t.Run("Update", func(t *testing.T) {
		testUpdate(t)
	})

	t.Run("Delete", func(t *testing.T) {
		testDelete(t)
	})

	t.Run("EdgeCases", func(t *testing.T) {
		testEdgeCases(t)
	})

	t.Run("StreamManagerIntegration", func(t *testing.T) {
		testStreamManagerIntegration(t)
	})

	t.Run("MediaMTXCompatibility", func(t *testing.T) {
		testMediaMTXCompatibility(t)
	})

	// 테스트 후 정리
	cleanupTestStreams(t)
}

// testCreate는 Create 작업의 모든 경우를 테스트합니다
func testCreate(t *testing.T) {
	t.Run("CreateValidStream", func(t *testing.T) {
		stream := database.Stream{
			ID:             "test-create-1",
			Name:           "Test Create Stream 1",
			Source:         "rtsp://test.com/stream1",
			SourceOnDemand: true,
			RTSPTransport:  "tcp",
		}

		resp := createStream(t, stream)
		assert.Equal(t, stream.ID, resp.ID)
		assert.Equal(t, stream.Name, resp.Name)
		assert.Equal(t, stream.Source, resp.Source)
		assert.Equal(t, stream.SourceOnDemand, resp.SourceOnDemand)
		assert.Equal(t, stream.RTSPTransport, resp.RTSPTransport)
		assert.NotEmpty(t, resp.CreatedAt)
		assert.NotEmpty(t, resp.UpdatedAt)
	})

	t.Run("CreateWithoutID_UseNameAsID", func(t *testing.T) {
		stream := database.Stream{
			Name:           "test-create-2",
			Source:         "rtsp://test.com/stream2",
			SourceOnDemand: true,
			RTSPTransport:  "tcp",
		}

		resp := createStream(t, stream)
		assert.Equal(t, stream.Name, resp.ID, "ID should default to Name")
		assert.Equal(t, stream.Name, resp.Name)
	})

	t.Run("CreateWithDefaultTransport", func(t *testing.T) {
		stream := database.Stream{
			ID:             "test-create-3",
			Name:           "Test Create Stream 3",
			Source:         "rtsp://test.com/stream3",
			SourceOnDemand: false,
			// RTSPTransport 생략
		}

		resp := createStream(t, stream)
		assert.Equal(t, "tcp", resp.RTSPTransport, "RTSPTransport should default to 'tcp'")
	})

	t.Run("CreateDuplicate_ShouldFail", func(t *testing.T) {
		stream := database.Stream{
			ID:             "test-duplicate",
			Name:           "Duplicate Stream",
			Source:         "rtsp://test.com/duplicate",
			SourceOnDemand: true,
			RTSPTransport:  "tcp",
		}

		// 첫 번째 생성은 성공
		createStream(t, stream)

		// 두 번째 생성은 실패해야 함
		body, _ := json.Marshal(stream)
		resp, err := http.Post(baseURL+"/api/v1/streams", "application/json", bytes.NewBuffer(body))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

		var errResp ErrorResponse
		json.NewDecoder(resp.Body).Decode(&errResp)
		assert.Contains(t, errResp.Error, "UNIQUE constraint failed")
	})

	t.Run("CreateWithInvalidJSON_ShouldFail", func(t *testing.T) {
		invalidJSON := []byte(`{"id": "invalid", "name": `)

		resp, err := http.Post(baseURL+"/api/v1/streams", "application/json", bytes.NewBuffer(invalidJSON))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

// testRead는 Read 작업의 모든 경우를 테스트합니다
func testRead(t *testing.T) {
	// 테스트 데이터 생성
	streams := []database.Stream{
		{
			ID:             "test-read-1",
			Name:           "Test Read Stream 1",
			Source:         "rtsp://test.com/read1",
			SourceOnDemand: true,
			RTSPTransport:  "tcp",
		},
		{
			ID:             "test-read-2",
			Name:           "Test Read Stream 2",
			Source:         "rtsp://test.com/read2",
			SourceOnDemand: false,
			RTSPTransport:  "udp",
		},
	}

	for _, stream := range streams {
		createStream(t, stream)
	}

	t.Run("GetStreamByID", func(t *testing.T) {
		resp := getStream(t, "test-read-1")
		assert.Equal(t, "test-read-1", resp.ID)
		assert.Equal(t, "Test Read Stream 1", resp.Name)
		assert.NotNil(t, resp.RuntimeInfo, "RuntimeInfo should be present")
		assert.Equal(t, true, resp.RuntimeInfo["is_active"])
	})

	t.Run("GetNonExistentStream_ShouldFail", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/v1/streams/non-existent-stream")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("ListAllStreams", func(t *testing.T) {
		resp := listStreams(t)
		assert.GreaterOrEqual(t, resp.Count, 2)
		assert.Len(t, resp.Streams, resp.Count)

		// 모든 스트림에 RuntimeInfo가 있는지 확인
		for _, stream := range resp.Streams {
			if stream.ID == "test-read-1" || stream.ID == "test-read-2" {
				assert.NotNil(t, stream.RuntimeInfo, "RuntimeInfo should be present for "+stream.ID)
			}
		}
	})

	t.Run("ListStreamsCount", func(t *testing.T) {
		resp := listStreams(t)
		assert.Equal(t, len(resp.Streams), resp.Count)
	})
}

// testUpdate는 Update 작업의 모든 경우를 테스트합니다
func testUpdate(t *testing.T) {
	// 테스트 데이터 생성
	original := database.Stream{
		ID:             "test-update-1",
		Name:           "Original Name",
		Source:         "rtsp://test.com/original",
		SourceOnDemand: true,
		RTSPTransport:  "tcp",
	}
	createStream(t, original)

	t.Run("UpdateName", func(t *testing.T) {
		updated := database.Stream{
			ID:             "test-update-1",
			Name:           "Updated Name",
			Source:         original.Source,
			SourceOnDemand: original.SourceOnDemand,
			RTSPTransport:  original.RTSPTransport,
		}

		resp := updateStream(t, "test-update-1", updated)
		assert.Equal(t, "Updated Name", resp.Name)
	})

	t.Run("UpdateSource", func(t *testing.T) {
		updated := database.Stream{
			ID:             "test-update-1",
			Name:           "Updated Name",
			Source:         "rtsp://test.com/updated-source",
			SourceOnDemand: original.SourceOnDemand,
			RTSPTransport:  original.RTSPTransport,
		}

		resp := updateStream(t, "test-update-1", updated)
		assert.Equal(t, "rtsp://test.com/updated-source", resp.Source)
	})

	t.Run("UpdateSourceOnDemand", func(t *testing.T) {
		updated := database.Stream{
			ID:             "test-update-1",
			Name:           "Updated Name",
			Source:         "rtsp://test.com/updated-source",
			SourceOnDemand: false, // true -> false
			RTSPTransport:  original.RTSPTransport,
		}

		resp := updateStream(t, "test-update-1", updated)
		assert.Equal(t, false, resp.SourceOnDemand)
	})

	t.Run("UpdateRTSPTransport", func(t *testing.T) {
		updated := database.Stream{
			ID:             "test-update-1",
			Name:           "Updated Name",
			Source:         "rtsp://test.com/updated-source",
			SourceOnDemand: false,
			RTSPTransport:  "udp", // tcp -> udp
		}

		resp := updateStream(t, "test-update-1", updated)
		assert.Equal(t, "udp", resp.RTSPTransport)
	})

	t.Run("UpdateNonExistentStream_ShouldFail", func(t *testing.T) {
		updated := database.Stream{
			ID:             "non-existent",
			Name:           "Non Existent",
			Source:         "rtsp://test.com/none",
			SourceOnDemand: true,
			RTSPTransport:  "tcp",
		}

		body, _ := json.Marshal(updated)
		req, _ := http.NewRequest(http.MethodPut, baseURL+"/api/v1/streams/non-existent", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("UpdateWithInvalidJSON_ShouldFail", func(t *testing.T) {
		invalidJSON := []byte(`{"name": "Invalid"`)

		req, _ := http.NewRequest(http.MethodPut, baseURL+"/api/v1/streams/test-update-1", bytes.NewBuffer(invalidJSON))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

// testDelete는 Delete 작업의 모든 경우를 테스트합니다
func testDelete(t *testing.T) {
	t.Run("DeleteExistingStream", func(t *testing.T) {
		// 스트림 생성
		stream := database.Stream{
			ID:             "test-delete-1",
			Name:           "Test Delete Stream 1",
			Source:         "rtsp://test.com/delete1",
			SourceOnDemand: true,
			RTSPTransport:  "tcp",
		}
		createStream(t, stream)

		// 삭제
		deleteStream(t, "test-delete-1")

		// 삭제 확인 - GET 시도하면 404
		resp, err := http.Get(baseURL + "/api/v1/streams/test-delete-1")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("DeleteNonExistentStream_ShouldFail", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, baseURL+"/api/v1/streams/non-existent", nil)

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// 존재하지 않는 스트림 삭제 시도는 실패해야 함
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("DeleteAndRecreate", func(t *testing.T) {
		stream := database.Stream{
			ID:             "test-delete-recreate",
			Name:           "Test Delete Recreate",
			Source:         "rtsp://test.com/delete-recreate",
			SourceOnDemand: true,
			RTSPTransport:  "tcp",
		}

		// 생성
		createStream(t, stream)

		// 삭제
		deleteStream(t, "test-delete-recreate")

		// 같은 ID로 재생성
		resp := createStream(t, stream)
		assert.Equal(t, stream.ID, resp.ID)
		assert.Equal(t, stream.Name, resp.Name)
	})

	t.Run("DeleteRemovedFromList", func(t *testing.T) {
		stream := database.Stream{
			ID:             "test-delete-list",
			Name:           "Test Delete From List",
			Source:         "rtsp://test.com/delete-list",
			SourceOnDemand: true,
			RTSPTransport:  "tcp",
		}

		createStream(t, stream)

		// 삭제 전 목록 확인
		beforeList := listStreams(t)
		beforeCount := beforeList.Count

		// 삭제
		deleteStream(t, "test-delete-list")

		// 삭제 후 목록 확인
		afterList := listStreams(t)
		afterCount := afterList.Count

		assert.Equal(t, beforeCount-1, afterCount, "Count should decrease by 1")

		// 삭제된 스트림이 목록에 없는지 확인
		for _, s := range afterList.Streams {
			assert.NotEqual(t, "test-delete-list", s.ID, "Deleted stream should not appear in list")
		}
	})
}

// testEdgeCases는 엣지 케이스를 테스트합니다
func testEdgeCases(t *testing.T) {
	t.Run("EmptyName", func(t *testing.T) {
		stream := database.Stream{
			ID:             "test-edge-empty-name",
			Name:           "", // 빈 이름
			Source:         "rtsp://test.com/empty-name",
			SourceOnDemand: true,
			RTSPTransport:  "tcp",
		}

		resp := createStream(t, stream)
		assert.Equal(t, "", resp.Name)
	})

	t.Run("SpecialCharactersInID", func(t *testing.T) {
		stream := database.Stream{
			ID:             "test-edge-special_chars-123",
			Name:           "Special Chars Test",
			Source:         "rtsp://test.com/special",
			SourceOnDemand: true,
			RTSPTransport:  "tcp",
		}

		resp := createStream(t, stream)
		assert.Equal(t, "test-edge-special_chars-123", resp.ID)
	})

	t.Run("LongStreamName", func(t *testing.T) {
		longName := "This is a very long stream name that contains many characters and should still work properly without any issues even though it is extremely long"

		stream := database.Stream{
			ID:             "test-edge-long-name",
			Name:           longName,
			Source:         "rtsp://test.com/long-name",
			SourceOnDemand: true,
			RTSPTransport:  "tcp",
		}

		resp := createStream(t, stream)
		assert.Equal(t, longName, resp.Name)
	})

	t.Run("RTSPURLWithAuth", func(t *testing.T) {
		stream := database.Stream{
			ID:             "test-edge-auth",
			Name:           "Auth Test",
			Source:         "rtsp://admin:password123@192.168.1.100:554/stream",
			SourceOnDemand: true,
			RTSPTransport:  "tcp",
		}

		resp := createStream(t, stream)
		assert.Equal(t, "rtsp://admin:password123@192.168.1.100:554/stream", resp.Source)
	})

	t.Run("RTSPURLWithSpecialCharsInPassword", func(t *testing.T) {
		stream := database.Stream{
			ID:             "test-edge-special-pwd",
			Name:           "Special Password Test",
			Source:         "rtsp://admin:p@ssw0rd%21@192.168.1.100:554/stream",
			SourceOnDemand: true,
			RTSPTransport:  "tcp",
		}

		resp := createStream(t, stream)
		assert.Contains(t, resp.Source, "p@ssw0rd%21")
	})
}

// testStreamManagerIntegration은 StreamManager와의 통합을 테스트합니다
func testStreamManagerIntegration(t *testing.T) {
	t.Run("CreateAddsToStreamManager", func(t *testing.T) {
		stream := database.Stream{
			ID:             "test-sm-create",
			Name:           "StreamManager Create Test",
			Source:         "rtsp://test.com/sm-create",
			SourceOnDemand: true,
			RTSPTransport:  "tcp",
		}

		resp := createStream(t, stream)

		// RuntimeInfo가 존재하는지 확인 (StreamManager에 Stream 객체가 생성되었음을 의미)
		getResp := getStream(t, resp.ID)
		assert.NotNil(t, getResp.RuntimeInfo, "RuntimeInfo should be present")
		assert.Equal(t, true, getResp.RuntimeInfo["is_active"])
		assert.Equal(t, float64(0), getResp.RuntimeInfo["subscriber_count"])
	})

	t.Run("DeleteRemovesFromStreamManager", func(t *testing.T) {
		stream := database.Stream{
			ID:             "test-sm-delete",
			Name:           "StreamManager Delete Test",
			Source:         "rtsp://test.com/sm-delete",
			SourceOnDemand: true,
			RTSPTransport:  "tcp",
		}

		createStream(t, stream)
		deleteStream(t, "test-sm-delete")

		// 삭제 후 GET 시도하면 404 (StreamManager에서도 제거됨)
		resp, err := http.Get(baseURL + "/api/v1/streams/test-sm-delete")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("RuntimeInfoInList", func(t *testing.T) {
		stream := database.Stream{
			ID:             "test-sm-list",
			Name:           "StreamManager List Test",
			Source:         "rtsp://test.com/sm-list",
			SourceOnDemand: true,
			RTSPTransport:  "tcp",
		}

		createStream(t, stream)

		listResp := listStreams(t)

		// 생성한 스트림을 찾아서 RuntimeInfo 확인
		found := false
		for _, s := range listResp.Streams {
			if s.ID == "test-sm-list" {
				found = true
				assert.NotNil(t, s.RuntimeInfo, "RuntimeInfo should be present in list")
				break
			}
		}
		assert.True(t, found, "Created stream should appear in list")
	})
}

// testMediaMTXCompatibility는 mediaMTX 호환성을 테스트합니다
func testMediaMTXCompatibility(t *testing.T) {
	// 테스트 데이터 생성
	streams := []database.Stream{
		{
			ID:             "test-mtx-1",
			Name:           "MediaMTX Test 1",
			Source:         "rtsp://test.com/mtx1",
			SourceOnDemand: true,
			RTSPTransport:  "tcp",
		},
		{
			ID:             "test-mtx-2",
			Name:           "MediaMTX Test 2",
			Source:         "rtsp://test.com/mtx2",
			SourceOnDemand: false,
			RTSPTransport:  "udp",
		},
	}

	for _, stream := range streams {
		createStream(t, stream)
	}

	t.Run("PathsListFormat", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/v3/config/paths/list")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var pathsResp PathsListResponse
		err = json.NewDecoder(resp.Body).Decode(&pathsResp)
		require.NoError(t, err)

		assert.GreaterOrEqual(t, pathsResp.ItemCount, 2)
		assert.Equal(t, len(pathsResp.Items), pathsResp.ItemCount)
		assert.Equal(t, 1, pathsResp.PageCount)
	})

	t.Run("PathsListContainsStreams", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/v3/config/paths/list")
		require.NoError(t, err)
		defer resp.Body.Close()

		var pathsResp PathsListResponse
		json.NewDecoder(resp.Body).Decode(&pathsResp)

		// test-mtx-1, test-mtx-2가 포함되어 있는지 확인
		foundMTX1 := false
		foundMTX2 := false

		for _, item := range pathsResp.Items {
			if item.Name == "test-mtx-1" {
				foundMTX1 = true
				assert.Equal(t, "rtsp://test.com/mtx1", item.Source)
			}
			if item.Name == "test-mtx-2" {
				foundMTX2 = true
				assert.Equal(t, "rtsp://test.com/mtx2", item.Source)
			}
		}

		assert.True(t, foundMTX1, "test-mtx-1 should appear in paths list")
		assert.True(t, foundMTX2, "test-mtx-2 should appear in paths list")
	})
}

// Helper functions

func isServerRunning() bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(baseURL + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func createStream(t *testing.T, stream database.Stream) StreamResponse {
	body, err := json.Marshal(stream)
	require.NoError(t, err)

	resp, err := http.Post(baseURL+"/api/v1/streams", "application/json", bytes.NewBuffer(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Failed to create stream")

	var result StreamResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	return result
}

func getStream(t *testing.T, id string) StreamResponse {
	resp, err := http.Get(baseURL + "/api/v1/streams/" + id)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "Failed to get stream")

	var result StreamResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	return result
}

func listStreams(t *testing.T) ListResponse {
	resp, err := http.Get(baseURL + "/api/v1/streams")
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "Failed to list streams")

	var result ListResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	return result
}

func updateStream(t *testing.T, id string, stream database.Stream) StreamResponse {
	body, err := json.Marshal(stream)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPut, baseURL+"/api/v1/streams/"+id, bytes.NewBuffer(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "Failed to update stream")

	var result StreamResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	return result
}

func deleteStream(t *testing.T, id string) {
	req, err := http.NewRequest(http.MethodDelete, baseURL+"/api/v1/streams/"+id, nil)
	require.NoError(t, err)

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "Failed to delete stream")

	var result SuccessResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, "success", result.Status)
	assert.Equal(t, id, result.ID)
}

func cleanupTestStreams(t *testing.T) {
	// 테스트 스트림 목록 조회
	resp, err := http.Get(baseURL + "/api/v1/streams")
	if err != nil {
		return // 서버가 실행 중이 아니면 스킵
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	var listResp ListResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return
	}

	// "test-"로 시작하는 모든 스트림 삭제
	for _, stream := range listResp.Streams {
		if len(stream.ID) >= 5 && stream.ID[:5] == "test-" {
			req, _ := http.NewRequest(http.MethodDelete, baseURL+"/api/v1/streams/"+stream.ID, nil)
			client := &http.Client{}
			resp, err := client.Do(req)
			if err == nil {
				resp.Body.Close()
			}
		}
	}

	// 정리 후 잠시 대기
	time.Sleep(100 * time.Millisecond)
}

// TestHealthCheck는 헬스 체크를 테스트합니다
func TestHealthCheck(t *testing.T) {
	if !isServerRunning() {
		t.Skip("Server is not running")
	}

	resp, err := http.Get(baseURL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Contains(t, string(body), "status")
}

// TestStatsEndpoint는 통계 엔드포인트를 테스트합니다
func TestStatsEndpoint(t *testing.T) {
	if !isServerRunning() {
		t.Skip("Server is not running")
	}

	resp, err := http.Get(baseURL + "/api/v1/stats")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var stats map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&stats)
	require.NoError(t, err)

	// 통계에 필요한 필드가 있는지 확인
	assert.Contains(t, stats, "streams")
	assert.Contains(t, stats, "peers")
}

// BenchmarkCreateStream은 스트림 생성 성능을 벤치마크합니다
func BenchmarkCreateStream(b *testing.B) {
	if !isServerRunning() {
		b.Skip("Server is not running")
	}

	// 벤치마크 전 정리
	cleanupBenchmarkStreams()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		stream := database.Stream{
			ID:             fmt.Sprintf("bench-stream-%d", i),
			Name:           fmt.Sprintf("Benchmark Stream %d", i),
			Source:         fmt.Sprintf("rtsp://test.com/bench-%d", i),
			SourceOnDemand: true,
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

func cleanupBenchmarkStreams() {
	resp, err := http.Get(baseURL + "/api/v1/streams")
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var listResp ListResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return
	}

	for _, stream := range listResp.Streams {
		if len(stream.ID) >= 6 && stream.ID[:6] == "bench-" {
			req, _ := http.NewRequest(http.MethodDelete, baseURL+"/api/v1/streams/"+stream.ID, nil)
			client := &http.Client{}
			resp, err := client.Do(req)
			if err == nil {
				resp.Body.Close()
			}
		}
	}
}
