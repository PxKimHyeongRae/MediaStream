# 🎥 RTSP to WebRTC 미디어 서버 사용 가이드

> IP 카메라 영상을 웹 브라우저에서 실시간으로 시청할 수 있는 시스템입니다.

## 📖 목차

1. [시작하기 전에](#시작하기-전에)
2. [설치하기](#설치하기)
3. [설정하기](#설정하기)
4. [실행하기](#실행하기)
5. [웹에서 보기](#웹에서-보기)
6. [문제 해결](#문제-해결)

---

## 시작하기 전에

### 필요한 것들

- **Go 언어**: 1.23 이상
  - 다운로드: https://golang.org/dl/
  - 설치 확인: `go version`

- **Git**: 코드 다운로드용
  - 다운로드: https://git-scm.com/
  - 설치 확인: `git --version`

- **IP 카메라**: RTSP 프로토콜 지원 카메라
  - RTSP URL 예시: `rtsp://admin:password@192.168.1.100:554/stream`

### 이 시스템이 하는 일

```
[IP 카메라] --RTSP--> [이 프로그램] --WebRTC--> [웹 브라우저]
                      (변환해줌)
```

- IP 카메라의 RTSP 스트림을 받아서
- WebRTC로 변환해서
- 웹 브라우저에서 실시간으로 볼 수 있게 해줍니다!

---

## 설치하기

### 1단계: 코드 다운로드

```bash
# GitHub에서 다운로드
git clone https://github.com/PxKimHyeongRae/MediaStream.git
cd MediaStream
```

### 2단계: 의존성 설치

```bash
# Go 모듈 다운로드
go mod download
```

### 3단계: 빌드하기

#### Windows
```bash
go build -o bin/media-server.exe cmd/server/main.go
```

#### Mac / Linux
```bash
go build -o bin/media-server cmd/server/main.go
```

빌드가 완료되면 `bin/` 폴더에 실행 파일이 생성됩니다!

---

## 설정하기

### 카메라 정보 입력하기

`configs/config.yaml` 파일을 열어서 카메라 정보를 입력합니다.

#### 예시 1: 기본 카메라

```yaml
paths:
  my_camera:
    source: rtsp://192.168.1.100:554/stream
    sourceOnDemand: no
    rtspTransport: tcp
```

#### 예시 2: 비밀번호가 있는 카메라

```yaml
paths:
  my_camera:
    source: rtsp://admin:password123@192.168.1.100:554/stream
    sourceOnDemand: no
    rtspTransport: tcp
```

#### 예시 3: 여러 대의 카메라

```yaml
paths:
  living_room:
    source: rtsp://192.168.1.100:554/stream
    sourceOnDemand: no
    rtspTransport: tcp

  front_door:
    source: rtsp://192.168.1.101:554/stream
    sourceOnDemand: yes
    rtspTransport: tcp

  parking:
    source: rtsp://192.168.1.102:554/stream
    sourceOnDemand: yes
    rtspTransport: tcp
```

### 설정 항목 설명

| 항목 | 설명 | 예시 |
|------|------|------|
| `my_camera` | 카메라 이름 (원하는 대로) | `living_room`, `front_door` |
| `source` | 카메라 RTSP 주소 | `rtsp://ip주소:포트/경로` |
| `sourceOnDemand` | 필요할 때만 연결 (`yes`/`no`) | `no`: 항상 연결, `yes`: 필요시 연결 |
| `rtspTransport` | 전송 방식 | `tcp` (권장) 또는 `udp` |

### ⚠️ 비밀번호 특수문자 주의!

비밀번호에 특수문자가 있으면 변환해야 합니다:

| 특수문자 | 변환 | 예시 |
|---------|------|------|
| `!` | `%21` | `pass!word` → `pass%21word` |
| `@` | `%40` | `pass@word` → `pass%40word` |
| `#` | `%23` | `pass#word` → `pass%23word` |

**변환 예시:**
```yaml
# 원래 비밀번호: admin123!@#
# 변환 후:
source: rtsp://admin:admin123%21%40%23@192.168.1.100:554/stream
```

---

## 실행하기

### Windows에서 실행

```bash
# cmd 또는 PowerShell에서
bin\media-server.exe
```

### Mac / Linux에서 실행

```bash
# 터미널에서
./bin/media-server
```

### 정상 실행 확인

다음과 같은 메시지가 나오면 성공입니다:

```
INFO: Starting RTSP to WebRTC Media Server
INFO: HTTP server listening on :8080
INFO: WebSocket server listening on :8081
INFO: Stream 'my_camera' connected
```

---

## 웹에서 보기

서버가 실행 중이면 웹 브라우저를 열고 접속하세요!

### 대시보드 (모든 카메라 한눈에)

```
http://localhost:8080/static/dashboard.html
```

- 모든 카메라를 그리드로 표시
- 자동으로 연결됨
- 각 카메라별 개별 제어 가능

### 단일 뷰어 (카메라 하나씩)

```
http://localhost:8080/static/viewer.html
```

- 드롭다운에서 카메라 선택
- 연결 버튼 클릭
- 통계 정보 표시

### API 테스트

```
http://localhost:8080/api/v1/health
```

서버가 정상이면 `{"status":"ok"}` 응답이 나옵니다.

---

## 문제 해결

### 1. 서버가 시작 안 돼요

#### 포트가 이미 사용 중
```
Error: listen tcp :8080: bind: address already in use
```

**해결:**
- 다른 프로그램이 8080 포트를 사용 중입니다.
- 해당 프로그램을 종료하거나
- `config.yaml`에서 포트 번호를 변경하세요:

```yaml
server:
  http_port: 9080  # 8080 → 9080으로 변경
```

#### Go 버전이 낮음
```
Error: go.mod requires go >= 1.23.0
```

**해결:**
- Go를 최신 버전으로 업데이트하세요: https://golang.org/dl/

---

### 2. 카메라 연결이 안 돼요

#### 401 Unauthorized
```
Error: bad status code: 401 (Unauthorized)
```

**해결:**
- 카메라 사용자명/비밀번호를 확인하세요
- 비밀번호의 특수문자를 URL 인코딩했는지 확인하세요

#### Timeout
```
Error: connection timeout
```

**해결:**
1. 카메라 IP 주소가 맞는지 확인
2. 네트워크 연결 확인
3. 카메라가 켜져 있는지 확인
4. 방화벽 설정 확인

**네트워크 확인 방법:**
```bash
# Windows
ping 192.168.1.100

# Mac/Linux
ping 192.168.1.100
```

---

### 3. 웹에서 영상이 안 나와요

#### 연결 중에서 멈춤

**확인 사항:**
1. 서버가 실행 중인가요?
   - 터미널에서 에러 메시지 확인

2. 브라우저 콘솔 확인
   - F12 키 → Console 탭
   - 에러 메시지 확인

3. 방화벽 확인
   - Windows: 방화벽에서 프로그램 허용
   - Mac: 시스템 환경설정 → 보안 및 개인정보 보호

#### Firefox에서 안 보임

**원인:** Firefox는 H.265 코덱을 지원하지 않습니다.

**해결:**
- Chrome 또는 Edge 브라우저를 사용하세요
- 또는 카메라를 H.264로 설정하세요

---

### 4. 성능이 느려요

#### 여러 카메라 동시 시청 시 느림

**해결책:**

1. **config.yaml 성능 설정 조정**
```yaml
performance:
  worker_pool_size: 200  # 100 → 200
  gc_percent: 30         # 50 → 30
```

2. **온디맨드 스트림 사용**
```yaml
paths:
  my_camera:
    sourceOnDemand: yes  # 필요할 때만 연결
```

3. **비트레이트 제한**
```yaml
media:
  codec:
    max_bitrate: 2000  # 2Mbps로 제한
```

---

## 고급 사용법

### Makefile 사용하기

더 쉽게 명령어를 실행할 수 있습니다:

```bash
# 빌드
make build

# 실행
make run

# 테스트
make test

# 정리
make clean
```

### 로그 레벨 변경

`config.yaml`에서 로그 상세도를 조정할 수 있습니다:

```yaml
logging:
  level: "debug"  # debug, info, warn, error
  output: "both"  # console, file, both
```

### 여러 서버 실행

포트를 다르게 설정하여 여러 서버를 실행할 수 있습니다:

```yaml
# config1.yaml
server:
  http_port: 8080

# config2.yaml
server:
  http_port: 9080
```

```bash
# 서버 1
./bin/media-server -config=configs/config1.yaml

# 서버 2 (다른 터미널에서)
./bin/media-server -config=configs/config2.yaml
```

---

## 추가 도움말

### 더 자세한 정보

- **CLAUDE.md**: 개발자를 위한 상세 문서
- **README.md**: 프로젝트 소개
- **.claude/skills/**: 재사용 가능한 패턴 및 지식

### 브라우저 호환성

| 브라우저 | H.264 | H.265 |
|---------|-------|-------|
| Chrome 107+ | ✅ | ✅ |
| Edge 107+ | ✅ | ✅ |
| Firefox | ✅ | ❌ |
| Safari 16+ | ✅ | ✅ |

### 권장 카메라 설정

- **코덱**: H.264 (호환성 최고) 또는 H.265 (화질 우수)
- **해상도**: 1920x1080 (Full HD)
- **프레임레이트**: 15-30 fps
- **비트레이트**: 2-4 Mbps
- **전송 방식**: TCP (안정성) 또는 UDP (지연시간)

---

## 자주 묻는 질문 (FAQ)

### Q: 외부에서도 접속할 수 있나요?

A: 로컬에서만 작동합니다. 외부 접속을 위해서는:
1. 공유기 포트 포워딩 설정 (8080, 8081 포트)
2. HTTPS/WSS 설정 필요
3. 도메인 또는 고정 IP 필요

### Q: 녹화 기능이 있나요?

A: 현재 버전에는 없습니다. 향후 업데이트 예정입니다.

### Q: 몇 개의 카메라를 동시에 볼 수 있나요?

A: 서버 성능에 따라 다릅니다. 일반 PC에서는 4-8개 권장합니다.

### Q: 모바일에서도 되나요?

A: 네! 모바일 브라우저(Chrome, Safari)에서 동일한 주소로 접속하면 됩니다.

### Q: 비용이 드나요?

A: 무료 오픈소스입니다!

---

## 업데이트

최신 버전으로 업데이트하려면:

```bash
# 최신 코드 받기
git pull

# 다시 빌드
go build -o bin/media-server.exe cmd/server/main.go

# 실행
bin/media-server.exe
```

---

## 도움이 더 필요하세요?

- **GitHub Issues**: https://github.com/PxKimHyeongRae/MediaStream/issues
- **이메일**: (이메일 주소)
- **Wiki**: (Wiki 링크)

---

**즐거운 CCTV 모니터링 되세요!** 🎥✨

> 이 가이드로 해결되지 않는 문제가 있다면 GitHub Issues에 질문을 남겨주세요.
