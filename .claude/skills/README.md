# Skills Directory - Living Documentation

> **핵심 원칙**: Skills는 CLAUDE.md처럼 살아있는 문서입니다. 프로젝트가 진행되면서 지속적으로 CRUD하며 최신 상태를 유지해야 합니다.

## 📋 Skills 목록

### 1. rtsp-webrtc-streaming.md
**버전**: 1.0 (2025-10-29)
**상태**: ✅ Production-ready
**설명**: RTSP to WebRTC 미디어 스트리밍 시스템 구축을 위한 종합 가이드

**포함 내용**:
- mediaMTX 스타일 다중 카메라 시스템
- 동적 코덱 선택 (H.265/H.264)
- 온디맨드 스트림 관리
- 재사용 가능한 WebRTC 엔진 라이브러리
- 실전 트러블슈팅 (9가지 해결된 이슈)
- 프로젝트 템플릿 및 Quick Start

**사용 시나리오**:
- IP 카메라 웹 스트리밍 구축
- 보안 모니터링 대시보드 개발
- 실시간 영상 전송 시스템
- 다중 카메라 관제 시스템

**다음 업데이트 계획**:
- PTZ 카메라 제어 패턴 추가
- 녹화/재생 기능 추가
- TURN 서버 설정 가이드
- Docker 배포 패턴
- Prometheus 메트릭 통합

---

## 🔄 Skill 관리 가이드

### Skill 추가하기 (Create)

1. `.claude/skills/` 디렉토리에 새 마크다운 파일 생성
2. 다음 구조를 따름:
   ```markdown
   # Skill Title
   > Living Skill Document: CRUD 원칙 명시

   ## 📋 Skill Overview
   - 사용 시나리오
   - 기술 스택

   ## 🏗️ Architecture Pattern
   ## 🔑 Critical Implementation Patterns
   ## 🐛 Common Issues & Solutions
   ## 📚 References
   ## 🔄 Skill Maintenance Log
   ```
3. 이 README.md에 skill 정보 추가
4. Git에 커밋

### Skill 읽기 (Read)

```bash
# 특정 skill 보기
cat .claude/skills/rtsp-webrtc-streaming.md

# 모든 skills 목록
ls -la .claude/skills/

# Skill 검색
grep -r "WebRTC" .claude/skills/
```

### Skill 업데이트하기 (Update)

1. 프로젝트에서 새로운 패턴 발견 시:
   - Skill 파일 열기
   - 해당 섹션 업데이트
   - "Skill Maintenance Log" 섹션에 변경사항 기록

2. 업데이트 예시:
   ```markdown
   ## 🔄 Skill Maintenance Log

   ### Version 1.1 (2025-11-01)
   - Added PTZ control pattern
   - Updated ICE handling for complex NAT
   - Fixed TURN server configuration example
   ```

3. 이 README.md의 버전 정보 갱신
4. Git에 커밋

### Skill 삭제하기 (Delete)

1. Skill 파일 삭제
2. 이 README.md에서 해당 skill 항목 제거
3. Git에 커밋

---

## 📝 Skill 작성 Best Practices

### 1. 재사용 가능성
- 특정 프로젝트가 아닌 일반적인 패턴으로 작성
- 다른 프로젝트에서도 적용 가능하도록

### 2. 실전 경험 기반
- 실제로 겪은 문제와 해결책 포함
- 이론보다는 실용적인 코드 예시

### 3. 트러블슈팅 중심
- "Common Issues & Solutions" 섹션 필수
- 에러 메시지와 해결 방법 명시

### 4. 코드 스니펫
- 복사-붙여넣기 가능한 코드
- 주석으로 설명 추가
- ❌/✅ 패턴으로 잘못된/올바른 방법 비교

### 5. 버전 관리
- "Skill Maintenance Log" 섹션 필수
- 날짜와 함께 변경사항 기록
- 기반 프로젝트 버전 명시

---

## 🔗 Skill 간 연관성

현재는 단일 skill이지만, 향후 다음과 같이 분리 가능:

```
rtsp-webrtc-streaming.md (메인)
  ├─ webrtc-peer-management.md (WebRTC 피어 관리)
  ├─ rtsp-client-patterns.md (RTSP 클라이언트 패턴)
  ├─ mediamtx-config.md (mediaMTX 설정 패턴)
  └─ stream-pubsub.md (스트림 Pub/Sub 패턴)
```

---

## 📊 Skill 메트릭

| Metric | Value |
|--------|-------|
| Total Skills | 1 |
| Production-ready | 1 |
| In Development | 0 |
| Total Lines | 800+ |
| Code Examples | 20+ |
| Resolved Issues | 9 |

---

## 🎯 향후 Skill 계획

### 우선순위 1 (단기)
- [ ] PTZ 카메라 제어 패턴
- [ ] 스트림 녹화/재생 기능
- [ ] Docker 배포 가이드

### 우선순위 2 (중기)
- [ ] Kubernetes 배포 패턴
- [ ] 부하 테스트 및 성능 최적화
- [ ] 모니터링 및 알림 시스템

### 우선순위 3 (장기)
- [ ] AI 기반 객체 감지 통합
- [ ] 클라우드 스토리지 연동
- [ ] 모바일 앱 통합

---

## 💡 Skill 사용 가이드

### Claude Code에서 Skill 활용하기

1. **프롬프트에서 참조**:
   ```
   "RTSP to WebRTC 스트리밍 시스템을 구축하려고 해.
   .claude/skills/rtsp-webrtc-streaming.md를 참고해서 설계해줘."
   ```

2. **특정 패턴 적용**:
   ```
   "동적 코덱 선택 패턴을 적용해서 브라우저 호환성을 개선하고 싶어.
   rtsp-webrtc-streaming skill의 패턴을 사용해줘."
   ```

3. **트러블슈팅**:
   ```
   "RTP 패킷을 못 받고 있어. rtsp-webrtc-streaming skill의
   트러블슈팅 섹션을 참고해서 해결해줘."
   ```

---

## 📚 관련 문서

- **CLAUDE.md**: 현재 프로젝트의 살아있는 문서
- **README.md**: 프로젝트 소개 및 사용 가이드
- **Skills**: 재사용 가능한 패턴 및 지식

**차이점**:
- CLAUDE.md: 현재 프로젝트의 구체적인 상태와 의사결정
- Skills: 일반화된 패턴과 재사용 가능한 지식

---

**Last Updated**: 2025-10-29
**Maintained By**: cctv3 project team
**License**: Internal use
