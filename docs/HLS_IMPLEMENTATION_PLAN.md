# HLS (HTTP Live Streaming) 구현 계획

> **작성일**: 2025-11-17
> **상태**: 계획 단계
> **목표**: 기존 WebRTC 스트리밍에 HLS 출력 추가

---

## 📋 목차
1. [개요](#개요)
2. [현재 아키텍처 분석](#현재-아키텍처-분석)
3. [HLS 아키텍처 설계](#hls-아키텍처-설계)
4. [구현 옵션 비교](#구현-옵션-비교)
5. [상세 구현 계획](#상세-구현-계획)
6. [API 설계](#api-설계)
7. [파일 시스템 구조](#파일-시스템-구조)
8. [성능 고려사항](#성능-고려사항)
9. [테스트 계획](#테스트-계획)
10. [일정 및 마일스톤](#일정-및-마일스톤)

---

## 개요

### 요구사항
- **기존**: RTSP → WebRTC 스트리밍 (✅ 구현 완료)
- **추가**: RTSP → HLS 스트리밍 (🔶 신규 요구사항)

### 목표
1. WebRTC와 HLS를 **동시 지원**하는 멀티 프로토콜 미디어 서버
2. 사용자가 선택 가능: WebRTC (저지연) vs HLS (범용성)
3. 기존 WebRTC 기능에 영향 없이 HLS 추가

### 사용 시나리오

| 프로토콜 | 사용 사례 | 지연시간 | 브라우저 지원 |
|---------|----------|---------|-------------|
| **WebRTC** | 실시간 모니터링, 양방향 통신 | < 1초 | Chrome, Firefox, Safari |
| **HLS** | 녹화 재생, 모바일 앱, Safari 호환 | 5-15초 | 모든 브라우저 (HTML5 video) |

---

## 현재 아키텍처 분석

### 현재 데이터 플로우
```
[RTSP Camera (H.265/H.264)]
    ↓ TCP/RTSP
[RTSP Client (gortsplib v4)]
    ↓ RTP Packets (OnPacketRTPAny)
[Stream Manager (Pub/Sub)]
    ↓ Subscribe
[WebRTC Peer (pion v4)]
    ↓ WebRTC/SRTP
[Web Browser] ✅ 실시간 영상 재생
```

### 핵심 컴포넌트
- **RTSP Client**: RTP 패킷 수신
- **Stream Manager**: Pub/Sub 패턴으로 여러 구독자에게 패킷 분배
- **WebRTC Peer**: RTP → WebRTC 변환

### 활용 가능한 인프라
- ✅ RTP 패킷 수신 메커니즘 (gortsplib v4)
- ✅ Stream Manager (다중 구독자 지원)
- ✅ HTTP API Server (Gin)
- ✅ 정적 파일 서빙 인프라

---

## HLS 아키텍처 설계

### 확장된 데이터 플로우
```
[RTSP Camera (H.265/H.264)]
    ↓ TCP/RTSP
[RTSP Client (gortsplib v4)]
    ↓ RTP Packets (OnPacketRTPAny)
[Stream Manager (Pub/Sub)]
    ├─────────────────┬─────────────────┐
    ↓                 ↓                 ↓
[WebRTC Peer]    [HLS Muxer]      [Future: RTMP, etc]
    ↓                 ↓
[Browser]    [M3U8 + TS Segments]
                      ↓
             [HTTP Static File Server]
                      ↓
             [HTML5 Video Player]
```

### 신규 컴포넌트

#### 1. **HLS Muxer** (`internal/hls/muxer.go`)
- **역할**: RTP 패킷 → TS (Transport Stream) 세그먼트 변환
- **기능**:
  - RTP 패킷 디패킷화 (de-packetization)
  - H.264/H.265 NAL 유닛 추출
  - MPEG-TS 컨테이너로 먹싱
  - 세그먼트 파일 생성 (예: segment_0.ts, segment_1.ts)
- **라이브러리**:
  - `github.com/asticode/go-astits` (TS muxing)
  - 또는 FFmpeg 프로세스 호출

#### 2. **HLS Manager** (`internal/hls/manager.go`)
- **역할**: HLS 세그먼트 및 플레이리스트 관리
- **기능**:
  - M3U8 플레이리스트 생성 및 업데이트
  - 세그먼트 파일 보관 정책 (오래된 세그먼트 삭제)
  - 스트림별 HLS 세션 관리
- **라이브러리**: `github.com/grafov/m3u8`

#### 3. **HLS HTTP Handler** (`internal/api/hls_handler.go`)
- **역할**: HLS 파일 제공 (M3U8, TS)
- **엔드포인트**:
  - `GET /hls/{stream_id}/index.m3u8` - 플레이리스트
  - `GET /hls/{stream_id}/segment_{n}.ts` - 세그먼트 파일

---

## 구현 옵션 비교

### Option 1: Pure Go Implementation (권장 ⭐)

**장점**:
- 전체 시스템을 Go로 통일 (유지보수 용이)
- 도커 이미지 크기 최소화 (FFmpeg 불필요)
- 세밀한 제어 가능 (버퍼링, 세그먼트 크기 등)

**단점**:
- HLS muxing 라이브러리 성숙도 확인 필요
- 초기 개발 시간 증가

**라이브러리**:
```go
// TS Muxing
github.com/asticode/go-astits

// M3U8 Playlist
github.com/grafov/m3u8

// RTP de-packetization (이미 사용 중)
github.com/pion/rtp
```

### Option 2: FFmpeg Process

**장점**:
- 검증된 안정성 (FFmpeg는 업계 표준)
- 다양한 코덱 지원
- HLS 변환 성능 우수

**단점**:
- 외부 프로세스 관리 복잡도
- 도커 이미지 크기 증가 (이미 FFmpeg 포함됨 ✅)
- 프로세스 간 통신 오버헤드

**구현 예시**:
```bash
ffmpeg -i rtsp://camera-url \
  -c:v copy \
  -hls_time 2 \
  -hls_list_size 10 \
  -hls_flags delete_segments \
  /app/hls/{stream_id}/index.m3u8
```

### Option 3: mediaMTX HLS 기능 활용

**장점**:
- mediaMTX는 이미 HLS를 지원함
- 프로덕션에서 검증됨
- 코드 참조 가능

**단점**:
- 코드베이스 복잡도 (mediaMTX는 대규모 프로젝트)
- 라이선스 확인 필요 (MIT)
- 불필요한 기능까지 포함될 수 있음

---

## 상세 구현 계획

### Phase 1: 기본 HLS 지원 (2-3일)

#### 1.1 HLS Muxer 구현
```go
// internal/hls/muxer.go
package hls

type Muxer struct {
    streamID       string
    outputDir      string
    segmentDuration int // seconds
    currentSegment *Segment
    playlist       *m3u8.MediaPlaylist
}

func NewMuxer(streamID string, config Config) *Muxer
func (m *Muxer) Start() error
func (m *Muxer) WriteRTPPacket(pkt *rtp.Packet) error
func (m *Muxer) Stop()
```

#### 1.2 세그먼트 관리
```go
type Segment struct {
    Index      int
    Filename   string
    Duration   float64
    StartTime  time.Time
    Writer     *astits.Muxer
}
```

#### 1.3 Stream Manager 통합
```go
// internal/core/stream_manager.go에 추가
func (sm *StreamManager) AddHLSSubscriber(streamID string, muxer *hls.Muxer) error
```

### Phase 2: HTTP API 및 파일 서빙 (1일)

#### 2.1 API 엔드포인트 추가
```go
// internal/api/server.go
func (s *Server) setupHLSRoutes() {
    hls := s.router.Group("/hls")
    {
        hls.GET("/:stream_id/index.m3u8", s.handleHLSPlaylist)
        hls.GET("/:stream_id/:segment", s.handleHLSSegment)
    }
}
```

#### 2.2 CORS 설정
```go
// HLS 요청은 비디오 플레이어에서 오므로 CORS 필요
config := cors.DefaultConfig()
config.AllowOrigins = []string{"*"}
```

### Phase 3: 웹 UI 추가 (1일)

#### 3.1 HLS Engine 라이브러리 (WebRTCEngine 스타일)

**파일**: `web/static/js/hls-engine.js`

```javascript
/**
 * HLSEngine - 재사용 가능한 HLS 클라이언트 라이브러리
 * WebRTCEngine과 동일한 API 패턴 사용
 *
 * @example
 * const engine = new HLSEngine({
 *   streamId: 'CCTV-TEST',
 *   videoElement: document.getElementById('video1')
 * });
 *
 * engine.on('loaded', () => console.log('HLS loaded'));
 * engine.on('error', (err) => console.error(err));
 * engine.on('stats', (stats) => console.log(stats));
 *
 * await engine.load();
 */

class HLSEngine {
    constructor(config) {
        // 필수 파라미터 검증
        if (!config.videoElement) {
            throw new Error('videoElement is required');
        }
        if (!config.streamId) {
            throw new Error('streamId is required');
        }

        // 설정
        this.streamId = config.streamId;
        this.videoElement = config.videoElement;
        this.baseUrl = config.baseUrl || '';
        this.autoPlay = config.autoPlay !== undefined ? config.autoPlay : true;

        // HLS 설정
        this.hlsConfig = {
            enableWorker: true,
            lowLatencyMode: true,  // 저지연 모드
            backBufferLength: 90,  // 백버퍼 길이
            ...config.hlsConfig
        };

        // 상태
        this.hls = null;
        this.loaded = false;
        this.playing = false;

        // 통계
        this.stats = {
            currentLevel: 0,
            loadedFragments: 0,
            droppedFrames: 0,
            bandwidth: 0
        };
        this.statsInterval = null;

        // 이벤트 핸들러
        this.eventHandlers = {
            'loaded': [],
            'playing': [],
            'paused': [],
            'error': [],
            'stats': [],
            'levelChanged': []
        };

        // 비디오 엘리먼트 속성 설정
        this.videoElement.controls = true;
        this.videoElement.autoplay = this.autoPlay;
        this.videoElement.playsinline = true;

        this.log(`🎬 HLSEngine initialized for stream: ${this.streamId}`);
    }

    /**
     * 이벤트 리스너 등록
     */
    on(event, callback) {
        if (!this.eventHandlers[event]) {
            throw new Error(`Unknown event: ${event}`);
        }
        this.eventHandlers[event].push(callback);
        return this;
    }

    /**
     * 이벤트 발생
     */
    emit(event, data) {
        if (this.eventHandlers[event]) {
            this.eventHandlers[event].forEach(callback => {
                try {
                    callback(data);
                } catch (error) {
                    console.error('[HLSEngine] Event handler error:', error);
                }
            });
        }
    }

    /**
     * HLS 스트림 로드
     */
    async load() {
        try {
            const hlsUrl = `${this.baseUrl}/hls/${this.streamId}/index.m3u8`;
            this.log(`📡 Loading HLS stream: ${hlsUrl}`);

            // HLS.js 지원 확인
            if (Hls.isSupported()) {
                this.log('✅ HLS.js is supported');
                this.hls = new Hls(this.hlsConfig);

                // 이벤트 리스너 설정
                this.setupHlsEvents();

                // 스트림 로드
                this.hls.loadSource(hlsUrl);
                this.hls.attachMedia(this.videoElement);

                // 비디오 이벤트 리스너
                this.setupVideoEvents();

            } else if (this.videoElement.canPlayType('application/vnd.apple.mpegurl')) {
                // Safari 네이티브 HLS 지원
                this.log('✅ Native HLS is supported (Safari)');
                this.videoElement.src = hlsUrl;
                this.setupVideoEvents();
            } else {
                throw new Error('HLS is not supported in this browser');
            }

            this.loaded = true;
            this.emit('loaded', { streamId: this.streamId });

        } catch (error) {
            this.log(`❌ Failed to load HLS: ${error.message}`, 'error');
            this.emit('error', error);
            throw error;
        }
    }

    /**
     * HLS.js 이벤트 설정
     */
    setupHlsEvents() {
        if (!this.hls) return;

        // 매니페스트 로드 완료
        this.hls.on(Hls.Events.MANIFEST_PARSED, (event, data) => {
            this.log(`📋 Manifest parsed: ${data.levels.length} levels`);
            this.stats.levels = data.levels;
        });

        // 레벨 변경
        this.hls.on(Hls.Events.LEVEL_SWITCHED, (event, data) => {
            this.stats.currentLevel = data.level;
            this.emit('levelChanged', { level: data.level });
        });

        // 프래그먼트 로드 완료
        this.hls.on(Hls.Events.FRAG_LOADED, (event, data) => {
            this.stats.loadedFragments++;
        });

        // 에러 처리
        this.hls.on(Hls.Events.ERROR, (event, data) => {
            if (data.fatal) {
                this.handleFatalError(data);
            } else {
                this.log(`⚠️ Non-fatal error: ${data.type}`, 'warn');
            }
        });
    }

    /**
     * 비디오 엘리먼트 이벤트 설정
     */
    setupVideoEvents() {
        this.videoElement.addEventListener('playing', () => {
            this.playing = true;
            this.emit('playing');
            this.startStatsCollection();
        });

        this.videoElement.addEventListener('pause', () => {
            this.playing = false;
            this.emit('paused');
            this.stopStatsCollection();
        });

        this.videoElement.addEventListener('error', (e) => {
            this.log(`❌ Video error: ${e.message}`, 'error');
            this.emit('error', e);
        });
    }

    /**
     * 치명적 에러 처리
     */
    handleFatalError(data) {
        this.log(`❌ Fatal error: ${data.type} - ${data.details}`, 'error');

        switch(data.type) {
            case Hls.ErrorTypes.NETWORK_ERROR:
                this.log('🔄 Network error, attempting to recover...');
                this.hls.startLoad();
                break;
            case Hls.ErrorTypes.MEDIA_ERROR:
                this.log('🔄 Media error, attempting to recover...');
                this.hls.recoverMediaError();
                break;
            default:
                this.log('💥 Unrecoverable error');
                this.emit('error', new Error(`${data.type}: ${data.details}`));
                this.destroy();
                break;
        }
    }

    /**
     * 통계 수집 시작
     */
    startStatsCollection() {
        if (this.statsInterval) return;

        this.statsInterval = setInterval(() => {
            if (!this.videoElement) return;

            // 비디오 통계
            const videoStats = {
                currentTime: this.videoElement.currentTime,
                duration: this.videoElement.duration,
                buffered: this.getBufferedTime(),
                videoWidth: this.videoElement.videoWidth,
                videoHeight: this.videoElement.videoHeight,
                ...this.stats
            };

            // HLS.js 통계
            if (this.hls) {
                const hlsStats = this.hls.bandwidthEstimate;
                videoStats.bandwidth = Math.round(hlsStats / 1000); // kbps
            }

            this.emit('stats', videoStats);
        }, 1000);
    }

    /**
     * 통계 수집 중지
     */
    stopStatsCollection() {
        if (this.statsInterval) {
            clearInterval(this.statsInterval);
            this.statsInterval = null;
        }
    }

    /**
     * 버퍼링된 시간 계산
     */
    getBufferedTime() {
        const buffered = this.videoElement.buffered;
        if (buffered.length === 0) return 0;

        const currentTime = this.videoElement.currentTime;
        for (let i = 0; i < buffered.length; i++) {
            if (buffered.start(i) <= currentTime && currentTime <= buffered.end(i)) {
                return buffered.end(i) - currentTime;
            }
        }
        return 0;
    }

    /**
     * 재생
     */
    play() {
        return this.videoElement.play();
    }

    /**
     * 일시정지
     */
    pause() {
        this.videoElement.pause();
    }

    /**
     * 볼륨 설정
     */
    setVolume(volume) {
        this.videoElement.volume = Math.max(0, Math.min(1, volume));
    }

    /**
     * 음소거 토글
     */
    toggleMute() {
        this.videoElement.muted = !this.videoElement.muted;
        return this.videoElement.muted;
    }

    /**
     * 화질 변경
     */
    setQuality(level) {
        if (!this.hls) return;
        this.hls.currentLevel = level;
    }

    /**
     * 자동 화질 선택
     */
    setAutoQuality() {
        if (!this.hls) return;
        this.hls.currentLevel = -1; // auto
    }

    /**
     * 리소스 정리
     */
    destroy() {
        this.log('🗑️ Destroying HLSEngine...');

        this.stopStatsCollection();

        if (this.hls) {
            this.hls.destroy();
            this.hls = null;
        }

        if (this.videoElement) {
            this.videoElement.src = '';
            this.videoElement.load();
        }

        this.loaded = false;
        this.playing = false;

        this.log('✅ HLSEngine destroyed');
    }

    /**
     * 로그 출력
     */
    log(message, level = 'info') {
        const prefix = `[HLSEngine:${this.streamId}]`;
        switch(level) {
            case 'error':
                console.error(prefix, message);
                break;
            case 'warn':
                console.warn(prefix, message);
                break;
            default:
                console.log(prefix, message);
        }
    }

    /**
     * 현재 상태 반환
     */
    getState() {
        return {
            streamId: this.streamId,
            loaded: this.loaded,
            playing: this.playing,
            currentTime: this.videoElement.currentTime,
            duration: this.videoElement.duration,
            stats: this.stats
        };
    }
}
```

#### 3.2 사용 예시

**간단한 사용 (WebRTCEngine과 동일한 패턴):**
```javascript
// 1. HLS 엔진 생성
const hlsEngine = new HLSEngine({
    streamId: 'CCTV-TEST',
    videoElement: document.getElementById('video1')
});

// 2. 이벤트 리스너 등록
hlsEngine.on('loaded', () => {
    console.log('HLS stream loaded!');
});

hlsEngine.on('playing', () => {
    console.log('Video is playing');
});

hlsEngine.on('stats', (stats) => {
    console.log('Bandwidth:', stats.bandwidth, 'kbps');
    console.log('Buffer:', stats.buffered, 'seconds');
});

hlsEngine.on('error', (error) => {
    console.error('HLS error:', error);
});

// 3. 로드 시작
await hlsEngine.load();
```

**다중 스트림 (대시보드):**
```javascript
const streams = ['CCTV-TEST', 'CCTV-TEST2', 'CCTV-TEST3'];
const engines = {};

streams.forEach(streamId => {
    const videoElement = document.getElementById(`video-${streamId}`);

    engines[streamId] = new HLSEngine({
        streamId: streamId,
        videoElement: videoElement
    });

    engines[streamId]
        .on('loaded', () => updateStatus(streamId, 'loaded'))
        .on('error', (err) => updateStatus(streamId, 'error'))
        .load();
});
```

**프로토콜 전환 (WebRTC ↔ HLS):**
```javascript
let currentEngine = null;

async function switchProtocol(protocol) {
    // 기존 엔진 정리
    if (currentEngine) {
        currentEngine.destroy();
    }

    const videoElement = document.getElementById('video');
    const streamId = 'CCTV-TEST';

    if (protocol === 'webrtc') {
        currentEngine = new WebRTCEngine({
            streamId: streamId,
            videoElement: videoElement
        });
        await currentEngine.connect();
    } else {
        currentEngine = new HLSEngine({
            streamId: streamId,
            videoElement: videoElement
        });
        await currentEngine.load();
    }
}
```

#### 3.3 HLS 뷰어 페이지
```html
<!-- web/static/hls-viewer.html -->
<!DOCTYPE html>
<html>
<head>
    <title>HLS Viewer</title>
    <script src="https://cdn.jsdelivr.net/npm/hls.js@latest"></script>
    <script src="/static/js/hls-engine.js"></script>
</head>
<body>
    <h1>HLS Streaming Viewer</h1>

    <select id="streamSelect"></select>

    <video id="hlsVideo"></video>

    <div id="stats">
        <p>Bandwidth: <span id="bandwidth">-</span> kbps</p>
        <p>Buffer: <span id="buffer">-</span> seconds</p>
        <p>Quality: <span id="quality">-</span></p>
    </div>

    <script>
        let engine = null;

        async function loadStream(streamId) {
            if (engine) {
                engine.destroy();
            }

            engine = new HLSEngine({
                streamId: streamId,
                videoElement: document.getElementById('hlsVideo')
            });

            engine.on('loaded', () => {
                console.log('Stream loaded!');
            });

            engine.on('stats', (stats) => {
                document.getElementById('bandwidth').textContent = stats.bandwidth;
                document.getElementById('buffer').textContent = stats.buffered.toFixed(1);
                document.getElementById('quality').textContent =
                    `${stats.videoWidth}x${stats.videoHeight}`;
            });

            engine.on('error', (error) => {
                alert('Error: ' + error.message);
            });

            await engine.load();
        }

        // 스트림 목록 로드
        fetch('/v3/config/paths/list')
            .then(r => r.json())
            .then(data => {
                const select = document.getElementById('streamSelect');
                data.items.forEach(stream => {
                    const option = document.createElement('option');
                    option.value = stream.name;
                    option.textContent = stream.name;
                    select.appendChild(option);
                });

                if (data.items.length > 0) {
                    loadStream(data.items[0].name);
                }
            });

        document.getElementById('streamSelect').addEventListener('change', (e) => {
            loadStream(e.target.value);
        });
    </script>
</body>
</html>
```

#### 3.4 통합 뷰어 (WebRTC + HLS 선택)
```html
<!-- web/static/viewer.html 업데이트 -->
<select id="protocolSelect">
    <option value="webrtc">WebRTC (실시간, <1초)</option>
    <option value="hls">HLS (범용, ~10초)</option>
</select>

<script>
let currentEngine = null;

async function switchProtocol(protocol) {
    if (currentEngine) {
        currentEngine.destroy();
    }

    const videoElement = document.getElementById('video');
    const streamId = document.getElementById('streamSelect').value;

    if (protocol === 'webrtc') {
        currentEngine = new WebRTCEngine({
            streamId: streamId,
            videoElement: videoElement
        });
        await currentEngine.connect();
    } else {
        currentEngine = new HLSEngine({
            streamId: streamId,
            videoElement: videoElement
        });
        await currentEngine.load();
    }
}
</script>
```

### Phase 4: 설정 및 최적화 (1일)

#### 4.1 설정 추가
```yaml
# configs/config.yaml
hls:
  enabled: true
  segment_duration: 2      # 초 (짧을수록 지연시간 감소)
  segment_count: 10        # 플레이리스트에 유지할 세그먼트 수
  output_dir: "hls"        # 세그먼트 저장 디렉토리
  cleanup_threshold: 20    # 디스크에 유지할 최대 세그먼트 수
```

#### 4.2 파일 정리 로직
```go
// 오래된 세그먼트 자동 삭제
func (m *HLSManager) cleanupOldSegments(streamID string)
```

---

## API 설계

### HLS 관련 엔드포인트

#### 1. 플레이리스트 조회
```
GET /hls/{stream_id}/index.m3u8

Response: (M3U8 플레이리스트)
#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:2
#EXT-X-MEDIA-SEQUENCE:5
#EXTINF:2.000,
segment_5.ts
#EXTINF:2.000,
segment_6.ts
#EXTINF:2.000,
segment_7.ts
```

#### 2. 세그먼트 파일 다운로드
```
GET /hls/{stream_id}/segment_{n}.ts

Response: (Binary TS file)
Content-Type: video/MP2T
```

#### 3. HLS 활성화/비활성화
```
POST /api/v1/streams/{stream_id}/hls/start
DELETE /api/v1/streams/{stream_id}/hls/stop
GET /api/v1/streams/{stream_id}/hls/status
```

### 스트림 정보 API 확장
```json
// GET /api/v1/streams/{stream_id}
{
  "id": "CCTV-TEST",
  "name": "CCTV-TEST",
  "status": "running",
  "outputs": {
    "webrtc": {
      "enabled": true,
      "peers": 2
    },
    "hls": {
      "enabled": true,
      "segment_count": 10,
      "playlist_url": "/hls/CCTV-TEST/index.m3u8"
    }
  }
}
```

---

## 파일 시스템 구조

### HLS 파일 저장 구조
```
/app/hls/
├── CCTV-TEST/
│   ├── index.m3u8           # 플레이리스트
│   ├── segment_0.ts         # 세그먼트 파일
│   ├── segment_1.ts
│   ├── segment_2.ts
│   └── ...
├── CCTV-TEST2/
│   ├── index.m3u8
│   └── ...
└── CCTV-TEST3/
    └── ...
```

### 도커 볼륨 마운트
```yaml
# docker-compose.yml
volumes:
  - ./hls:/app/hls  # HLS 세그먼트 저장
  - ./log/media:/app/logs
```

---

## 성능 고려사항

### 1. 디스크 I/O
- **문제**: 세그먼트 파일을 디스크에 지속적으로 쓰기
- **해결**:
  - 메모리 버퍼 활용 (최근 세그먼트 메모리 캐싱)
  - SSD 사용 권장
  - 세그먼트 크기 최적화 (2초 권장)

### 2. 동시 스트림 수
- **목표**: 100개 스트림 동시 HLS 변환
- **리소스 예상**:
  - CPU: 각 스트림당 ~5% (인코딩 안 함, 리먹싱만)
  - 디스크: 각 스트림당 ~50MB (10개 세그먼트 × 5MB)
  - 메모리: 각 스트림당 ~10MB

### 3. 네트워크 대역폭
- **세그먼트 크기**: ~2-5MB (2초, 1080p H.264 기준)
- **동시 시청자**: HLS는 HTTP 기반이므로 캐싱 가능 (CDN 활용)

### 4. 지연시간 vs 안정성 트레이드오프

| 세그먼트 길이 | 지연시간 | 안정성 | 사용 사례 |
|-------------|---------|--------|----------|
| 1초 | 3-5초 | 낮음 | 실시간 모니터링 |
| 2초 (권장) | 6-10초 | 중간 | 일반적인 라이브 스트리밍 |
| 6초 | 18-30초 | 높음 | VOD, 네트워크 불안정 환경 |

---

## 테스트 계획

### 단위 테스트
```go
// internal/hls/muxer_test.go
func TestMuxer_WriteRTPPacket(t *testing.T)
func TestMuxer_SegmentRotation(t *testing.T)
func TestMuxer_PlaylistGeneration(t *testing.T)
```

### 통합 테스트
```go
// test/e2e/hls_test.go
func TestHLSStreaming(t *testing.T) {
    // 1. RTSP 스트림 시작
    // 2. HLS muxer 시작
    // 3. M3U8 플레이리스트 확인
    // 4. 세그먼트 파일 다운로드
    // 5. TS 파일 유효성 검증
}
```

### 브라우저 테스트

| 브라우저 | HLS.js | 네이티브 HLS | 테스트 결과 |
|---------|--------|-------------|-----------|
| Chrome | ✅ | ❌ | 🔶 예정 |
| Firefox | ✅ | ❌ | 🔶 예정 |
| Safari | ✅ | ✅ | 🔶 예정 |
| Edge | ✅ | ❌ | 🔶 예정 |

---

## 일정 및 마일스톤

### 전체 일정: 5-7일

#### Day 1-2: HLS Muxer 구현
- [ ] HLS Muxer 기본 구조 작성
- [ ] RTP → TS 변환 로직 구현
- [ ] 세그먼트 파일 생성 테스트
- [ ] M3U8 플레이리스트 생성

#### Day 3: Stream Manager 통합
- [ ] Stream Manager에 HLS 구독자 추가
- [ ] RTP 패킷 전달 파이프라인 구성
- [ ] 다중 출력 테스트 (WebRTC + HLS 동시)

#### Day 4: HTTP API 및 파일 서빙
- [ ] HLS HTTP 엔드포인트 구현
- [ ] CORS 설정
- [ ] 정적 파일 서빙 테스트
- [ ] API 문서 업데이트

#### Day 5: 웹 UI
- [ ] HLS 뷰어 페이지 작성 (hls.js)
- [ ] 통합 뷰어 업데이트 (프로토콜 선택)
- [ ] 대시보드에 HLS 링크 추가

#### Day 6: 최적화 및 테스트
- [ ] 세그먼트 정리 로직 구현
- [ ] 성능 테스트 (다중 스트림)
- [ ] 메모리 프로파일링
- [ ] E2E 테스트 작성

#### Day 7: 문서화 및 배포
- [ ] README 업데이트
- [ ] API 문서 업데이트 (docs/API.md)
- [ ] CLAUDE.md 업데이트
- [ ] 도커 이미지 빌드 및 테스트

---

## 참조 자료

### Go 라이브러리
- **TS Muxing**: https://github.com/asticode/go-astits
- **M3U8**: https://github.com/grafov/m3u8
- **RTP**: https://github.com/pion/rtp (이미 사용 중)

### HLS 사양
- **Apple HLS RFC**: https://datatracker.ietf.org/doc/html/rfc8216
- **HLS Authoring Specification**: https://developer.apple.com/documentation/http_live_streaming

### 프론트엔드
- **hls.js**: https://github.com/video-dev/hls.js
- **Video.js HLS**: https://github.com/videojs/videojs-contrib-hls

### 참조 프로젝트
- **mediaMTX**: https://github.com/bluenviron/mediamtx (HLS 구현 참조)
- **livego**: https://github.com/gwuhaolin/livego (Go HLS 서버)

---

## 리스크 및 대응 방안

### 리스크 1: HLS 지연시간
- **리스크**: HLS는 기본적으로 세그먼트 버퍼링으로 인해 지연시간 발생
- **대응**:
  - 세그먼트 길이 최소화 (2초)
  - LL-HLS (Low-Latency HLS) 향후 고려

### 리스크 2: 디스크 공간 부족
- **리스크**: 다중 스트림 시 세그먼트 파일 누적
- **대응**:
  - 세그먼트 정리 로직 구현
  - 디스크 용량 모니터링
  - 설정으로 세그먼트 개수 제한

### 리스크 3: 라이브러리 성숙도
- **리스크**: Go HLS 라이브러리가 상대적으로 덜 검증됨
- **대응**:
  - FFmpeg 폴백 옵션 준비
  - 철저한 테스트
  - mediaMTX 코드 참조

### 리스크 4: 코덱 호환성
- **리스크**: H.265는 일부 브라우저에서 HLS 재생 불가
- **대응**:
  - H.264로 트랜스코딩 옵션 추가 (향후)
  - 브라우저별 코덱 지원 안내

---

## 핵심 요약: HLSEngine vs WebRTCEngine

### API 비교

| 기능 | WebRTCEngine | HLSEngine |
|------|-------------|-----------|
| **초기화** | `new WebRTCEngine({streamId, videoElement})` | `new HLSEngine({streamId, videoElement})` |
| **시작** | `await engine.connect()` | `await engine.load()` |
| **종료** | `engine.disconnect()` | `engine.destroy()` |
| **이벤트** | `on('connected', 'error', 'stats')` | `on('loaded', 'error', 'stats', 'playing')` |
| **통계** | 패킷, 비트레이트 | 대역폭, 버퍼, 화질 |
| **추가 기능** | ICE 상태, 재연결 | 화질 선택, 볼륨 제어 |

### 사용 패턴 동일

```javascript
// WebRTC
const webrtcEngine = new WebRTCEngine({
    streamId: 'CCTV-TEST',
    videoElement: document.getElementById('video')
});
webrtcEngine.on('connected', () => console.log('Ready'));
await webrtcEngine.connect();

// HLS (완전히 동일한 패턴!)
const hlsEngine = new HLSEngine({
    streamId: 'CCTV-TEST',
    videoElement: document.getElementById('video')
});
hlsEngine.on('loaded', () => console.log('Ready'));
await hlsEngine.load();
```

### 프로토콜 전환 매우 간단

```javascript
// 한 줄로 프로토콜 전환!
const engine = protocol === 'webrtc'
    ? new WebRTCEngine(config)
    : new HLSEngine(config);
```

### 구현 우선순위

#### High Priority (필수)
1. ✅ **HLSEngine.js** - WebRTCEngine 패턴 복제
2. 🔶 **HLS Muxer** - RTP → TS 변환
3. 🔶 **HLS Manager** - M3U8 플레이리스트
4. 🔶 **HTTP 엔드포인트** - /hls/{id}/index.m3u8

#### Medium Priority (중요)
5. 🔶 세그먼트 정리 로직
6. 🔶 통합 뷰어 (프로토콜 선택)
7. 🔶 E2E 테스트

#### Low Priority (향후)
8. 🔶 LL-HLS (저지연 HLS)
9. 🔶 적응형 비트레이트 (ABR)
10. 🔶 DVR 기능 (되감기)

---

## 다음 단계

### 즉시 시작 가능
1. **기술 검증**: go-astits, m3u8 라이브러리 PoC
2. **설계 리뷰**: 아키텍처 및 API 설계 검토
3. **환경 준비**: 개발 환경에 HLS 테스트 도구 설치

### 의사결정 필요
- [ ] 구현 옵션 선택 (Pure Go vs FFmpeg) - **권장: Pure Go**
- [ ] 세그먼트 길이 결정 (지연시간 vs 안정성) - **권장: 2초**
- [ ] 디스크 저장 vs 메모리 캐싱 전략 - **권장: 디스크 + 메모리 캐시**
- [ ] HLSEngine.js 개발 우선 vs 백엔드 우선 - **권장: 백엔드 먼저**

### 승인 대기
- [ ] HLS 구현 시작 승인
- [ ] 일정 확정 (5-7일)
- [ ] 리소스 할당 (개발자, 서버 등)

---

**문서 버전**: v1.1
**작성자**: Claude Code
**최종 수정**: 2025-11-17
**주요 추가사항**: HLSEngine.js (WebRTCEngine 스타일) 설계 추가
**다음 리뷰 일정**: 구현 시작 전
