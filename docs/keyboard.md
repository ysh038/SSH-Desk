# 키보드·마우스 점검표 (Mac 배열 / PC 배열 / 리눅스 콘솔)

tuidesk는 서버(리눅스)에서 실행되므로 **사용자 PC의 터미널 앱이 보내 주는 키만** 받을 수 있습니다. 같은 동작이라도 키보드와 환경에 따라 쓸 수 있는 키가 다르기 때문에, 동작마다 여러 키를 함께 연결합니다(별칭). 이 문서는 [ADR 0002](adr/0002-keymap.md)를 보완하는 점검표입니다.

| 환경 | 키보드 | 비고 |
|---|---|---|
| **맥 Terminal.app → ssh** | **Mac 배열** (⌘ Cmd, ⌥ Option, fn) | 1순위 |
| **맥 Terminal.app → ssh** | **PC(Windows) 배열** | macOS가 Windows 키는 Cmd로, Alt는 Option으로 바꿉니다. Home/End/PgUp/PgDn/Delete/Insert와 F키가 따로 있습니다 |
| **리눅스 콘솔** | PC 배열 | 서버에 직접 연결한 키보드, IPMI/KVM. 키보드 전용 |

표기: ✅ 실측 확인 · 🔶 동작할 것으로 예상(검증 필요) · ❌ 안 됨(대체 키 사용)

## 1. 바뀔 수 없는 제약

- **Cmd(⌘) 키는 서버로 전달되지 않습니다.** PC 키보드의 Windows 키도 맥에서는 Cmd이므로 마찬가지입니다.
  - 터미널 앱과 macOS가 Cmd 조합을 먼저 처리하고, 마우스 신호(SGR) 규격에도 Cmd 정보를 담을 자리가 없습니다. 그래서 **Cmd+클릭, Cmd+화살표, Cmd+C 같은 동작은 받을 방법이 없습니다.**
  - 예외는 **Cmd+V**입니다. 터미널이 클립보드 내용을 "붙여넣기"로 보내 주기 때문입니다.
- **macOS가 먼저 가져가는 키**: Ctrl+←/→/↑/↓(Mission Control, 데스크톱 전환), Ctrl+Space(한/영 전환), F11(데스크톱 보기). 키보드 배열과 상관없이 같습니다.
- **Mac 배열 키보드의 F키**는 기본적으로 밝기, 볼륨 키라서 fn과 함께 눌러야 합니다. PC 배열 키보드는 키보드와 macOS 설정에 따라 다릅니다.
- **Mac 배열 키보드**에는 Home/End/PgUp/PgDn/⌦ 키가 따로 없습니다. fn+←/→/↑/↓, fn+delete로 입력합니다.
- **Terminal.app은 기본 설정에서 Shift+↑/↓를 그냥 ↑/↓로 보냅니다.** Shift+←/→는 `ESC[1;2D/C`로 구분해 보내지만 위아래는 구분하지 않아서, 에디터와 파일 관리자에서 Shift+↑/↓ 선택이 커서 이동으로만 동작합니다. 해결 방법은 5장을 봅니다.
- **Option(⌥) 조합**은 Terminal.app의 "Option을 Meta 키로 사용" 설정이 꺼져 있으면 특수문자(예: Option+. → ≥)로 들어옵니다.
- **리눅스 콘솔**
  - Shift/Ctrl+방향키, Shift+Tab, Shift+Delete는 수정키가 사라져 그냥 방향키, Tab, Delete로 들어옵니다.
  - Alt+←/→와 Alt+F1~F12는 가상 콘솔 전환 키입니다.

## 2. 지금 동작하는 키 (Phase 1~2)

