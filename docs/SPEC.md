# tuidesk (가칭) — 폐쇄망 서버용 TUI 데스크톱 환경 기획서

## 1. 한 줄 요약

ssh로 접속한 화면 없는 리눅스 서버에서, 터미널 안에 **GUI OS 데스크톱처럼 동작하는 환경**을 띄운다. 마우스로 창을 옮기고, 파일을 드래그해서 이동하고, 우클릭 메뉴로 이름을 바꾸고, 파일을 더블클릭하면 micro처럼 편한 내장 편집기로 열린다. AI/MCP 연동은 **선택 모듈**이며 기본 빌드에는 포함하지 않는다.

## 2. 배경, 목표, 비목표

배경: 폐쇄망 Ubuntu 서버에서 nginx.conf 같은 설정 파일 수정, 파일 정리 작업을 vim/nano와 셸 명령으로만 하는 것이 불편하다. Warp 같은 편한 사용감은 로컬 GUI 앱이라 서버 안에서는 쓸 수 없다.

목표
- 실행 파일 하나(정적 바이너리)로 폐쇄망 서버에 반입·설치할 수 있다.
- 마우스만으로 파일 탐색, 이동, 복사, 이름 변경, 삭제(휴지통), 편집이 가능하다.
- 모든 마우스 동작에는 키보드 대체 수단이 있다(터미널마다 마우스 지원이 다르기 때문).
- 한글 파일명과 한글 텍스트가 깨지지 않는다.
- 런타임에 외부 네트워크 호출이 0이다(AI 모듈을 켜지 않는 한).

비목표
- 실제 그래픽(X11/Wayland) 데스크톱 대체. 이미지 렌더링, 웹 브라우저 내장은 하지 않는다.
- 다중 사용자 동시 접속 공유 세션(초기 범위 밖).
- vim/emacs 수준의 고급 편집기. 기준은 "micro 정도면 충분".

## 3. 핵심 사용 시나리오

S1. 설정 파일 수정: ssh 접속 → `tuidesk` 실행 → 파일 창에서 `/etc/nginx`로 이동 → `nginx.conf` 더블클릭 → 에디터에서 `server` 스니펫 삽입 → Ctrl+S → 권한 부족 시 "sudo로 저장" 확인창 → 저장 후 `nginx -t` 자동 검사 → 결과를 알림으로 표시.

S2. 파일 정리: 로그 파일 여러 개를 드래그 박스로 선택 → 다른 창의 `backup/` 폴더로 드래그 → "이동/복사" 팝업에서 이동 선택 → 하나를 우클릭 → "이름 바꾸기".

S3. (선택, full 빌드) 내부망 LLM이 있는 환경에서 터미널 창에 Claude Code 등 MCP 클라이언트를 실행 → tuidesk의 MCP 서버에 자동 연결 → "지금 열린 파일의 location 블록 검토해줘" 같은 요청 시 에디터의 현재 파일/선택 영역을 컨텍스트로 사용, 파일 수정은 승인 후 반영.

## 4. "너무 무거워지지 않을까?"에 대한 설계 원칙

1. **정적 단일 바이너리**: Go, `CGO_ENABLED=0`. 목표 크기 25MB 이하(lite 빌드).
2. **이벤트 기반 렌더링**: 입력·파일 변경·타이머가 있을 때만 다시 그린다. 유휴 시 CPU 사용률 거의 0, 유휴 메모리 목표 50MB 이하.
3. **무거운 기능은 뒤로, 그리고 분리**: 가장 무거운 두 가지는 내장 터미널 에뮬레이터와 AI. 둘 다 후반 Phase이고, AI/MCP는 빌드 태그로 코드 자체를 제외할 수 있다.
4. **지연 로딩**: 문법 정의, 스니펫, 대용량 디렉터리 목록은 필요할 때 로드한다.

빌드 구성
- `lite` (기본, `go build`): 데스크톱 + 파일 관리자 + 에디터 + 스니펫 + 터미널 창. AI/MCP 코드 미포함.
- `full` (`go build -tags full`): lite + MCP 서버 + AI 패널. 포함되어도 config에서 켜야 동작한다.

## 5. 기술 스택

