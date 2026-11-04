# 터미널 호환성 매트릭스

절차는 [phase0-checklist.md](phase0-checklist.md)를 따릅니다. 원본 로그는 `docs/compat-logs/`에 둡니다.

범례: ✅ 정상 · ⚠️ 조건부(설정 변경 필요/일부만) · ❌ 안 됨 · ❓ 미확인
칸 안의 "L"은 맥 로컬 실행, "S"는 ssh로 서버에서 실행한 결과입니다.
Cursor 내장 터미널(xterm.js 기반, `TERM_PROGRAM=vscode`)은 참고용 열입니다. 지원 대상이 아닙니다(발견 1의 결정 참고).

## 테스트 환경

| 항목 | 값 |
|---|---|
| 맥 OS 버전 | macOS 26.5.1 |
| 서버 OS | Ubuntu 20.04.6 LTS (dev-server) |
| 서버 아키텍처 | amd64 |
| evlog 커밋 | a3eb906 |
| 테스트 일자 | 2026-11-04 (1차: 로컬 iTerm2·Terminal.app, 서버 iTerm2·Cursor 터미널) |

| 터미널 | 버전 | 서버에서 본 TERM | 바꾼 설정 |
|---|---|---|---|
| Warp | | 미테스트 | |
| iTerm2 | 3.7.6 | xterm-256color (로컬·서버 동일, 서버에선 COLORTERM 없음 → 256색) | 기본값 |
| Ghostty | | 미테스트 | |
| Terminal.app | 470.2 | xterm-256color (로컬) | 기본값 |
| Cursor 내장 터미널 | 3.27.22 | xterm-256color (서버, LANG=en_US.UTF-8) | 기본값 |

## 결과

| ID | 항목 | Warp | iTerm2 | Ghostty | Terminal.app | Cursor 내장 터미널 | tmux 안 (선택) | 비고 |
|---|---|---|---|---|---|---|---|---|
| M1 | 왼쪽 클릭 | ❓ | L✅ S✅ | ❓ | L✅ | S⚠️ | ❓ | Cursor: 누름만 오고 뗌(release)이 안 옴 — 발견 1 |
| M2 | 더블/트리플 클릭 | ❓ | ❓ | ❓ | ❓ | ❓ | ❓ | 더블클릭 판별 로그 없음(시도 안 했거나 실패 — 재확인) |
| M3 | 우클릭 | ❓ | S❌ | ❓ | L✅ | S⚠️ | ❓ | iTerm2(기본 설정): 우클릭 시 iTerm2 메뉴가 뜨고 앱에 전달되지 않음 — 발견 4. Terminal.app L: Click까지 정상. Cursor: 누름만 옴 |
| M4 | 드래그(1002) | ❓ | L✅ S✅ | ❓ | L✅ | S✅ | ❓ | Cursor: (1,1)→(199,25) 드래그 정상 |
| M5 | 휠(세로/가로) | ❓ | L✅ | ❓ | ❓ | ❓ | ❓ | iTerm2: Wheel Up/Down 정상, 가로 휠 미확인 |
| M6 | 수정키+클릭 (Shift/Option/Ctrl/Cmd) | ❓ | ❓ | ❓ | ❓ | ❓ | ❓ | 모든 로그 mod=- (시도 여부 확인 필요) |
| M7 | 모션(1003) | ❓ | ❓ | ❓ | ❓ | ❓ | ❓ |  |
| M8 | 한글 위 좌표 정확도 | ❓ | ❓ | ❓ | ❓ | ❓ | ❓ |  |
| M9 | 가운데 버튼 | ❓ | ❓ | ❓ | ❓ | ❓ | ❓ |  |
| K1 | Ctrl 단축키 (S/Q/Z/Y/F/H/Space) | ❓ | L⚠️ S⚠️ | ❓ | L✅ | S✅ | ❓ | Ctrl+C는 정상. 한글 입력 상태에서 iTerm2는 로컬·서버 모두 Ctrl+C가 `Ctrl+ㅊ`로 옴 — 발견 2 |
| K2 | Option(Alt) 조합 | ❓ | ❓ | ❓ | ❓ | ❓ | ❓ |  |
| K3 | F1~F12, Shift+F10 | ❓ | ❓ | ❓ | ❓ | ❓ | ❓ |  |
| K4 | Shift/Ctrl+방향키, Home/End | ❓ | ❓ | ❓ | ❓ | ❓ | ❓ |  |
| K5 | Esc 단독(지연) | ❓ | ❓ | ❓ | ❓ | ❓ | ❓ |  |
| K6 | Cmd 키 (전달 안 되는 게 정상) | ❓ | ❓ | ❓ | ❓ | ❓ | ❓ |  |
| K7 | 한글 IME 입력 | ❓ | ❓ | ❓ | L✅ | ❓ | ❓ | Terminal.app: 자모(ㅁㄴㅇ)가 개별 rune으로 도착 |
| T1 | 한글 폭 | ❓ | ❓ | ❓ | ❓ | ❓ | ❓ |  |
| T2 | 모호폭 문자 | ❓ | ❓ | ❓ | ❓ | ❓ | ❓ |  |
| T3 | 이모지(참고) | ❓ | ❓ | ❓ | ❓ | ❓ | ❓ |  |
| C1 | OSC52 쓰기 | ❓ | ❓ | ❓ | ❓ | ❓ | ❓ |  |
| C2 | OSC52 읽기(참고) | ❓ | ❓ | ❓ | ❓ | ❓ | ❓ |  |
| P1 | bracketed paste | ❓ | ❓ | ❓ | ❓ | ❓ | ❓ |  |
| P2 | 한글 붙여넣기 | ❓ | ❓ | ❓ | ❓ | ❓ | ❓ |  |
| E1 | 포커스 이벤트 | ❓ | L✅ | ❓ | ❓ | S✅ | ❓ | Terminal.app은 포커스 이벤트 없음(미지원인지 재확인) |
| E2 | 리사이즈 | ❓ | ❓ | ❓ | ❓ | ❓ | ❓ |  |