### 데스크톱
| 동작 | Mac 배열 | PC 배열 (맥에 연결) | 콘솔 |
|---|---|---|---|
| 메뉴(런처) | fn+F9 🔶 | F9 🔶 | F9 ✅ |
| 새 창(파일 관리자, `/`) | Ctrl+N 🔶 | Ctrl+N 🔶 | Ctrl+N ✅ |
| 다음/이전 창 | fn+F6 / fn+Shift+F6, Tab / Shift+Tab 🔶 | F6 / Shift+F6, Tab / Shift+Tab 🔶 | F6 ✅ / Shift+F6(→F16) ✅ |
| 창 메뉴 | fn+Shift+F10, 우클릭 ✅ | Shift+F10, 우클릭 ✅ | Shift+F10(→F20) ✅ |
| 창 닫기 / 종료 | Ctrl+W / Ctrl+Q 🔶 | Ctrl+W / Ctrl+Q 🔶 | ✅ |

### 파일 관리자
| 동작 | Mac 배열 | PC 배열 (맥에 연결) | 콘솔 |
|---|---|---|---|
| 여러 개 선택(추가) | **Option+클릭**, Ctrl+클릭 🔶 | **Ctrl+클릭**, Alt+클릭 🔶 | Space ✅ |
| 범위 선택 | Shift+클릭, Shift+↑/↓ 🔶 | Shift+클릭, Shift+↑/↓ 🔶 | Space 반복 ✅ |
| 박스 선택 | 드래그 ✅ | 드래그 ✅ | (마우스 없음) |
| 전체 선택 | Ctrl+A 🔶 | Ctrl+A 🔶 | Ctrl+A ✅ |
| 처음/끝으로 | fn+←/→ 🔶 ※ | Home/End, Ctrl+Home/End 🔶 ※ | Home/End ✅ |
| 상위 폴더 | delete(⌫), Option+↑ 🔶 | Backspace, Alt+↑ 🔶 | Backspace ✅ |
| 열기 | return, 더블클릭, Option+↓ 🔶 | Enter, 더블클릭, Alt+↓ 🔶 | Enter ✅ |
| 이름 바꾸기 | fn+F2 🔶 | F2 🔶 | F2 ✅ |
| 휴지통 | fn+delete(⌦) 🔶 | Delete 🔶 | Delete ✅ |
| 영구 삭제 | fn+Shift+delete 🔶 | Shift+Delete 🔶 | 메뉴만 (Shift가 사라짐) |
| 복사 / 잘라내기 | Ctrl+C / Ctrl+X 🔶 | Ctrl+C / Ctrl+X 🔶 | ✅ |
| 붙여넣기 | **Cmd+V**, Ctrl+V 🔶 | **Win+V**(=Cmd+V), Ctrl+V 🔶 | Ctrl+V ✅ |
| 권한 | Ctrl+P, Option+return 🔶 | Ctrl+P, Alt+Enter 🔶 | Ctrl+P, Alt+Enter ✅ |
| 숨김 파일 | Option+. 🔶 | Alt+. 🔶 | Alt+. ✅ |
| 실행취소 | Ctrl+Z 🔶 | Ctrl+Z 🔶 | ✅ |

※ Terminal.app은 기본 설정에서 Home/End/PgUp/PgDn을 **터미널 화면 스크롤**에 쓰고, Shift를 함께 눌러야 앱으로 보낸다고 알려져 있습니다. PC 키보드의 Home/End 키도 같은 영향을 받습니다. 실측이 필요합니다.

## 3. 에디터 키 (Phase 3에서 구현, Mac 배열은 실측 대기)

Cmd(Windows 키) 조합은 받을 수 없으므로 **PC 표준 키 + Mac에서 실제로 전달되는 대체 키 + 콘솔용 키**를 모두 연결했습니다. 메뉴, 도움말, 힌트 줄은 설정한 키보드 표기로 보여 줍니다(ADR 0003).