| 영역 | 선택 | 근거 |
|---|---|---|
| 언어 | Go | 정적 바이너리, 크로스 컴파일 쉬움, micro/TUIOS 등 참고 코드가 Go |
| 터미널 I/O | `github.com/gdamore/tcell/v2` | 마우스(SGR 1006, 드래그 1002), 와이드 문자, micro가 사용 중 |
| 문자 폭 | `github.com/mattn/go-runewidth` | 한글 2칸 폭 처리 |
| 편집기 코어 | micro(MIT)의 buffer/highlight 로직을 참고·이식 | 검증된 구현, 문법 정의(YAML) 재사용 가능. 라이선스 고지 필수 |
| PTY | `github.com/creack/pty` | 터미널 창용 |
| VT 에뮬레이터 | Phase 4 스파이크에서 결정 (`charmbracelet/x/vt`, `hinshun/vt10x` 등 후보) | 성숙도 비교 필요 |
| MCP | `github.com/modelcontextprotocol/go-sdk` | 공식 Go SDK (full 빌드 전용) |
| 설정 | TOML | 사람이 손으로 고치기 쉬움 |

대안 메모: Bubble Tea v2로도 창 관리자 구현 사례(TUIOS)가 있다. Phase 0에서 tcell 직접 사용 방식과 비교해 겹치는 창 컴포지팅·드래그 히트테스트 구현 난이도를 보고 최종 결정한다. 기본안은 tcell + 자체 컴포지터.

## 6. 아키텍처

```
[터미널 I/O: tcell]
      │  원시 키/마우스 이벤트
      ▼
[입력 라우터]  더블클릭 판별(시간 기반), 드래그 상태 머신, 키맵 해석
      │
      ▼
[창 관리자/컴포지터]  z-order, 히트테스트, 포커스, 클리핑, 더티 영역 렌더링
      │
      ▼
[앱들]  FileManager · Editor · Terminal · Settings · (AI 패널)
      │  공통 App 인터페이스 + 이벤트 버스(파일 열기 요청 등)
      ▼
[서비스]  fsops(이동/복사/휴지통/실행취소) · clipboard(내부+OSC52) · sudo 헬퍼
          · snippets · config · syntax
      │
      ▼
[통합(선택)]  MCP 서버 · AI provider · 외부 앱 매니페스트
```

App 인터페이스 초안

```go
type App interface {
    ID() string
    Title() string
    MinSize() (w, h int)
    Draw(s Surface, focused bool)
    HandleEvent(ev Event) (handled bool)
    Close() error
}
```

디렉터리 구조 초안

```
cmd/tuidesk/main.go
internal/term/        # tcell 래퍼, 이벤트 정규화, 더블클릭·드래그 판별
internal/wm/          # 창 관리자, 컴포지터, 히트테스트, 작업표시줄
internal/ui/          # 위젯: 메뉴바, 컨텍스트 메뉴, 다이얼로그, 리스트, 스크롤바, 토스트
internal/apps/files/
internal/apps/editor/
internal/apps/terminal/
internal/apps/ai/     # //go:build full
internal/fsops/
internal/clipboard/
internal/snippets/
internal/config/
internal/mcp/         # //go:build full
internal/bus/
assets/syntax/  assets/snippets/  assets/themes/
docs/
```

## 7. 기능 명세

### 7.1 데스크톱 셸
- 상단 바: 런처 메뉴, 열린 창 목록, 호스트명, 시계.
- 창: 제목줄 드래그 이동, 모서리 드래그 크기 조절, 최대화/최소화/닫기 버튼, 클릭 시 앞으로.
- 하단 작업표시줄: 최소화된 창 복원.
- 바탕화면 아이콘: 즐겨찾는 경로와 앱(설정 파일로 정의). 더블클릭으로 실행.
- 창 전환 키: 터미널에서는 Alt+Tab이 전달되지 않으므로 F-키 기반 키를 쓴다. 런처 F9, 창 전환 F6/Shift+F6 (Ctrl+Space는 macOS 입력 소스 전환과 겹쳐 보조 키, ADR 0002).
- 종료 시 창 배치 저장, 다음 실행 시 복원(옵션).

