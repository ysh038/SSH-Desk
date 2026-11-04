# Phase 0 터미널 호환성 확인 체크리스트

`evlog`는 터미널이 보내는 키·마우스·붙여넣기·포커스 이벤트를 화면과 로그 파일에 그대로 찍어 주는 확인용 프로그램입니다.
지원 대상은 두 가지입니다(SPEC 9.0). ① 맥 **Terminal.app에서 ssh로 Ubuntu 서버에 접속한 상태**, ② 서버의 **리눅스 기본 콘솔**. 아래 항목을 확인하고 결과를 [compat-matrix.md](compat-matrix.md)에 채웁니다. 다른 터미널은 참고용입니다.

## 0. 준비

```bash
# 맥(프로젝트 폴더)에서 빌드
make evlog-linux
```

서버 아키텍처를 확인한 뒤 맞는 바이너리를 복사합니다(`x86_64` → amd64, `aarch64` → arm64).

```bash
ssh <서버> uname -m
```

```bash
scp dist/evlog-linux-amd64 <서버>:~/evlog
```

서버에서 실행합니다. `-label`은 로그 파일 이름에 들어갈 터미널 이름입니다.

```bash
chmod +x ~/evlog && ~/evlog -label iterm2
```

- 종료: `F10`, 또는 `Ctrl+C`를 1.5초 안에 두 번
- 로그는 실행한 폴더에 `evlog-<label>-<시각>.log`로 저장됩니다.
- 기본 마우스 모드는 `drag(1002)`입니다. 실행 중 `F4`로 off → button(1000) → drag(1002) → motion(1003) 순서로 바뀝니다. 시작 모드는 `-mouse=motion`처럼 지정할 수도 있습니다.
- 맥 키보드에서 F키가 밝기·볼륨으로 동작하면 `fn`을 같이 누르세요(F2 → `fn+F2`).

### 화면 구성
- 1줄: 터미널 이름, TERM, 색 수, 크기, 마우스 모드
- 3줄: 현재 포인터 좌표와 버튼, 마지막으로 판별된 제스처(클릭/더블클릭/드래그/휠), 마지막 붙여넣기
- 초록 박스: 한글 폭 테스트(오른쪽 세로선이 모두 같은 열이어야 정상)
- 아래: 이벤트 로그(최신이 아래). `⇒` 줄은 앱이 해석한 제스처입니다.
- 마우스가 있다고 보고된 칸은 반전 표시됩니다(좌표가 실제 포인터와 맞는지 확인용).

## 1. 터미널별 미리 확인할 설정

설정 이름은 버전에 따라 다를 수 있습니다. **기본값 그대로 먼저 테스트**하고, 안 되는 항목만 설정을 바꿔 다시 테스트한 뒤 무엇을 바꿨는지 매트릭스의 비고란에 적어 주세요.

| 터미널 | 볼 곳 |
|---|---|
| iTerm2 | Profiles › Terminal › *Enable mouse reporting* · Profiles › Keys › *Left/Right Option key* (Esc+로 해야 Alt 전달) · General › Selection › *Applications in terminal may access clipboard*(OSC52) · Profiles › Text › *ambiguous characters are double-width*(꺼져 있어야 함) · Option+드래그는 앱 대신 iTerm2 선택이 됨 |
| Warp | Settings에서 "mouse", "clipboard", "option/meta" 검색 · Warp의 ssh 래퍼(Warpify)를 쓰는 경우와 일반 ssh를 따로 확인 |
| Ghostty | `~/.config/ghostty/config`: `mouse-shift-capture`(Shift+클릭을 앱에 줄지), `clipboard-write`/`clipboard-read`, `macos-option-as-alt` · TERM이 `xterm-ghostty`로 넘어감(서버에 terminfo가 없어도 evlog는 내장 정의 사용) |
| Terminal.app | 보기 메뉴 › *마우스 보고 허용*(Cmd+R) · 설정 › 프로파일 › 키보드 › *Option을 Meta 키로 사용* |

## 2. 확인 항목

각 항목을 해 보고 로그에 기대한 줄이 찍히는지 봅니다. 결과는 ✅ 정상 / ⚠️ 조건부(설정 변경 필요, 일부만 됨) / ❌ 안 됨 으로 기록합니다.

### 마우스
| ID | 할 일 | 정상일 때 로그 | 실패 모습 |
|---|---|---|---|
| M1 | 아무 곳이나 왼쪽 클릭 | `⇒ Click single Left (x,y)` | 로그 없음 |
| M2 | 같은 자리 빠르게 2번, 3번 클릭 | `Click double`, `Click triple` | single만 반복 |
| M3 | 우클릭 | `⇒ Click single Right` | 터미널 자체 메뉴가 뜸 |
| M4 | 왼쪽 버튼 누른 채 끌기 | `DragStart` → `[드래그 중] (연속 N회)` → `DragEnd` | 터미널이 글자 선택(파란 영역)만 함 |
| M5 | 휠 위/아래, 트랙패드 가로 스크롤 | `Wheel Up/Down/Left/Right` | 터미널 스크롤백이 움직임 |
| M6 | Shift+클릭, Option+클릭, Ctrl+클릭, Cmd+클릭 | `mod=Shift` 등 | mod가 `-`이거나, 클릭이 안 옴, Ctrl+클릭이 우클릭으로 바뀜 |
| M7 | F4로 `motion(1003)`으로 바꾼 뒤 버튼 없이 움직이기 | 3줄의 포인터 좌표가 따라 바뀜 | 좌표 고정 |
| M8 | 박스 안 한글 글자의 왼쪽 절반/오른쪽 절반 클릭 | 반전 칸이 실제 포인터 위치와 일치 | 한 칸씩 밀림 |
| M9 | 가운데 버튼(있다면) | `Click single Middle` | — |