| 동작 | PC 배열 | Mac 배열 (Cmd 대신) | 콘솔 |
|---|---|---|---|
| 줄 처음/끝 | Home / End | fn+←/→ | Home / End |
| 문서 처음/끝 | Ctrl+Home / Ctrl+End 🔶 | **Option+↑/↓** | Ctrl+Home/End 🔶 |
| 단어 이동 | Alt+←/→ (맥에서는 Alt=Option) | **Option+←/→** (Terminal.app이 ESC b/f로 보냄 → Alt+B/F) | Alt+B / Alt+F |
| 선택 | Shift+방향키 | Shift+방향키 | **Ctrl+Space**로 선택 시작 후 이동 |
| 전체 선택 | Ctrl+A | ⌃A (규칙 B: 명령은 ⌃) | Ctrl+A |
| 저장 / 찾기 / 실행취소 | Ctrl+S / Ctrl+F / Ctrl+Z | 같음 | 같음 |
| 복사 / 붙여넣기 | Ctrl+C / Ctrl+V, Win+V | Ctrl+C / Cmd+V | 같음 |

- **PC 배열이라도 맥에 연결하면** Ctrl+←/→는 macOS가 가져가므로, 단어 이동은 Alt(=Option)+←/→로 합니다.
- **Ctrl+A**: "명령은 ⌃" 규칙(ADR 0003 B)에 따라 전체 선택으로 정했습니다. 줄 처음은 fn←(Home)입니다.
- **Cmd+←/→를 꼭 쓰고 싶다면**: Terminal.app의 프로파일 › 키보드 설정에서 Cmd 조합을 Home/End로 매핑할 수 있는지 확인이 필요합니다. 가능하면 설정 방법을 안내하겠습니다.

## 4. 실측 방법 (`evlog`)

서버에서 `~/sanghoon/evlog -label <이름>`을 실행한 뒤 아래 키를 차례로 누르고, 로그 파일을 넘겨주세요. 아무것도 찍히지 않는 키도 그 자체가 결과입니다. 키보드별로 한 번씩, 각각 `-label mac-kbd`와 `-label pc-kbd`로 실행합니다.

1. Home · End · PgUp · PgDn: Mac 배열은 fn+←/→/↑/↓
2. Shift+Home · Shift+End: Mac 배열은 Shift+fn+←/→
3. Option(Alt)+← · →, Option+↑ · ↓, Option+. , Option+return(Enter)
4. Shift+← · → · ↑ · ↓, Shift+Tab
5. Ctrl+A · Ctrl+E · Ctrl+P · Ctrl+N · Ctrl+W · Ctrl+Home · Ctrl+End
6. F2 · F6 · Shift+F6 · F9 · Shift+F10: Mac 배열은 fn과 함께
7. Delete(⌦) · Shift+Delete: Mac 배열은 fn+delete
8. Cmd(Win)+← · → · ↑ · ↓: 아무것도 안 찍히는 것이 예상 결과
9. 마우스: Ctrl+클릭 · Option(Alt)+클릭 · Shift+클릭. 로그의 `mod=` 값을 확인합니다.

## 5. Terminal.app 설정: Shift+↑/↓ 보내기

맥에서 한 번만 설정하면 됩니다. 서버 쪽 tuidesk는 이미 `ESC[1;2A/B`를 Shift+↑/↓로 처리합니다.

1. Terminal › 설정(⌘,) › 프로파일 › 사용하는 프로파일(기본은 Basic) › **키보드** 탭
2. 아래 **+** → 키: **↑ 커서 위로**, 수정키: **Shift**, 동작: **텍스트 보내기**
3. 텍스트 칸에서 **Esc 키를 한 번 누르고**(`\033`으로 표시됨) 이어서 `[1;2A`를 입력 → 확인
4. 같은 방법으로 **↓ 커서 아래로 + Shift** → `\033[1;2B`
5. 이미 열린 창에도 바로 적용됩니다. 에디터에서 Shift+↓를 누르면 커서 위치부터 아랫줄 같은 열까지 선택됩니다.

설정하지 않아도 쓸 수 있는 대체 방법: 메뉴 Edit › **Start / stop selecting**(F10 또는 ⌃K에서 "select" 검색)을 켠 뒤 방향키로 이동하면 선택됩니다. ⌃Space는 macOS가 입력 소스 전환에 쓰므로 맥에서는 메뉴로 켭니다.