### 7.2 파일 관리자 (MVP 핵심)
- 보기: 목록 보기(이름/크기/수정일/권한), 아이콘 보기(텍스트 아이콘 기본, Nerd Font는 옵션).
- 경로 바: 클릭으로 상위 이동, 직접 입력 가능.
- 더블클릭: 디렉터리 진입 / 텍스트 파일 → 에디터 / 실행 파일 → 확인 후 터미널 창에서 실행 / 바이너리 → 정보 표시.
- 선택: 클릭, Shift+클릭 범위 선택, **Ctrl+클릭(Windows)·Option+클릭(Mac) 추가 선택**, Shift+방향키 범위 선택, 드래그 박스 선택, Space로 토글(키보드). Mac의 Cmd+클릭은 터미널이 전달하지 않아 지원할 수 없다.
  - 목록 보기에서는 **선택되지 않은 줄이나 빈 곳에서 끌면 박스 선택**, **선택된 줄에서 끌면 드래그 앤 드롭**이다.
- 드래그 앤 드롭: 같은 창 안 폴더로, 다른 파일 창으로, 바탕화면으로. 드롭 시 "이동 / 복사 / 취소" 팝업(수정키 전달이 터미널마다 불안정하므로 팝업이 기본, 설정으로 "기본=이동" 가능).
- 우클릭 컨텍스트 메뉴: 열기, 다음으로 열기, 이름 바꾸기(F2), 잘라내기, 복사, 붙여넣기, 복제, 휴지통으로, 영구 삭제(이중 확인), 새 파일, 새 폴더, 경로 복사, 여기서 터미널 열기, 권한(Permissions…), 실행 권한 추가/제거(+x/−x).
- 권한(chmod) 대화상자
  - Owner/Group/Others × Read/Write/Execute 체크박스 표와 8진수 입력칸이 양방향으로 연동된다. 기호 표기(`rwxr-xr-x`)를 함께 보여 주고, 프리셋 버튼(644, 755, 600, 700, 640, 750)을 둔다.
  - 여러 파일 선택 시 값이 다른 칸은 `[-]`로 표시하고, **사용자가 바꾼 칸만** 각 파일에 적용한다.
  - 폴더가 있으면 "Apply to contents" 옵션과 폴더/파일 값을 따로 둔다(예: 폴더 755, 파일 644). 적용 전에 대상 개수를 보여 준다.
  - 소유자 읽기 제거, 모두 쓰기 허용(o+w) 같은 위험한 변경은 경고한다. 실행취소(Ctrl+Z) 대상이다.
  - 소유자/그룹은 표시만 한다. chown과 "sudo로 다시 시도"는 Phase 3에서 sudo 기능과 함께 넣는다.
  - 체크박스는 ASCII(`[x]`, `[ ]`, `[-]`)로 그린다(리눅스 콘솔).
- 빠른 작업: Make executable(+x, 읽기 권한이 있는 쪽에 실행 추가), Remove executable(−x).
- 텍스트 파일 열기는 에디터가 생기기 전(Phase 3 전)까지 읽기 전용 보기 창으로 연다.
- 안전장치: 휴지통(freedesktop 규격 `~/.local/share/Trash`), 마지막 작업 실행취소(Ctrl+Z), 이름 충돌 시 덮어쓰기/건너뛰기/이름 변경 선택, 대용량 복사 진행률과 취소.
- 숨김 파일 토글(**Alt+.**, Ctrl+H는 리눅스 콘솔에서 Backspace와 같아 쓰지 않음), 정렬, 필터(입력 시 즉시 필터링). 새로고침 F5.
- 리눅스 콘솔(`TERM=linux`)에서는 한글 파일명을 `\uD55C` 형태로 이스케이프해 보여 준다(Terminal.app에서는 그대로).
- 성능: 10만 개 파일 디렉터리에서도 멈추지 않도록 비동기 로딩 + 가상 스크롤.

### 7.2.1 에러 표시와 입력 창 (공통)
- 확인만 하면 되는 가벼운 실패는 빨간 토스트로 알린다.
- 작업이 멈추거나 데이터에 영향을 주는 에러, 사용자가 결정해야 하는 상황(권한 없음, 이름 충돌 등)은 선택지 버튼이 있는 팝업으로 띄운다. 긴 출력은 [Details]로 본다.
- 한 줄 입력 위젯(커서, 한글, 붙여넣기, 비밀번호 모드)을 공통으로 만들어 새 이름, 경로, 필터, 8진수 권한 입력에 쓴다. 비밀번호 모드는 Phase 3 sudo 입력에 쓴다.
- sudo 비밀번호는 화면에 `*`로만 보이고, 로그·감사 기록·실행취소 기록에 남기지 않으며, `sudo -S`에 한 번 넘긴 뒤 메모리에서 지운다(Phase 3).
- 일반 명령이 터미널에서 묻는 입력(`[y/N]` 등)은 Phase 4 터미널 창에서 사용자가 직접 입력한다.