## 터미널별 메모

### Warp

### iTerm2
- 로컬: 클릭, 드래그, 휠, 포커스가 모두 정상입니다. 클릭할 때 누름(press)과 뗌(release)이 70~120ms 간격으로 들어옵니다.
- 서버(`evlog-iterm2-20261104-154809.log`): 클릭, 드래그, 우클릭(누름과 뗌)이 모두 정상이고, 로컬 결과와 같습니다.
- ssh로 접속하면 `COLORTERM`이 서버에 전달되지 않아 256색으로 동작합니다. 로컬에서는 트루컬러로 동작했습니다. 테마는 256색 기준으로 만들고, 트루컬러는 가능한 환경에서만 쓰도록 합니다.
- **우클릭은 앱에 전달되지 않습니다(기본 설정).** 우클릭하면 iTerm2 자체 메뉴가 뜨고, evlog 화면 로그에도 찍히지 않습니다(사용자 확인). 로그에 남은 `btn=Right` 1건은 일반 우클릭이 아닌 다른 입력으로 보입니다. 대응은 발견 4에 정리했습니다.
- 서버에서도 한글 입력 상태에서는 Ctrl+C가 `Ctrl+ㅊ`로 5번 들어왔습니다. 이 때문에 evlog가 종료되지 않았고, 입력기를 영문으로 바꾼 뒤에야 종료할 수 있었습니다(발견 2가 실제로 사용을 막은 사례).

### Cursor 내장 터미널
- 서버(`evlog-cursor-20261104-145748.log`): 드래그는 정상이고, release도 드래그 끝에서만 왔습니다. 일반 클릭과 우클릭은 누름만 오고 release가 오지 않았습니다.
- Cursor(VS Code 계열)의 터미널이 클릭과 우클릭을 자체 기능(선택, 컨텍스트 메뉴, 링크 열기 등)에 쓰면서 release를 가로채는 것으로 추정합니다. Cursor 설정 `terminal.integrated.rightClickBehavior` 등을 바꿔 다시 확인해 볼 수 있습니다.

### Ghostty

### Terminal.app
- 로컬: 왼쪽 클릭, 우클릭(release와 Click 판별까지), 드래그가 모두 정상입니다. 터미널 자체 메뉴도 뜨지 않았습니다.
- 포커스 이벤트는 기록되지 않았습니다.

## 결과에서 나온 발견 (1차)

