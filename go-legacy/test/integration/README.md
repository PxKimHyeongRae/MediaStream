# CRUD Integration Tests

Database-centric 아키텍처의 모든 CRUD 작업과 경우의 수를 테스트하는 종합 테스트 스위트입니다.

## 테스트 개요

이 테스트는 다음을 검증합니다:
- **Database as Single Source of Truth**: 모든 스트림 메타데이터가 데이터베이스에 저장됨
- **StreamManager Integration**: StreamManager가 런타임 정보를 제공
- **CRUD Operations**: Create, Read, Update, Delete 작업의 정확성
- **Edge Cases**: 특수 문자, 긴 이름, 인증 정보 등
- **mediaMTX Compatibility**: mediaMTX 호환 API 엔드포인트

## 테스트 실행

### 전제 조건
서버가 실행 중이어야 합니다:
```bash
./bin/media-server.exe
```

### 모든 테스트 실행
```bash
go test -v ./test/integration/crud_test.go
```

### 특정 테스트만 실행
```bash
# CRUD 작업 테스트
go test -v ./test/integration/crud_test.go -run TestCRUDOperations

# 헬스 체크 테스트
go test -v ./test/integration/crud_test.go -run TestHealthCheck

# 통계 엔드포인트 테스트
go test -v ./test/integration/crud_test.go -run TestStatsEndpoint
```

### 벤치마크 테스트
```bash
go test -v ./test/integration/crud_test.go -bench=BenchmarkCreateStream -benchtime=100x
```

## 테스트 커버리지

### 1. Create 작업 (5개 테스트)
- ✅ **CreateValidStream**: 유효한 스트림 생성
- ✅ **CreateWithoutID_UseNameAsID**: ID 없이 생성 시 Name을 ID로 사용
- ✅ **CreateWithDefaultTransport**: RTSPTransport 생략 시 기본값 "tcp" 사용
- ✅ **CreateDuplicate_ShouldFail**: 중복 ID 생성 시 실패 (UNIQUE constraint)
- ✅ **CreateWithInvalidJSON_ShouldFail**: 잘못된 JSON 형식 시 400 에러

**검증 항목**:
- Database에 저장됨
- StreamManager에 Stream 객체 생성됨
- 생성 시간(CreatedAt, UpdatedAt) 자동 설정
- 기본값 처리 (RTSPTransport)

### 2. Read 작업 (4개 테스트)
- ✅ **GetStreamByID**: ID로 스트림 조회
- ✅ **GetNonExistentStream_ShouldFail**: 존재하지 않는 스트림 조회 시 404
- ✅ **ListAllStreams**: 모든 스트림 목록 조회
- ✅ **ListStreamsCount**: 스트림 개수 일치 확인

**검증 항목**:
- Database에서 메타데이터 조회
- StreamManager에서 RuntimeInfo 조회
- RuntimeInfo 필드: is_active, codec, subscriber_count, packets_received, packets_sent, bytes_received, bytes_sent

### 3. Update 작업 (6개 테스트)
- ✅ **UpdateName**: 이름 변경
- ✅ **UpdateSource**: RTSP URL 변경
- ✅ **UpdateSourceOnDemand**: 온디맨드 설정 변경 (true → false)
- ✅ **UpdateRTSPTransport**: 전송 프로토콜 변경 (tcp → udp)
- ✅ **UpdateNonExistentStream_ShouldFail**: 존재하지 않는 스트림 업데이트 시 실패
- ✅ **UpdateWithInvalidJSON_ShouldFail**: 잘못된 JSON 형식 시 400 에러

**검증 항목**:
- Database 업데이트
- Source 변경 시 RTSP 클라이언트 재시작 (stopStreamHandler → startStreamHandler)
- UpdatedAt 시간 갱신

### 4. Delete 작업 (4개 테스트)
- ✅ **DeleteExistingStream**: 존재하는 스트림 삭제
- ✅ **DeleteNonExistentStream_ShouldFail**: 존재하지 않는 스트림 삭제 시 실패
- ✅ **DeleteAndRecreate**: 삭제 후 같은 ID로 재생성 가능
- ✅ **DeleteRemovedFromList**: 삭제된 스트림이 목록에서 제거됨

