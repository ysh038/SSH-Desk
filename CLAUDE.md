# CLAUDE.md — tuidesk 작업 규칙

## 프로젝트 개요
폐쇄망 리눅스 서버에서 ssh로 접속해 쓰는 TUI 데스크톱 환경. 전체 기획은 `docs/SPEC.md`를 기준으로 한다. 작업 전에 반드시 SPEC.md를 읽고, 현재 Phase의 체크리스트만 진행한다.

## 기술 스택
- Go (최신 안정 버전), `gdamore/tcell/v2`, `mattn/go-runewidth`
- 단, UI 프레임워크 최종 결정은 Phase 0 결과(`docs/adr/0001-ui-framework.md`)를 따른다.

## 명령
- 빌드(lite, 기본): `go build ./cmd/tuidesk`
- 빌드(full): `go build -tags full ./cmd/tuidesk`
- 테스트: `go test ./...`
- 린트: `go vet ./...` (+ 가능하면 `staticcheck ./...`)
- 리눅스 정적 빌드 확인: `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" ./cmd/tuidesk`

## 반드시 지킬 규칙
1. **Phase 순서대로** 진행한다. 다음 Phase로 넘어가기 전에 현재 Phase의 완료 기준을 만족하고, SPEC.md 체크박스를 갱신한다.
2. **CGO 금지.** 항상 `CGO_ENABLED=0`으로 빌드되어야 한다.
3. **lite 빌드는 네트워크 코드를 포함하지 않는다.** AI/MCP 관련 파일은 `//go:build full` 태그를 단다. lite 바이너리에 `net/http` 클라이언트 호출이 생기지 않게 주의한다.
4. **모든 마우스 동작에는 키보드 대체 수단**을 만든다.
5. **한글(와이드 문자) 처리**: 화면 폭 계산은 항상 runewidth 기준. 한글 파일명·텍스트 테스트 케이스를 함께 작성한다.
6. **파일 작업은 안전하게**: 삭제는 기본 휴지통, 영구 삭제는 이중 확인. fsops 테스트는 반드시 `t.TempDir()`에서만 수행하고 실제 홈/시스템 경로를 건드리지 않는다.
7. **앱을 root로 실행한다고 가정하지 않는다.** 권한 상승은 저장 시 `sudo tee` 방식으로만.
8. **의존성 추가는 신중히**: GPL/AGPL 라이브러리 금지. 새 의존성을 추가하면 이유를 커밋 메시지에 남기고 `THIRD_PARTY_NOTICES.md`를 갱신한다.
9. **렌더링은 이벤트 기반**: 바쁜 루프나 고정 주기 전체 재렌더링 금지(시계 같은 타이머는 해당 영역만).
10. 터미널 호환성에 영향을 주는 결정은 `docs/compat-matrix.md` 또는 `docs/adr/`에 기록한다.

## 코드 스타일
- 패키지 구조는 SPEC.md 6장을 따른다.
- 로직(드래그 판별, 더블클릭 판별, 히트테스트, fsops, 스니펫 파싱)은 tcell에 의존하지 않는 순수 함수로 분리해 단위 테스트한다.
- 사용자에게 보이는 문구는 한국어, 코드/주석/커밋 메시지는 영어도 무방.

## 작업 방식
- 큰 작업은 먼저 계획을 짧게 제시하고 진행한다.
- 작은 단위로 커밋한다.
- 터미널에서 직접 확인이 필요한 부분(실제 마우스 동작 등)은 내가 수동 테스트할 수 있도록 확인 방법을 정리해서 알려준다.