### 키보드
| ID | 할 일 | 정상일 때 로그 |
|---|---|---|
| K1 | Ctrl+S, Ctrl+Q, Ctrl+Z, Ctrl+Y, Ctrl+F, Ctrl+H, Ctrl+Space | `KEY Ctrl+S` 등. Ctrl+Space는 `Ctrl+Space` 또는 `Ctrl+@` |
| K2 | Option(Alt)+a, Option+←/→ | `mod=Alt` |
| K3 | F1~F12, Shift+F10 | `KEY F1` … (fn 필요 여부 비고에) |
| K4 | Shift+방향키, Ctrl+←/→, Home/End, PgUp/PgDn | `Shift+Left`, `Ctrl+Right`, `Home` … |
| K5 | Esc 한 번 | `KEY Esc`가 바로 찍힘(지연 체감 비고) |
| K6 | Cmd+S, Cmd+Z | 아무것도 안 오는 게 정상(Cmd는 원격으로 전달되지 않음). 뭔가 오면 기록 |
| K7 | 한글 입력기로 "한글" 타이핑 | `Rune[한]`, `Rune[글]`(조합 중 자모가 따로 오면 기록) |

### 글자 폭
| ID | 할 일 | 정상 |
|---|---|---|
| T1 | 박스 1~3줄(한글/영문 섞인 줄) | 오른쪽 `│`가 모두 같은 열 |
| T2 | 박스 4줄(모호폭: `…│※①○`) | 같은 열(어긋나면 터미널의 "모호한 문자 2칸" 설정 확인) |
| T3 | 박스 5줄(이모지) | 참고용. 어긋나도 기록만 |

### 클립보드·붙여넣기
| ID | 할 일 | 정상 |
|---|---|---|
| C1 | F2 누른 뒤 맥의 메모장 등에 Cmd+V | `tuidesk OSC52 테스트 HH:MM:SS`가 붙여넣어짐 |
| C2 | F5 (OSC52 읽기) | `CLIP OSC52 읽기 응답`이 오면 지원. 대부분 미지원/확인창이 정상 |
| P1 | 여러 줄 텍스트를 복사해 evlog 창에서 Cmd+V | `PASTE ▶ 시작` … `■ 끝: N줄`이 원문 줄 수와 일치 |
| P2 | 한글이 섞인 여러 줄 붙여넣기 | 로그 파일의 `paste begin/end` 사이 내용이 원문과 같음 |

### 기타
| ID | 할 일 | 정상 |
|---|---|---|
| E1 | 다른 앱을 눌렀다가 다시 터미널로 | `FOCUS 창 포커스 잃음/얻음` |
| E2 | 터미널 창 크기 변경 | `RESIZE WxH`, 화면 다시 그려짐 |
| E3 | (선택) 서버에서 `tmux` 안에서 같은 테스트 | 매트릭스의 tmux 칸에 기록. OSC52는 tmux `set-clipboard on` 필요할 수 있음 |

## 2-1. 리눅스 콘솔에서 확인 (키보드 전용)

서버에 모니터와 키보드를 직접 연결하거나 IPMI/KVM 원격 콘솔로 로그인한 뒤 실행합니다.

```bash
cd ~/sanghoon && ./evlog -label linux-console -mouse=off
```

| ID | 할 일 | 정상 |
|---|---|---|
| L1 | 화면 전체 | 상단 바, 박스, 로그 영역이 무너지지 않음. 박스 선(─│┌┐└┘)이 선으로 보임 |
| L2 | 박스 안 한글 | 깨지거나 ◆로 보이는 것이 예상 결과. 다른 줄까지 밀리는지만 기록 |
| L3 | K1~K5 키 항목 | 맥 터미널과 같은 이름으로 찍히는지. 특히 Shift+방향키, Ctrl+방향키, Ctrl+Space, F1~F12 |
| L4 | 헤더의 색 수 | 로그 파일 헤더의 `colors=` 값(8 또는 16 예상) |

리눅스 콘솔에는 한글 입력기가 없으므로 K7은 건너뜁니다.

## 3. 결과 전달

1. [compat-matrix.md](compat-matrix.md)의 표를 채웁니다(모르겠으면 ❓).
2. 서버의 로그 파일을 가져와 `docs/compat-logs/`에 넣습니다.

```bash
mkdir -p docs/compat-logs && scp '<서버>:~/evlog-*.log' docs/compat-logs/
```

3. Claude Code에 "compat 결과 정리했어"라고 말하면, 로그를 읽어 매트릭스의 빈칸을 보완하고 ADR(UI 프레임워크 결정)과 다음 단계(겹치는 창 드래그 데모)로 넘어갑니다.
