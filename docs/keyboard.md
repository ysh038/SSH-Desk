# 키보드·마우스 점검표 (Mac / Windows / 리눅스 콘솔)

tuidesk는 서버에서 실행되므로 **사용자 PC의 터미널 앱이 보내 주는 키만** 받을 수 있습니다. 같은 동작이라도 환경마다 쓸 수 있는 키가 다르기 때문에, 동작마다 여러 키를 함께 연결합니다(별칭). 이 문서는 [ADR 0002](adr/0002-keymap.md)를 보완하는 점검표입니다.

| 환경 | 접속 방법 | 상태 |
|---|---|---|
| **Mac** | macOS Terminal.app → ssh | 1순위 지원 대상 |
| **Windows** | Windows Terminal(Windows 10/11 기본) → ssh | 지원 대상에 추가(2026-11-05). PuTTY는 대상 아님 |
| **리눅스 콘솔** | 서버 모니터, IPMI/KVM | 키보드 전용 |

표기: ✅ 실측 확인 · 🔶 동작할 것으로 예상(검증 필요) · ❌ 안 됨(대체 키 사용)

## 1. 바뀔 수 없는 제약

- **Mac의 Cmd(⌘) 키는 서버로 전달되지 않습니다.** 터미널 앱과 macOS가 Cmd 조합을 먼저 처리하고, 마우스 신호(SGR) 규격에도 Cmd 정보를 담을 자리가 없습니다. 그래서 **Cmd+클릭, Cmd+화살표, Cmd+C 같은 동작은 받을 방법이 없습니다.**
  - 예외는 **Cmd+V**입니다. 터미널이 클립보드 내용을 "붙여넣기"로 보내 주기 때문에 tuidesk가 받을 수 있습니다(아래 2장 참고).
- **Mac의 Ctrl+←/→/↑/↓**는 macOS가 Mission Control과 데스크톱 전환에 씁니다(기본 설정).
- **Mac의 Ctrl+Space**는 한/영 입력 소스 전환입니다.
- **Windows Terminal 기본 단축키**
  - Ctrl+V: 붙여넣기
  - Ctrl+C: 선택 영역이 있을 때만 복사
  - Alt+Enter, F11: 전체 화면
  - Ctrl+Shift+T/W 등: 탭과 창 조작
- **리눅스 콘솔**
  - Shift/Ctrl+방향키, Shift+Tab, Shift+Delete는 수정키가 사라져 그냥 방향키, Tab, Delete로 들어옵니다.
  - Alt+←/→와 Alt+F1~F12는 가상 콘솔 전환 키입니다.

## 2. 지금 동작하는 키 (Phase 1~2)

### 데스크톱
| 동작 | Windows | Mac (Terminal.app) | 콘솔 |
|---|---|---|---|
| 메뉴(런처) | F9 🔶 | fn+F9 🔶 | F9 ✅ |
| 새 창(파일 관리자, `/`) | Ctrl+N 🔶 | Ctrl+N 🔶 | Ctrl+N ✅ |
| 다음/이전 창 | F6 / Shift+F6, Tab / Shift+Tab 🔶 | fn+F6, Tab / Shift+Tab 🔶 | F6 ✅ / Shift+F6(→F16) ✅ |
| 창 메뉴 | Shift+F10, 우클릭 🔶 | fn+Shift+F10, 우클릭 ✅ | Shift+F10(→F20) ✅ |
| 창 닫기 / 종료 | Ctrl+W / Ctrl+Q 🔶 | Ctrl+W / Ctrl+Q 🔶 | ✅ |

### 파일 관리자
| 동작 | Windows | Mac (Terminal.app) | 콘솔 |
|---|---|---|---|
| 여러 개 선택 (추가) | **Ctrl+클릭** 🔶 | **Option+클릭** 또는 Ctrl+클릭 🔶 | Space ✅ |
| 범위 선택 | Shift+클릭, Shift+↑/↓ 🔶 | Shift+클릭, Shift+↑/↓ 🔶 | Space 반복 ✅ |
| 박스 선택 | 드래그 🔶 | 드래그 ✅ | (마우스 없음) |
| 전체 선택 | Ctrl+A 🔶 | Ctrl+A 🔶 | Ctrl+A ✅ |
| 처음/끝으로 | Home/End, Ctrl+Home/End 🔶 | fn+←/→ (Home/End) 🔶 ※ | Home/End ✅ |
| 상위 폴더 | Backspace, Alt+↑ 🔶 | delete(⌫), Option+↑ 🔶 | Backspace ✅ |
| 열기 | Enter, 더블클릭, Alt+↓ 🔶 | return, 더블클릭, Option+↓ 🔶 | Enter ✅ |
| 이름 바꾸기 | F2 🔶 | fn+F2 🔶 | F2 ✅ |
| 휴지통 | Delete 🔶 | fn+delete(⌦) 🔶 | Delete ✅ |
| 영구 삭제 | Shift+Delete 🔶 | fn+Shift+delete 🔶 | 메뉴만 (Shift 사라짐) |
| 복사 / 잘라내기 | Ctrl+C / Ctrl+X 🔶 | Ctrl+C / Ctrl+X 🔶 | ✅ |
| 붙여넣기 | **Ctrl+V** (터미널이 붙여넣기로 보냄 → 파일 붙여넣기로 처리) 🔶 | **Cmd+V** 또는 Ctrl+V 🔶 | Ctrl+V ✅ |
| 권한 | **Ctrl+P** (Alt+Enter는 전체 화면이라 사용 불가) | Ctrl+P, Option+Enter 🔶 | Ctrl+P, Alt+Enter ✅ |
| 숨김 파일 | Alt+. 🔶 | Option+. 🔶 ※※ | Alt+. ✅ |
| 실행취소 | Ctrl+Z 🔶 | Ctrl+Z 🔶 | ✅ |