### 7.3 에디터 (micro 수준)
- 키맵: Ctrl+S 저장, Ctrl+Q 닫기, Ctrl+Z/Y 실행취소/재실행, Ctrl+C/X/V, Ctrl+F 찾기, Ctrl+R 바꾸기, Ctrl+G 줄 이동, Ctrl+A 전체 선택, Home/End, Ctrl+←/→ 단어 이동, Shift+방향키 선택.
- 마우스: 클릭 커서 이동, 드래그 선택, 더블클릭 단어 선택, 트리플클릭 줄 선택, 휠 스크롤, 우클릭 메뉴(잘라내기/복사/붙여넣기/스니펫 삽입).
- 문법 하이라이트: nginx, yaml, json, toml, ini/conf, sh/bash, systemd unit, Dockerfile, python, go, markdown 우선.
- 자동 들여쓰기, 탭/스페이스·탭 크기 설정, `.editorconfig` 인식.
- 탭으로 여러 파일 열기, 줄 번호, 현재 줄 강조, 괄호 짝 표시.
- 권한 없는 파일 저장 시 "sudo로 저장" 확인 → 비밀번호 입력 다이얼로그 → `sudo tee` 방식으로 저장. 앱 자체를 root로 실행하지 않는다.
- 저장 전 자동 백업(`.bak`, 옵션).
- 저장 후 검사기(on-save validator): 경로 패턴별 명령을 설정으로 지정. 예: `/etc/nginx/**` → `nginx -t`. 결과는 토스트 알림 + 실패 시 출력 창.
- 대용량 파일(예: 50MB 이상)은 읽기 전용 모드로 열고 경고.

### 7.4 스니펫
- 형식: `assets/snippets/<언어>.toml` + 사용자 디렉터리 `~/.config/tuidesk/snippets/`.
- 사용: 트리거 단어 입력 후 Tab, 또는 Ctrl+Space 스니펫 팔레트(검색 가능), 우클릭 메뉴.
- 탭스톱 문법: `${1:server_name}`, `${2}`, `$0`. Tab으로 다음 위치 이동.
- 기본 번들
  - nginx: server, location, proxy_pass, upstream, ssl 설정, gzip, 리다이렉트
  - systemd: service 유닛, timer 유닛
  - bash: shebang + `set -euo pipefail`, 인자 파싱, 함수
  - docker-compose: service, volume, healthcheck
  - 기타: crontab 줄, logrotate 블록

```toml
[[snippet]]
trigger = "server"
title = "nginx server block"
body = """
server {
    listen ${1:80};
    server_name ${2:example.com};

    location / {
        ${0}
    }
}
"""
```

### 7.5 터미널 창 (Phase 4)
- PTY 기반 셸 창. 여러 개 열기 가능. 안에서 vim, htop, Claude Code 등 TUI 프로그램 실행 가능.
- 파일 관리자의 "여기서 터미널 열기"와 연동.
- 터미널 창의 환경변수로 `TUIDESK_SOCKET`을 노출해 외부 도구가 데스크톱과 통신할 수 있게 한다(full 빌드).

### 7.6 클립보드
- 내부 클립보드(파일 잘라내기/복사, 텍스트).
- 텍스트 복사 시 OSC 52로 사용자의 로컬(맥) 클립보드에도 전달(터미널이 지원할 때).
- bracketed paste 지원: 맥에서 Cmd+V로 붙여넣은 여러 줄이 그대로 들어오게.

### 7.7 설정
- `~/.config/tuidesk/config.toml`: 테마, 키맵 재정의, 드롭 기본 동작, 검사기 규칙, 바탕화면 아이콘, AI/MCP 활성화.
- 설정 앱(창)에서 주요 항목 변경 가능.

## 8. AI / MCP 통합 (full 빌드 전용, 기본 OFF)

원칙
- lite 빌드에는 코드가 없다. full 빌드라도 config에서 켜기 전까지 리스너도 열지 않는다.
- 외부 네트워크는 사용자가 명시한 LLM 엔드포인트로만, 그 외 호출 없음.
- 파일 쓰기, 명령 실행 같은 부작용 있는 도구는 매번 승인 다이얼로그.
- 모든 도구 호출을 감사 로그(`~/.local/state/tuidesk/audit.log`)에 기록.
- 접근 허용 경로 화이트리스트.