**검증 항목**:
- Database에서 삭제
- RTSP 클라이언트 정지 (stopStreamHandler)
- StreamManager에서 제거 (RemoveStream)
- 삭제 후 GET 시도하면 404

### 5. Edge Cases (5개 테스트)
- ✅ **EmptyName**: 빈 이름 처리
- ✅ **SpecialCharactersInID**: ID에 특수 문자(_,-,숫자) 포함
- ✅ **LongStreamName**: 긴 스트림 이름 처리
- ✅ **RTSPURLWithAuth**: RTSP URL에 인증 정보 포함
- ✅ **RTSPURLWithSpecialCharsInPassword**: 비밀번호에 특수문자 포함 (URL 인코딩)

**검증 항목**:
- 다양한 입력 형식 처리
- URL 인코딩 처리 (@, %, !, # 등)

### 6. StreamManager Integration (3개 테스트)
- ✅ **CreateAddsToStreamManager**: Create 시 StreamManager에 Stream 객체 생성
- ✅ **DeleteRemovesFromStreamManager**: Delete 시 StreamManager에서 제거
- ✅ **RuntimeInfoInList**: List 응답에 RuntimeInfo 포함

**검증 항목**:
- Database와 StreamManager 동기화
- RuntimeInfo의 정확성
- subscriber_count, packet statistics 등

### 7. mediaMTX Compatibility (2개 테스트)
- ✅ **PathsListFormat**: /v3/config/paths/list 응답 형식 검증
- ✅ **PathsListContainsStreams**: 생성한 스트림이 paths 목록에 포함됨

**검증 항목**:
- mediaMTX 호환 API 형식
- items, itemCount, pageCount 필드
- name 필드가 stream.ID를 사용 (mediaMTX 호환성)

### 8. Additional Tests
- ✅ **TestHealthCheck**: /health 엔드포인트 검증
- ✅ **TestStatsEndpoint**: /api/v1/stats 엔드포인트 검증

## 테스트 결과

### 전체 테스트 통과율
```
총 테스트: 31개
통과: 31개
실패: 0개
성공률: 100%
```

### 테스트 실행 시간
```
TestCRUDOperations:              0.43s - 0.55s
  - Create:                      0.02s - 0.03s
  - Read:                        0.01s
  - Update:                      0.02s - 0.04s
  - Delete:                      0.03s - 0.05s
  - EdgeCases:                   0.02s
  - StreamManagerIntegration:    0.01s - 0.03s
  - MediaMTXCompatibility:       0.01s

TestHealthCheck:                 0.00s - 0.01s
TestStatsEndpoint:               0.00s - 0.02s

전체 소요 시간: 0.52s - 0.66s
```

### 벤치마크 결과
```
BenchmarkCreateStream-12: 5,745,430 ns/op (~5.7ms per stream creation)
```

**해석**:
- 스트림 하나 생성에 약 5.7ms 소요
- Database 저장 + StreamManager 생성 + HTTP 왕복 시간 포함
- 1초에 약 174개 스트림 생성 가능 (이론적 최대치)

## 테스트 시나리오별 상세 결과

### Scenario 1: 기본 CRUD 흐름
```
1. POST /api/v1/streams (Create) ✅
2. GET /api/v1/streams/:id (Read) ✅
3. PUT /api/v1/streams/:id (Update) ✅
4. DELETE /api/v1/streams/:id (Delete) ✅
5. GET /api/v1/streams/:id (404 확인) ✅
```

### Scenario 2: StreamManager 통합
```
1. POST /api/v1/streams ✅
2. GET /api/v1/streams/:id (RuntimeInfo 확인) ✅
3. DELETE /api/v1/streams/:id ✅
4. StreamManager에서 제거 확인 ✅
```

### Scenario 3: mediaMTX 호환성
```
1. POST /api/v1/streams (여러 스트림 생성) ✅
2. GET /v3/config/paths/list ✅
3. 응답 형식 검증 (items, itemCount, pageCount) ✅
4. name 필드가 stream.ID 사용 확인 ✅
```

### Scenario 4: 에러 처리
```
1. 중복 ID 생성 → 500 에러 (UNIQUE constraint) ✅
2. 존재하지 않는 스트림 조회 → 404 에러 ✅
3. 존재하지 않는 스트림 업데이트 → 500 에러 ✅
4. 존재하지 않는 스트림 삭제 → 500 에러 ✅
5. 잘못된 JSON → 400 에러 ✅
```

## 아키텍처 검증

### Database as Single Source of Truth ✅
- ✅ 모든 Create 작업이 Database에 먼저 저장됨
- ✅ 모든 Read 작업이 Database에서 조회함
- ✅ 모든 Update 작업이 Database를 업데이트함
- ✅ 모든 Delete 작업이 Database에서 삭제함

### StreamManager Integration ✅
- ✅ Create 시 StreamManager.CreateStream() 호출
- ✅ Delete 시 StreamManager.RemoveStream() 호출
- ✅ RuntimeInfo가 모든 GET 응답에 포함
- ✅ RuntimeInfo는 StreamManager에서만 제공 (Database에는 저장 안 됨)

### RTSP Lifecycle Management ✅
- ✅ Update (Source 변경) 시 RTSP 클라이언트 재시작
- ✅ Delete 시 RTSP 클라이언트 정지
- ✅ sourceOnDemand=false인 스트림은 서버 시작 시 자동 시작

## 테스트 커버리지 분석

### API Endpoints
- ✅ POST /api/v1/streams
- ✅ GET /api/v1/streams
- ✅ GET /api/v1/streams/:id
- ✅ PUT /api/v1/streams/:id
- ✅ DELETE /api/v1/streams/:id
- ✅ GET /v3/config/paths/list
- ✅ GET /health
- ✅ GET /api/v1/stats

### HTTP Status Codes
- ✅ 200 OK (Read, Update, Delete 성공)
- ✅ 201 Created (Create 성공)
- ✅ 400 Bad Request (잘못된 JSON)
- ✅ 404 Not Found (존재하지 않는 리소스)
- ✅ 500 Internal Server Error (Database 에러, UNIQUE constraint 등)

### Database Operations
- ✅ INSERT (Create)
- ✅ SELECT (Read, List)
- ✅ UPDATE (Update)
- ✅ DELETE (Delete)
- ✅ UNIQUE constraint 검증

### StreamManager Operations
- ✅ CreateStream()
- ✅ GetStream()
- ✅ RemoveStream()
- ✅ GetStats() (RuntimeInfo)

## 알려진 제약사항

1. **서버 실행 필요**: 테스트 실행 전 서버가 반드시 실행 중이어야 함
2. **테스트 격리**: 테스트 간 격리를 위해 "test-" 접두사 사용
3. **정리 로직**: 테스트 전후로 자동 정리 (cleanupTestStreams)
4. **비동기 작업**: RTSP 클라이언트 시작/정지는 비동기로 처리되므로 즉시 확인 불가

## 스트림 생명주기 테스트 (Stream Lifecycle Tests)

### 개요

`stream_lifecycle_test.go`는 **실제 RTSP 연결을 기반**으로 스트림 생명주기 전체를 테스트합니다.

### 실행 방법

```bash
# 모든 생명주기 테스트 실행
go test -v ./test/integration/stream_lifecycle_test.go ./test/integration/crud_test.go -run TestStreamLifecycle

# 특정 테스트만 실행
go test -v ./test/integration/ -run TestStreamLifecycle/OnDemandStreamManagement
go test -v ./test/integration/ -run TestStreamLifecycle/MultipleUsersOneCamera

# RTSP 클라이언트 검증 테스트
go test -v ./test/integration/ -run TestRTSPClientVerification

# 부하 테스트 (5개 스트림 동시)
go test -v ./test/integration/ -run TestStressTest
```

### 테스트 커버리지

#### 1. 온디맨드 스트림 관리 (`testOnDemandStreamManagement`)

**목표**: 첫 사용자 접속 시 RTSP 시작, 마지막 사용자 종료 시 RTSP 정지

**검증 항목**:
- ✅ 초기 상태: RTSP 클라이언트 없음 (packets_received = 0)
- ✅ 첫 사용자 접속: RTSP 클라이언트 시작
- ✅ RTSP 연결 확인: is_active = true, packets_received > 0
- ✅ 온디맨드 스트림 생명주기 완료

#### 2. 여러 사용자 - 하나의 카메라 (`testMultipleUsersOneCamera`)

**목표**: 여러 사용자가 하나의 카메라 접속 시 RTSP 연결은 1개만 유지

**검증 항목**:
- ✅ RTSP 클라이언트 1개로 모든 사용자에게 스트림 제공
- ✅ 5명의 가상 사용자 동시 접속
- ✅ 패킷 수신 계속 증가 (RTSP 연결 유지 확인)
- ✅ 구독자 수 추적

**핵심**: 카메라 부하 방지 - RTSP 연결은 항상 1개!

#### 3. 사용자 연결 추적 (`testUserConnectionTracking`)

**목표**: 사용자 연결 상태를 정확히 추적

**검증 항목**:
- ✅ 초기 구독자 수: 0
- ✅ 스트림 목록에서도 상태 확인
- ✅ RuntimeInfo 정확성

#### 4. 동시 다중 스트림 (`testConcurrentMultipleStreams`)

**목표**: 여러 스트림 동시 생성 및 관리

**검증 항목**:
- ✅ 3개 스트림 동시 생성 (goroutine)
- ✅ 모든 스트림 RTSP 활성화
- ✅ 각 스트림 독립적으로 패킷 수신
- ✅ 리소스 정리 확인

#### 5. 빠른 연결/해제 반복 (`testRapidConnectDisconnect`)

**목표**: 리소스 누수 없이 빠른 연결/해제 처리

**검증 항목**:
- ✅ 5회 반복: 생성 → 연결 → 삭제
- ✅ 각 사이클마다 리소스 정리 확인
- ✅ 메모리 누수 없음

#### 6. 스트림 중단 복구 (`testStreamInterruptionRecovery`)

**목표**: 스트림 중단 후 재연결

**검증 항목**:
- ✅ 초기 RTSP 연결 및 패킷 수신
- ✅ UPDATE를 통한 RTSP 재시작
- ✅ 재연결 후 정상 동작

#### 7. 리소스 정리 (`testResourceCleanupOnDisconnect`)

**목표**: DELETE 시 모든 리소스 정리

**검증 항목**:
- ✅ RTSP 클라이언트 정지
- ✅ HLS muxer 제거
- ✅ StreamManager에서 제거
- ✅ Database에서 삭제
- ✅ 목록에서 제거

#### 8. 첫 프레임 시간 (TTFF) (`testFirstFrameTime`)

**목표**: Time to First Frame 측정

**검증 항목**:
- ✅ 온디맨드 스트림 시작부터 첫 패킷까지 시간 측정
- ✅ 10초 이내 첫 프레임 수신
- ✅ 성능 등급: 우수 (< 5초), 양호 (< 10초)

#### 9. RTSP 클라이언트 검증 (`TestRTSPClientVerification`)

**목표**: RTSP 클라이언트가 실제로 생성되고 작동하는지 검증

**검증 항목**:
- ✅ RuntimeInfo 존재
- ✅ is_active = true
- ✅ 코덱 자동 감지
- ✅ packets_received > 0
- ✅ bytes_received > 0

**이 테스트가 가장 중요!** - 모든 테스트의 기반

#### 10. 부하 테스트 (`TestStressTest`)

**목표**: 다중 스트림 동시 관리 부하 테스트

**검증 항목**:
- ✅ 5개 스트림 동시 생성
- ✅ 모든 스트림 RTSP 활성화
- ✅ 총 수신 패킷 수 집계
- ✅ 리소스 정리

**참고**: `-short` 플래그 없이 실행해야 함

### 테스트 결과 예시

```
=== RUN   TestStreamLifecycle
=== RUN   TestStreamLifecycle/OnDemandStreamManagement
    stream_lifecycle_test.go:47: ✅ 초기 상태: RTSP 클라이언트 없음 확인
    stream_lifecycle_test.go:55: ✅ 첫 사용자 접속: RTSP 클라이언트 시작 요청
    stream_lifecycle_test.go:69: ✅ RTSP 클라이언트 활성화: 234 packets received
    stream_lifecycle_test.go:74: ✅ 온디맨드 스트림 생명주기 테스트 완료
=== RUN   TestStreamLifecycle/MultipleUsersOneCamera
    stream_lifecycle_test.go:102: ✅ 초기 RTSP 클라이언트 활성화: 156 packets
    stream_lifecycle_test.go:133: ✅ 여러 사용자 접속 후에도 RTSP 클라이언트 1개 유지: 892 packets (초기: 156)
=== RUN   TestStreamLifecycle/FirstFrameTime
    stream_lifecycle_test.go:345: ✅ Time to First Frame (TTFF): 2.8s
    stream_lifecycle_test.go:349:    → 우수 (< 5초)
--- PASS: TestStreamLifecycle (45.23s)
    --- PASS: TestStreamLifecycle/OnDemandStreamManagement (8.12s)
    --- PASS: TestStreamLifecycle/MultipleUsersOneCamera (6.45s)
    --- PASS: TestStreamLifecycle/FirstFrameTime (5.67s)

=== RUN   TestRTSPClientVerification
    stream_lifecycle_test.go:379: 감지된 코덱: H265
    stream_lifecycle_test.go:393: ✅ RTSP 클라이언트 검증 완료:
    stream_lifecycle_test.go:394:    - 활성화: true
    stream_lifecycle_test.go:395:    - 코덱: H265
    stream_lifecycle_test.go:396:    - 수신 패킷: 423
    stream_lifecycle_test.go:397:    - 수신 바이트: 1234567
--- PASS: TestRTSPClientVerification (5.34s)

=== RUN   TestStressTest
    stream_lifecycle_test.go:424: ✅ 5개 스트림 생성 완료
    stream_lifecycle_test.go:451: ✅ 부하 테스트 완료:
    stream_lifecycle_test.go:452:    - 활성 스트림: 5/5
    stream_lifecycle_test.go:453:    - 총 수신 패킷: 2156
--- PASS: TestStressTest (12.45s)

PASS
```

### 실제 RTSP 연결 검증 방법

모든 테스트는 다음을 확인하여 **실제 RTSP 연결**을 검증합니다:

1. **is_active 확인**: `RuntimeInfo["is_active"] == true`
2. **패킷 수신 확인**: `RuntimeInfo["packets_received"] > 0`
3. **바이트 수신 확인**: `RuntimeInfo["bytes_received"] > 0`
4. **코덱 감지 확인**: `RuntimeInfo["codec"]` 값 존재

**패킷이 0개이면 테스트 실패** - RTSP 클라이언트가 실제로 작동하지 않는 것!

### 향후 개선 사항

1. ✅ **RTSP 연결 테스트**: 실제 RTSP 소스에 연결하여 스트리밍 테스트 - **완료!**
2. ✅ **동시성 테스트**: 다중 클라이언트 동시 Create/Update/Delete 테스트 - **완료!**
3. ✅ **부하 테스트**: 다중 스트림 생성 및 관리 테스트 - **완료!**
4. ✅ **성능 테스트**: TTFF (Time to First Frame) 측정 - **완료!**
5. **WebRTC 연결 테스트**: 실제 WebRTC 피어 연결 및 스트리밍 테스트
6. **장애 복구 테스트**: Database 연결 끊김, RTSP 연결 실패 등

## 문제 해결

### 테스트 실패 시
1. 서버가 실행 중인지 확인: `curl http://localhost:8107/health`
2. Database 파일 확인: `data/streams.db` 존재 여부
3. 로그 확인: `logs/media-server-YYYY-MM-DD.log`
4. 테스트 정리: `cleanupTestStreams()` 수동 실행

### 포트 충돌 시
```bash
# Windows
netstat -ano | findstr :8107
taskkill /PID <pid> /F

# Linux/Mac
lsof -i :8107
kill -9 <pid>
```

## 결론

**모든 CRUD 작업과 경우의 수가 정상적으로 동작합니다!** ✅

- Database-centric 아키텍처가 올바르게 구현됨
- StreamManager 통합이 정상 작동함
- mediaMTX 호환성 유지됨
- 에러 처리가 적절함
- 성능이 우수함 (5.7ms per stream creation)

이 테스트 스위트는 프로덕션 배포 전 회귀 테스트로 사용할 수 있으며, CI/CD 파이프라인에 통합 가능합니다.