※ Terminal.app은 기본 설정에서 Home/End/PgUp/PgDn을 **터미널 화면 스크롤**에 쓰고, Shift를 함께 눌러야 앱으로 보낸다고 알려져 있습니다. 실측이 필요합니다.
※※ Option 조합은 Terminal.app의 "Option을 Meta 키로 사용" 설정이 꺼져 있으면 특수문자(예: Option+. → ≥)로 들어옵니다. 실측이 필요합니다.

## 3. 에디터 키 계획 (Phase 3, 실측 후 확정)

Mac 사용자가 기대하는 Cmd 조합은 받을 수 없습니다. 그래서 **Windows 표준 키 + Mac에서 실제로 전달되는 대체 키 + 콘솔용 키**를 모두 연결할 계획입니다.

| 동작 | Windows | Mac에서 쓸 키 (Cmd 대신) | 콘솔 |
|---|---|---|---|
| 줄 처음/끝 | Home / End | fn+←/→ (Home/End), **Ctrl+A / Ctrl+E**(macOS 텍스트 기본 키) | Home / End |
| 문서 처음/끝 | Ctrl+Home / Ctrl+End | fn+Ctrl+←/→ 🔶, **Option+↑/↓** | Ctrl+Home/End 🔶 |
| 단어 이동 | Ctrl+← / Ctrl+→ | **Option+←/→** (Terminal.app이 ESC b/f로 보냄 → Alt+B/F) | Alt+B / Alt+F |
| 선택 | Shift+방향키 | Shift+방향키 | 선택 모드 키(미정, Shift가 사라짐) |
| 전체 선택 | Ctrl+A | Ctrl+A가 줄 처음과 겹침 → 메뉴 또는 별도 키(미정) | |
| 저장 / 찾기 / 실행취소 | Ctrl+S / Ctrl+F / Ctrl+Z | 같음 | 같음 |
| 복사 / 붙여넣기 | Ctrl+C / Ctrl+V (Windows Terminal은 선택 영역이 없을 때만 Ctrl+C를 보냄) | Ctrl+C / Cmd+V | 같음 |

- **Ctrl+A 충돌**: Windows와 SPEC 기준으로는 "전체 선택"이고, macOS 텍스트 기준으로는 "줄 처음"입니다. 실측 결과를 보고 정합니다.
- **Cmd+←/→를 꼭 쓰고 싶다면**: Terminal.app의 프로파일 › 키보드 설정에서 Cmd 조합을 Home/End로 매핑할 수 있는지 확인이 필요합니다. 가능하면 설정 방법을 안내하겠습니다.

## 4. 실측 방법 (`evlog`)

서버에서 `~/sanghoon/evlog -label <이름>`을 실행한 뒤 아래 키를 차례로 누르고, 로그 파일을 넘겨주세요. 아무것도 찍히지 않는 키도 그 자체가 결과입니다.

**Mac (Terminal.app, `-label mac-keys`)**
1. fn+← · fn+→ · fn+↑ · fn+↓ (Home/End/PgUp/PgDn)
2. Shift+fn+← · Shift+fn+→
3. Option+← · Option+→ · Option+↑ · Option+↓ · Option+. · Option+return
4. Shift+← · Shift+→ · Shift+↑ · Shift+↓ · Shift+Tab
5. Ctrl+A · Ctrl+E · Ctrl+P · Ctrl+N · Ctrl+W
6. fn+F2 · fn+F6 · fn+Shift+F6 · fn+F9 · fn+Shift+F10
7. fn+delete(⌦) · fn+Shift+delete
8. Cmd+← · Cmd+→ · Cmd+↑ · Cmd+↓ (아무것도 안 찍히는 것이 예상 결과)
9. 마우스: Ctrl+클릭 · Option+클릭 · Shift+클릭 (각각 로그의 `mod=` 값 확인)

**Windows (Windows Terminal, `-label win-keys`)**: 위 목록을 Windows 키로 똑같이 누르고, 추가로 다음을 누릅니다.
1. Home · End · PgUp · PgDn · Ctrl+Home · Ctrl+End · Ctrl+← · Ctrl+→
2. Ctrl+V (붙여넣기로 찍히는지) · Alt+Enter (전체 화면이 되는지)
3. 마우스: Ctrl+클릭 · Shift+클릭 · 우클릭