### 8.1 tuidesk = MCP 서버
- 전송: 로컬 유닉스 소켓(기본), stdio. 원격 바인딩 금지.
- 도구 초안
  - `fs_list(path)`, `fs_read(path)`, `fs_write(path, content)` (승인 필요)
  - `editor_open(path, line?)`, `editor_get_selection()`, `editor_insert(text)` (승인 필요)
  - `files_reveal(path)` — 파일 관리자에서 해당 파일 표시
  - `notify(message)` — 토스트 알림
  - `terminal_run(command)` (승인 필요)
  - `desktop_list_windows()`
- 터미널 창 안에서 Claude Code 같은 MCP 클라이언트를 실행하면 `TUIDESK_SOCKET`으로 바로 연결할 수 있게 연결 방법을 문서화한다.

### 8.2 AI 패널 (MCP 호스트 겸 LLM 클라이언트)
- Provider 추상화: Anthropic API / OpenAI 호환 엔드포인트(폐쇄망 내부 vLLM, Ollama 등) / 없음.
- 현재 열린 파일, 선택 영역, 파일 관리자 선택 항목을 컨텍스트로 첨부.
- 8.1의 도구를 그대로 사용(같은 승인 정책).

### 8.3 외부 앱(서드파티) 연결
- 1단계: 앱 매니페스트(`~/.config/tuidesk/apps/*.toml`)로 외부 TUI 프로그램을 런처/바탕화면에 등록 → 터미널 창 안에서 실행. (예: lazygit, btop, mc)
- 2단계: 외부 프로세스가 `TUIDESK_SOCKET`의 MCP 도구로 데스크톱과 상호작용(파일 열기, 알림 등).

```toml
# ~/.config/tuidesk/apps/lazygit.toml
name = "lazygit"
icon = "G"
command = "lazygit"
cwd = "{selected_dir}"
```

## 9. 터미널 호환성 리스크

### 9.0 지원 대상 터미널 (2026-11-04 결정, 근거: `docs/compat-matrix.md`)

| 순위 | 환경 | 범위 |
|---|---|---|
| 1 | macOS Terminal.app → ssh | 마우스 + 키보드 전체 기능 |
| 2 | 리눅스 기본 콘솔 (서버에 모니터 직결, IPMI/KVM 원격 콘솔, `TERM=linux`) | 키보드 전용 |
| 참고 | iTerm2, Warp, Ghostty, GNOME Terminal, Windows Terminal, PuTTY, Cursor/VS Code 내장 터미널 | 화면이 깨지지 않을 정도만. 전용 대응은 하지 않음 |

키보드 배열: Mac 배열 키보드와 PC(Windows) 배열 키보드를 모두 지원한다. 맥에 PC 키보드를 연결하면 Windows 키는 Cmd, Alt는 Option으로 동작한다. 키별 대응은 `docs/keyboard.md`.

UI 언어: **영어** (2026-11-04 결정). 리눅스 콘솔에는 한글 글꼴이 없어 한글 UI를 읽을 수 없으므로 메뉴, 대화상자, 도움말, 알림 문구는 영어로 쓴다. 한글 파일명·본문 같은 사용자 데이터는 계속 지원한다(Terminal.app에서 정상 표시, 콘솔에서는 13장 미결정).

리눅스 콘솔의 제약과 대응
- 마우스 없음(gpm은 tcell 미지원) → 모든 기능을 키보드로 쓸 수 있어야 한다(원래 원칙과 같음).
- 8/16색 → 테마에 16색 폴백을 둔다.
- **한글 글꼴 없음**: 커널 콘솔 글꼴은 글리프 수가 적어 한글을 표시하지 못한다. 한글 파일명이나 본문이 깨지거나 ◆로 보인다. `TERM=linux`를 감지하면 대체 표시를 쓴다(방식은 13장 미결정).
- OSC52, bracketed paste 없음 → 내부 클립보드만 쓴다.
- 키 제약(QEMU VT로 확인, `docs/compat-matrix.md` 발견 5): Shift/Ctrl+방향키와 Shift+Tab은 수정키가 사라진다. Alt+←/→와 Alt+F1~F12는 가상 콘솔 전환 키다. Shift+F10은 F20으로 들어온다. Ctrl+H는 Backspace와 구분되지 않는다. → 키맵은 리눅스 콘솔에서 되는 조합만으로 모든 기능을 쓸 수 있게 설계한다.
- 테스트 환경: `scripts/linux-console-vm.sh`(맥에서 QEMU로 Alpine VM을 띄워 커널 VT를 재현).