1. **뗌(release) 누락.** Cursor 내장 터미널에서 클릭의 release가 오지 않았습니다. 실제 터미널(iTerm2, Terminal.app)에서는 정상이었습니다.
   - 문제: tcell의 `EventMouse`는 "버튼 상태"만 알려주고, 새로 누른 것인지 누른 채 움직인 것인지 구분하지 않습니다. release가 빠진 상태에서 다른 위치를 누르면, 현재 판별 로직은 이를 **드래그로 오인**합니다.
   - **결정(2026-11-04): Cursor/VS Code 내장 터미널은 지원 대상에서 제외합니다.** 이 터미널에서는 키보드 조작을 쓰도록 문서에만 안내하고, release 누락을 보정하는 로직은 넣지 않습니다.
   - 다만 release 누락에 대비해 상태가 꼬이지 않게 하는 최소한의 방어(다른 위치를 새로 누르면 드래그로 오인하지 않기)는 Phase 1에서 다시 검토합니다. tcell을 그대로 쓸지, SGR 원시 시퀀스를 직접 파싱할지와 함께 ADR 0001의 비교 항목에 넣습니다.
2. **한글 입력 상태의 Ctrl 단축키.** tcell은 시작할 때 확장 키 프로토콜(kitty CSI-u, xterm modifyOtherKeys)을 켭니다. 이 때문에 iTerm2에서 한글 입력기가 켜져 있으면 Ctrl+C가 `Ctrl+ㅊ`(U+314A)로 들어옵니다.
   - 대응(적용함): `internal/term/keymap.go`의 `ShortcutRune`이 Ctrl/Alt와 함께 들어온 두벌식 자모를 QWERTY 키로 바꿉니다(ㅊ→c, ㄴ→s, ㅋ→z 등). Ctrl/Alt 없이 입력한 한글은 바꾸지 않습니다. evlog에도 적용해서, 한글 입력 상태에서 Ctrl+C 두 번으로 종료할 수 있습니다.
3. **우클릭이 드래그로 판별됨.** 우클릭할 때 손이 1칸만 움직여도 `DragStart Right`로 처리되었습니다(드래그 시작 기준 1칸). 대응(적용함): 우클릭은 드래그로 판별하지 않고, 뗄 때 누른 위치의 클릭으로 처리합니다(`Detector.RightDrag` 기본값 false). SPEC에서 우클릭의 용도는 컨텍스트 메뉴뿐이기 때문입니다.
4. **iTerm2 우클릭 = 터미널 메뉴.** iTerm2 문서([Pointer](https://iterm2.com/documentation-preferences-pointer.html), [Profiles › Terminal](https://iterm2.com/documentation-preferences-profiles-terminal.html))에 따르면 관련 설정은 다음과 같습니다. 다만 우클릭의 기본 동작이 무엇인지는 문서에 나와 있지 않습니다.
   - Pointer › General › *Ctrl-click reported to apps, does not open menu*: 켜면 Ctrl+클릭이 메뉴 대신 앱으로 전달됩니다.
   - Pointer › Bindings: 우클릭에 *Open Context Menu* 동작이 묶여 있는지 확인하고, 있으면 지우거나 다른 동작으로 바꿉니다.
   - Profiles › Terminal › *Enable mouse reporting*, *Report mouse clicks & drags*: 둘 다 켜져 있어야 합니다. Option을 누르고 있으면 리포팅이 잠시 꺼집니다.
   - **앱 쪽 대응(Phase 1)**: 컨텍스트 메뉴를 여는 방법을 세 가지로 둡니다. (1) 우클릭 (2) **Ctrl+왼쪽 클릭**(맥에서 익숙한 우클릭 대용이고, 위 iTerm2 옵션을 켜면 앱에 전달됨) (3) Shift+F10 또는 메뉴 키. 이렇게 하면 터미널 설정을 바꾸지 않아도 키보드로 메뉴를 열 수 있습니다.
5. 서버 기본 로케일은 `LANG=en_US.UTF-8`이었습니다. 한글 표시(T1)는 서버 환경에서 아직 확인하지 못했습니다.

## 미리 정한 기술적 결정

- **모호폭(East Asian Ambiguous) 문자 폭 = 1칸.** tcell은 `RUNEWIDTH_EASTASIAN` 환경변수가 있을 때만 2칸으로 계산합니다. 반면 go-runewidth를 기본값으로 쓰면 `LANG=ko_KR.*`인 서버에서 2칸으로 계산합니다. 이 차이 때문에 화면이 어긋날 수 있어서, `internal/term`은 tcell과 같은 규칙을 쓰도록 고정했습니다(`text.go`, 테스트 포함). 터미널 쪽 "모호한 문자 2칸" 옵션이 켜져 있으면 T2가 어긋납니다.
- **더블클릭은 시간 기반 판별**: 같은 칸, 같은 버튼, 이전 클릭 후 400ms 이내(`-dblclick`로 조정 가능). 드래그가 끼면 연속 클릭 카운트를 초기화합니다.
