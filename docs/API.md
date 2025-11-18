# RTSP to WebRTC 미디어 서버 API 문서

> SQLite 데이터베이스 + config.yaml 기반 스트림 관리 및 WebRTC 스트리밍 시스템

## 📋 목차

1. [개요](#개요)
2. [시스템 아키텍처](#시스템-아키텍처)
3. [스트림 관리 API](#스트림-관리-api)
4. [API 엔드포인트](#api-엔드포인트)
5. [WebSocket 시그널링](#websocket-시그널링)
6. [사용 예시](#사용-예시)

---

## 개요

### 시스템 특징

- ✅ **Dual Source Loading**: config.yaml + SQLite Database 통합 관리
- ✅ **CRUD API**: 스트림 생성/조회/수정/삭제 REST API
- ✅ **Runtime Info**: 실시간 코덱, 구독자 수, 패킷 통계 제공
- ✅ **WebRTC 스트리밍**: RTSP를 WebRTC로 변환하여 브라우저에서 재생
- ✅ **온디맨드 스트리밍**: 필요한 카메라만 RTSP 연결
- ✅ **mediaMTX 호환**: mediaMTX 스타일 API 제공

### 기술 스택

- **언어**: Go 1.23+
- **프로토콜**: RTSP, WebRTC, WebSocket
- **데이터베이스**: SQLite (`modernc.org/sqlite`)
- **프레임워크**: Gin (HTTP), Gorilla WebSocket

---

## 시스템 아키텍처

```
[config.yaml] ──┐
                 ├─→ [Stream Loader]
[SQLite DB] ─────┘        ↓
                   [Stream Manager] ← Dual Source
                          ↓
                   [RTSP Client] ← 온디맨드 연결
                          ↓ RTP Packets
                   [WebRTC Peer]
                          ↓ WebSocket Signaling
                   [웹 브라우저] ← 실시간 영상 재생
```

### 스트림 소스

1. **config.yaml** - 정적 스트림 설정
   - 서버 시작 시 자동 로드
   - `source_type: "config"`
   - 수정 시 서버 재시작 필요

2. **SQLite Database** - 동적 스트림 관리
   - CRUD API를 통해 실시간 추가/수정/삭제
   - `source_type: "database"`
   - 서버 재시작 없이 관리 가능

### 주요 컴포넌트

1. **Stream Repository** (`internal/database/stream_repository.go`)
   - SQLite 기반 CRUD 작업
   - 스트림 메타데이터 영구 저장

2. **API Server** (`internal/api/server.go`)
   - REST API 제공 (CRUD)
   - WebSocket 시그널링
   - Dual Source 통합 조회

3. **Stream Manager** (`internal/core/stream_manager.go`)
   - 스트림 생명주기 관리
   - Pub/Sub 패턴 구현
   - 다중 구독자 지원
   - Runtime 정보 제공

4. **WebRTC Manager** (`internal/webrtc/manager.go`)
   - WebRTC 피어 관리
   - 동적 코덱 선택 (H.264/H.265)
   - ICE 연결 처리

---

## 스트림 관리 API

### 📌 스트림 CRUD

#### 1. 스트림 생성 (Create)

**POST** `/api/v1/streams`

새로운 스트림을 데이터베이스에 추가합니다.

**Request Body:**
```json
{
  "id": "my-camera-1",
  "name": "My Camera 1",
  "source": "rtsp://user:pass@192.168.1.100:554/stream",
  "source_on_demand": true,
  "rtsp_transport": "tcp"
}
```

**Response:**
```json
{
  "id": "my-camera-1",
  "name": "My Camera 1",
  "source": "rtsp://user:pass@192.168.1.100:554/stream",
  "source_on_demand": true,
  "rtsp_transport": "tcp",
  "created_at": "2025-11-18T10:30:00+09:00",
  "updated_at": "2025-11-18T10:30:00+09:00"
}
```

#### 2. 스트림 목록 조회 (List)

**GET** `/api/v1/streams`

모든 스트림 목록을 조회합니다 (config.yaml + database 통합).

**Response:**
```json
{
  "count": 4,
  "streams": [
    {
      "id": "CCTV-TEST1",
      "name": "CCTV-TEST1",
      "source": "runtime (config.yaml)",
      "source_on_demand": true,
      "rtsp_transport": "tcp",
      "source_type": "config",
      "runtime_info": {
        "is_active": true,
        "codec": "H265",
        "subscriber_count": 2,
        "packets_received": 12345,
        "packets_sent": 12340,
        "bytes_received": 5242880,
        "bytes_sent": 5240000
      }
    },
    {
      "id": "my-camera-1",
      "name": "My Camera 1",
      "source": "rtsp://user:pass@192.168.1.100:554/stream",
      "source_on_demand": true,
      "rtsp_transport": "tcp",
      "source_type": "database",
      "created_at": "2025-11-18T10:30:00+09:00",
      "updated_at": "2025-11-18T10:30:00+09:00",
      "runtime_info": {
        "is_active": false,
        "codec": "",
        "subscriber_count": 0,
        "packets_received": 0,
        "packets_sent": 0,
        "bytes_received": 0,
        "bytes_sent": 0
      }
    }
  ]
}
```

**Response Fields:**
- `source_type`: `"config"` (config.yaml) 또는 `"database"` (SQLite)
- `runtime_info`: 실행 중인 스트림의 실시간 정보
  - `is_active`: 스트림 활성화 여부
  - `codec`: 비디오 코덱 (H264/H265)
  - `subscriber_count`: 현재 시청자 수
  - `packets_received/sent`: RTP 패킷 통계
  - `bytes_received/sent`: 데이터 전송량

#### 3. 단일 스트림 조회 (Get)

**GET** `/api/v1/streams/:id`

특정 스트림의 상세 정보를 조회합니다.

**Response (Database 스트림):**
```json
{
  "id": "my-camera-1",
  "name": "My Camera 1",
  "source": "rtsp://user:pass@192.168.1.100:554/stream",
  "source_on_demand": true,
  "rtsp_transport": "tcp",
  "source_type": "database",
  "created_at": "2025-11-18T10:30:00+09:00",
  "updated_at": "2025-11-18T10:30:00+09:00",
  "runtime_info": {
    "is_active": true,
    "codec": "H264",
    "subscriber_count": 1,
    "packets_received": 5678,
    "packets_sent": 5670,
    "bytes_received": 2097152,
    "bytes_sent": 2095000
  }
}
```

**Response (Config 스트림):**
```json
{
  "id": "CCTV-TEST1",
  "name": "CCTV-TEST1",
  "source": "runtime (config.yaml)",
  "source_on_demand": true,
  "rtsp_transport": "tcp",
  "source_type": "config",
  "runtime_info": {
    "is_active": true,
    "codec": "H265",
    "subscriber_count": 2,
    "packets_received": 12345,
    "packets_sent": 12340,
    "bytes_received": 5242880,
    "bytes_sent": 5240000
  }
}
```

#### 4. 스트림 수정 (Update)

**PUT** `/api/v1/streams/:id`

데이터베이스 스트림 정보를 수정합니다 (config.yaml 스트림은 수정 불가).

**Request Body:**
```json
{
  "name": "Updated Camera Name",
  "source": "rtsp://user:newpass@192.168.1.100:554/stream",
  "source_on_demand": false,
  "rtsp_transport": "tcp"
}
```

**Response:**
```json
{
  "id": "my-camera-1",
  "name": "Updated Camera Name",
  "source": "rtsp://user:newpass@192.168.1.100:554/stream",
  "source_on_demand": false,
  "rtsp_transport": "tcp",
  "created_at": "2025-11-18T10:30:00+09:00",
  "updated_at": "2025-11-18T11:00:00+09:00"
}
```

#### 5. 스트림 삭제 (Delete)

**DELETE** `/api/v1/streams/:id`

데이터베이스 스트림을 삭제합니다 (config.yaml 스트림은 삭제 불가).

**Response:**
```json
{
  "status": "success",
  "id": "my-camera-1",
  "message": "Stream deleted successfully"
}
```

#### 6. 온디맨드 스트림 시작

**POST** `/api/v1/streams/:id/start`

온디맨드 스트림을 시작합니다.

**Response:**
```json
{
  "status": "success",
  "stream_id": "my-camera-1",
  "message": "Stream started successfully"
}
```

---

## API 엔드포인트

### 1. 헬스 체크

**GET** `/health`
**GET** `/api/v1/health`

서버 상태 확인

**Response:**
```json
{
  "status": "ok",
  "version": "0.2.0",
  "streams": 4,
  "clients": 2,
  "peers": 3
}
```

---

### 2. 서버 통계

**GET** `/api/v1/stats`

서버 통계 정보 조회

**Response:**
```json
{
  "uptime": "2h 30m 15s",
  "streams": 4,
  "clients": 2,
  "peers": 3,
  "api_enabled": true,
  "cctvs": 4,
  "cctv_list": [
    {
      "id": "plx_cctv_01",
      "name": "plx_cctv_01",
      "sourceOnDemand": true,
      "status": "running",
      "codec": "H265",
      "subscribers": 2
    },
    {
      "id": "plx_cctv_02",
      "name": "plx_cctv_02",
      "sourceOnDemand": true,
      "status": "stopped",
      "codec": null,
      "subscribers": 0
    }
  ]
}
```

---

### 3. CCTV 목록 동기화 (수동)

**POST** `/api/v1/sync`

외부 AIOT API에서 CCTV 목록을 수동으로 동기화합니다.

**주의:**
- 주기적 자동 동기화는 비활성화되어 있습니다
- 필요시 이 엔드포인트를 호출하여 수동 동기화
- 동기화 과정: 인증 → CCTV Sync → CCTV 목록 조회

**Response (Success):**
```json
{
  "status": "success",
  "message": "CCTV list synchronized successfully",
  "count": 4
}
```

**Response (Error):**
```json
{
  "error": "Sync failed: authentication failed: ..."
}
```

**상태 코드:**
- `200 OK`: 동기화 성공
- `500 Internal Server Error`: 동기화 실패
- `503 Service Unavailable`: CCTV Manager가 비활성화됨

---

### 4. CCTV Paths 목록 (mediaMTX 스타일)

**GET** `/v3/config/paths/list`

현재 사용 가능한 CCTV 스트림 목록을 mediaMTX 형식으로 반환합니다.

**Response:**
```json
{
  "pageCount": 1,
  "itemCount": 4,
  "items": [
    {
      "name": "plx_cctv_01",
      "source": "rtsp://admin:***@192.168.4.121:554/Streaming/Channels/101"
    },
    {
      "name": "plx_cctv_02",
      "source": "rtsp://admin:***@192.168.4.54:554/profile2/media.smp"
    },
    {
      "name": "plx_cctv_03",
      "source": "rtsp://admin:***@192.168.4.46:554/profile2/media.smp"
    },
    {
      "name": "park_cctv_01",
      "source": "rtsp://***@121.190.36.211:554/..."
    }
  ]
}
```

**주의:**
- `source` 필드의 비밀번호는 마스킹되어 표시됩니다
- 실제 RTSP 연결은 마스킹되지 않은 원본 URL 사용
- 프론트엔드에서 스트림 선택 시 `name` 필드 사용

---

### 5. Path 추가 (현재 비지원)

**POST** `/v3/config/paths/add/:name`

**Response:**
```json
{
  "error": "Path addition not supported in API-based mode"
}
```

**상태 코드:** `501 Not Implemented`

**이유:** 외부 AIOT API 기반으로 CCTV를 관리하므로, 서버에서 직접 추가 불가능합니다.

---

### 6. Path 삭제 (현재 비지원)

**DELETE** `/v3/config/paths/delete/:name`

**Response:**
```json
{
  "error": "Path deletion not supported in API-based mode"
}
```

**상태 코드:** `501 Not Implemented`

**이유:** 외부 AIOT API 기반으로 CCTV를 관리하므로, 서버에서 직접 삭제 불가능합니다.

---

## WebSocket 시그널링

### 연결

**WebSocket** `ws://localhost:8080/ws`

WebRTC 시그널링을 위한 WebSocket 연결입니다.

### 메시지 형식

모든 메시지는 JSON 형식입니다:
```json
{
  "type": "offer|answer|ice",
  "payload": { ... }
}
```

### Offer (클라이언트 → 서버)

```json
{
  "type": "offer",
  "payload": {
    "sdp": "v=0\r\no=- ...",
    "streamId": "plx_cctv_01"
  }
}
```

**처리 과정:**
1. 서버가 `streamId`에 해당하는 CCTV 스트림 확인
2. 스트림이 stopped 상태면 온디맨드로 RTSP 연결 시작
3. WebRTC 피어 생성 및 스트림 구독
4. Answer SDP 생성 및 반환

### Answer (서버 → 클라이언트)

```json
{
  "type": "answer",
  "payload": "v=0\r\no=- ..."
}
```

### ICE Candidate (양방향)

```json
{
  "type": "ice",
  "payload": {
    "candidate": "candidate:...",
    "sdpMid": "0",
    "sdpMLineIndex": 0
  }
}
```

### Error (서버 → 클라이언트)

```json
{
  "type": "error",
  "payload": "stream not found"
}
```

---

## 외부 AIOT API 연동

### 설정

`configs/config.yaml`:
```yaml
api:
  enabled: true
  base_url: "https://aiot.pluxity.com/api"
  username: "your-username"
  password: "your-password"
  request_timeout_sec: 30  # API 요청 타임아웃
  on_demand_wait_sec: 2    # 온디맨드 스트림 시작 대기 시간
```

### AIOT API 엔드포인트

#### 1. 인증 (Sign-In)

**POST** `{base_url}/auth/sign-in`

```json
{
  "username": "your-username",
  "password": "your-password"
}
```

**Response:**
```json
{
  "accessToken": "eyJ...",
  "refreshToken": "eyJ..."
}
```

또는 쿠키 기반 인증도 지원합니다.

---

#### 2. CCTV 동기화

**GET** `{base_url}/cctvs/sync`

**Headers:**
- `Authorization: Bearer {accessToken}` (옵션, 쿠키 사용 시 불필요)

AIOT 시스템의 CCTV 목록을 동기화합니다.

**Response:**
```json
{
  "status": "success",
  "message": "Sync completed"
}
```

---

#### 3. CCTV 목록 조회

**GET** `{base_url}/cctvs`

**Headers:**
- `Authorization: Bearer {accessToken}` (옵션, 쿠키 사용 시 불필요)

**Response (Array):**
```json
[
  {
    "name": "plx_cctv_01",
    "url": "rtsp://admin:password@192.168.4.121:554/Streaming/Channels/101"
  },
  {
    "name": "plx_cctv_02",
    "url": "rtsp://admin:password@192.168.4.54:554/profile2/media.smp"
  }
]
```

또는 **Response (Object):**
```json
{
  "data": [
    { "name": "...", "url": "..." },
    { "name": "...", "url": "..." }
  ]
}
```

클라이언트는 두 형식 모두 지원합니다.

---

## 사용 예시

### 1. 서버 시작 후 CCTV 동기화

```bash
# 서버 시작
./bin/media-server.exe

# 수동 동기화 (필요시)
curl -X POST http://localhost:8080/api/v1/sync
```

서버 시작 시 자동으로 초기 동기화가 수행됩니다. 이후 CCTV 목록이 변경되면 `/api/v1/sync`를 호출하여 수동으로 동기화합니다.

---

### 2. 웹 클라이언트에서 CCTV 목록 가져오기

```javascript
// CCTV 목록 조회
const response = await fetch('/v3/config/paths/list');
const data = await response.json();
const streams = data.items; // [{name: "plx_cctv_01", source: "rtsp://..."}, ...]

console.log(`총 ${streams.length}개의 CCTV 스트림 사용 가능`);
```

---

### 3. WebRTC 연결

```javascript
const engine = new WebRTCEngine({
    streamId: 'plx_cctv_01',
    videoElement: document.getElementById('video'),
    autoReconnect: true
});

engine.on('connected', () => {
    console.log('WebRTC 연결 성공');
});

engine.on('error', (error) => {
    console.error('WebRTC 오류:', error);
});

await engine.connect();
```

---

### 4. 대시보드에서 모든 CCTV 표시

```javascript
// 1. CCTV 목록 로드
const response = await fetch('/v3/config/paths/list');
const data = await response.json();
const streams = data.items;

// 2. 각 CCTV에 대해 WebRTC 엔진 생성
const engines = {};
for (const stream of streams) {
    engines[stream.name] = new WebRTCEngine({
        streamId: stream.name,
        videoElement: document.getElementById(`video-${stream.name}`)
    });
    await engines[stream.name].connect();
}

// 3. 자동으로 모든 CCTV 연결됨
```

---

## 주요 변경사항 (v0.2.0)

### 이전 버전 (v0.1.0)과의 차이점

| 기능 | v0.1.0 (로컬 설정) | v0.2.0 (AIOT API) |
|------|-------------------|------------------|
| CCTV 설정 | `config.yaml` paths 섹션 | 외부 AIOT API |
| CCTV 추가/삭제 | 설정 파일 수정 → 재시작 | API 호출 → 동기화 |
| 동기화 | 주기적 자동 동기화 (5분) | 수동 동기화 (`POST /api/v1/sync`) |
| 스트림 시작 | `POST /api/v1/streams/:id/start` | 온디맨드 자동 시작 |
| API 엔드포인트 | `/api/v1/streams` | `/v3/config/paths/list` |

### 마이그레이션 가이드

**v0.1.0에서 v0.2.0으로 업데이트 시:**

1. **설정 파일 업데이트**
   ```yaml
   # config.yaml에 API 섹션 추가
   api:
     enabled: true
     base_url: "https://aiot.pluxity.com/api"
     username: "your-username"
     password: "your-password"

   # paths 섹션 제거 (AIOT API에서 자동으로 가져옴)
   ```

2. **프론트엔드 코드 업데이트**
   ```javascript
   // 이전
   const response = await fetch('/api/v1/streams');
   const data = await response.json();
   const streams = data.streams;

   // 현재
   const response = await fetch('/v3/config/paths/list');
   const data = await response.json();
   const streams = data.items;
   ```

3. **수동 동기화 추가**
   - 주기적 자동 동기화가 비활성화되었습니다
   - 필요시 `POST /api/v1/sync` 호출

---

## 보안

### URL 마스킹

API 응답의 `source` 필드는 비밀번호가 마스킹됩니다:
```
원본: rtsp://admin:password123@192.168.1.100:554/stream
마스킹: rtsp://admin:***@192.168.1.100:554/stream
```

### 인증 정보 보호

- AIOT API 인증 정보는 `config.yaml`에 평문으로 저장됩니다
- 프로덕션 환경에서는 환경 변수 또는 암호화된 설정 사용 권장
- `config.yaml` 파일은 `.gitignore`에 추가하여 버전 관리에서 제외

### HTTPS/WSS

- 로컬 개발 환경에서는 HTTP/WS 사용
- 프로덕션 환경에서는 HTTPS/WSS 설정 필요
- Nginx 또는 Caddy를 리버스 프록시로 사용 권장

---

## 문제 해결

### CCTV 목록이 비어 있음

**증상:** `/v3/config/paths/list`가 빈 배열 반환

**해결:**
1. AIOT API 인증 확인: `config.yaml`의 username/password
2. 수동 동기화 실행: `POST /api/v1/sync`
3. 서버 로그 확인: `authentication failed`, `sync failed` 등

### 동기화 실패

**증상:** `POST /api/v1/sync`가 500 에러 반환

**해결:**
1. AIOT API 서버 상태 확인: `https://aiot.pluxity.com/api` 접속 가능 여부
2. 인증 정보 확인: username/password 정확성
3. 네트워크 연결 확인: 방화벽, 프록시 설정

### WebRTC 연결 실패

**증상:** 브라우저에서 ICE connection state: failed

**해결:**
1. RTSP 스트림 확인: 온디맨드 스트림이 자동 시작되었는지 확인
2. 서버 로그 확인: RTSP client connected 메시지 확인
3. 브라우저 콘솔 확인: ICE candidate, SDP 교환 로그

---

## 라이센스

MIT License

---

**버전:** v0.2.0
**마지막 업데이트:** 2025-11-17