| 리스크 | 영향 | 대응 |
|---|---|---|
| 터미널이 우클릭을 가로챔(자체 메뉴) | 컨텍스트 메뉴 불가 | 터미널별 설정 가이드 문서화, 대체 키(Shift+F10, 메뉴 키) |
| 맥 Cmd 키는 원격 앱에 전달되지 않음 | Cmd 단축키 불가 | Ctrl 기반 키맵, 복붙은 OSC52 + bracketed paste |
| 드래그 이벤트(1002) 미지원/제한 | 드래그 앤 드롭 불가 | 잘라내기/붙여넣기, "이동" 다이얼로그 대체 |
| 수정키+클릭 전달 불안정 | Ctrl/Shift 클릭 다중 선택 불안정 | 드래그 박스 선택, Space 토글, 드롭 후 팝업 |
| 더블클릭 이벤트가 따로 없음 | — | 앱에서 시간 기반 판별(기본 400ms, 설정 가능) |
| 한글 폭/조합 | 화면 깨짐, 커서 위치 오류 | go-runewidth, 한글 파일명·텍스트 테스트를 CI에 포함 |
| terminfo 없음/구형 TERM | 색상·키 오동작 | xterm-256color 폴백, 진단 명령 `tuidesk doctor` |
| 파일 작업 사고 | 데이터 손실 | 휴지통, 실행취소, 영구 삭제 이중 확인 |
| 대용량 디렉터리 | 멈춤 | 비동기 로딩, 가상 스크롤 |

## 10. 로드맵

MVP = Phase 0 ~ 3.

### Phase 0 — 기술 검증 스파이크
- [x] 마우스/키 이벤트를 화면에 로그로 찍는 최소 tcell 앱 (`cmd/evlog`)
- [x] 지원 대상(9.0)에서 확인: Terminal.app → ssh → Ubuntu(클릭, 드래그, 우클릭, 휠, 수정키+클릭, 한글 폭, OSC52, bracketed paste) / 리눅스 콘솔(키 입력, 16색, 화면 깨짐 여부). 참고 터미널은 1차 결과만 기록 — Terminal.app의 휠·수정키+클릭·더블클릭은 미확인, Phase 1에서 재확인
- [x] 결과를 `docs/compat-matrix.md`에 표로 정리
- [x] tcell 직접 vs Bubble Tea v2 창 관리자 프로토타입 비교 후 결정 기록(`docs/adr/0001-ui-framework.md`) — tcell 직접 채택. Bubble Tea는 프로토타입 없이 검토(ADR에 사유)
- 완료 기준: 두 개 이상의 겹치는 창을 마우스로 옮길 수 있는 데모 — ✅ `cmd/wmdemo` (시뮬레이션 테스트 + 리눅스 콘솔 VM 확인, 2026-11-04)

### Phase 1 — 데스크톱 셸 + 창 관리자
- [x] 컴포지터(z-order, 클리핑, 더티 영역 렌더링) — 이전 프레임과 달라진 셀만 출력(`internal/tui`)
- [x] 창 이동/크기 조절/최대화/최소화/닫기, 포커스
- [x] 상단 바, 작업표시줄, 런처(F9, 보조 Ctrl+Space — ADR 0002)
- [x] 공통 위젯: 메뉴, 컨텍스트 메뉴, 다이얼로그, 토스트, 리스트, 스크롤바
- [x] 더블클릭·드래그 판별 로직 + 단위 테스트
- 완료 기준: 빈 창 여러 개를 마우스와 키보드로 모두 조작 가능, 유휴 CPU 거의 0 — 시나리오 테스트(마우스 전용·키보드 전용) 통과, 유휴 10초 CPU 0.00s·RSS 14.8MB, 리눅스 콘솔 VM 확인(`docs/img/phase1-console-*.png`). Terminal.app 실사용 확인 대기 (2026-11-04)

### Phase 2 — 파일 관리자
- [x] 공통 입력 위젯, 입력/에러 대화상자(7.2.1)
- [ ] 목록/아이콘 보기, 경로 바, 정렬, 숨김 토글, 필터 — 목록 보기·경로 바·정렬·숨김(Alt+.)·필터 완료, **아이콘 보기 미구현**
- [x] 선택(클릭, Shift, 드래그 박스, Space)
- [ ] 드래그 앤 드롭(창 내/창 간/바탕화면) + 이동/복사 팝업 — 창 내 폴더·창 간 완료, **바탕화면으로 드롭은 바탕화면 아이콘(7.1)과 함께 미구현**
- [x] 우클릭 메뉴 전체 항목, 이름 바꾸기 인라인 편집 ("다음으로 열기"는 에디터가 생기는 Phase 3, "여기서 터미널 열기"는 Phase 4에서 활성화)
- [x] 권한(chmod) 대화상자, +x/−x 빠른 작업
- [x] fsops: 이동/복사/휴지통/실행취소/충돌 처리/진행률 + 임시 디렉터리 기반 테스트
- 완료 기준: 시나리오 S2를 마우스만으로 수행 가능 — ✅ 시나리오 테스트(`internal/desktop/s2_test.go`)에서 마우스만으로 박스 선택→창 간 드래그→Move→우클릭 Rename까지 통과, 리눅스 콘솔 VM 확인(`docs/img/phase2-console-*.png`). Terminal.app 실사용 확인 대기 (2026-11-05)

### Phase 3 — 에디터 + 스니펫
- [ ] 버퍼, 실행취소, 선택, 클립보드, 찾기/바꾸기
- [ ] 문법 하이라이트(우선 목록)
- [ ] 스니펫 엔진 + 팔레트 + 기본 번들
- [ ] sudo 저장, 백업, on-save validator
- [ ] 파일 관리자 더블클릭 → 에디터 연동
- 완료 기준: 시나리오 S1을 처음부터 끝까지 수행 가능

### Phase 4 — 터미널 창
- [ ] VT 에뮬레이터 선택 스파이크 → PTY 셸 창
- [ ] "여기서 터미널 열기", 앱 매니페스트(8.3 1단계)
- 완료 기준: 터미널 창 안에서 htop, micro가 정상 동작

### Phase 5 — MCP 서버 (full 빌드)
- [ ] 8.1 도구, 승인 다이얼로그, 감사 로그, 경로 화이트리스트
- [ ] 터미널 창 안의 MCP 클라이언트 연결 문서
- 완료 기준: 시나리오 S3에서 editor_open, editor_get_selection 동작

### Phase 6 — AI 패널 (full 빌드)
- [ ] provider 추상화(Anthropic / OpenAI 호환 / 없음)
- [ ] 컨텍스트 첨부, 도구 호출 승인 흐름

## 11. 폐쇄망 배포

- 빌드: `CGO_ENABLED=0 GOOS=linux GOARCH=amd64|arm64 go build -trimpath -ldflags "-s -w"`
- 산출물: `tuidesk-<버전>-linux-<arch>.tar.gz` (바이너리, 기본 assets, `install.sh`, `SHA256SUMS`, 오프라인 README)
- `install.sh`: `/usr/local/bin` 또는 `~/.local/bin` 설치, 설정 디렉터리 생성, 기존 설정 덮어쓰지 않음
- 폐쇄망 내부에서 직접 빌드할 수 있도록 `go mod vendor` 결과 포함한 소스 tarball도 제공
- `tuidesk doctor`: TERM, 색상 수, 마우스 지원 여부, 한글 폭, OSC52 진단

## 12. 라이선스 메모

- 프로젝트 라이선스: MIT 또는 Apache-2.0 권장(사내/폐쇄망 배포 자유도).
- micro(MIT) 코드 이식 시 원 저작권 고지를 `THIRD_PARTY_NOTICES.md`에 유지.
- GPL/AGPL 의존성은 추가하지 않는다(iTerm2, Warp는 참고만).

## 13. 미결정 사항

- 제품 이름
- ~~UI 프레임워크 최종 결정(Phase 0 결과)~~ → tcell 직접 + 자체 컴포지터 (ADR 0001)
- 아이콘 보기에서 Nerd Font 사용 여부(사용자 맥에 폰트 설치 필요)
- 같은 서버 다중 사용자 사용 시 설정/휴지통 분리 정책
- root로 실행했을 때의 정책(경고만 / 차단)
- 리눅스 콘솔에서 한글 표시 방식(`?` 대체 / `\uXXXX` 이스케이프 / 상태줄에 원래 이름 별도 표시 등)
