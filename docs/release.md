# 폐쇄망 배포 패키지 (SPEC 11)

## 만들기

```bash
git tag v0.1.0        # 선택: 태그가 없으면 커밋 해시가 버전이 됨(예: 7cd4347, 수정 중이면 -dirty)
make package          # = scripts/package.sh
```

`dist/release/`에 생기는 파일:

| 파일 | 내용 |
|---|---|
| `tuidesk-<버전>-linux-amd64.tar.gz` | 서버용(대부분 이것) |
| `tuidesk-<버전>-linux-arm64.tar.gz` | ARM 서버용 |
| `tuidesk-<버전>-src.tar.gz` | 소스 + `vendor/`(폐쇄망 안에서 직접 빌드할 때) |
| `SHA256SUMS` | 위 세 압축 파일의 체크섬 |

압축 파일 하나의 구성: `tuidesk`(정적 바이너리, lite 빌드), `install.sh`, `rollback.sh`, `README.txt`(영어, 서버 콘솔용), `SHA256SUMS`(안의 파일들), `LICENSE`, `THIRD_PARTY_NOTICES.md`, `examples/apps/*.toml`.
스니펫 같은 기본 데이터는 바이너리에 내장되어 있어 따로 넣지 않습니다.

`package.sh`가 하는 일:
- `make check-lite`로 lite 빌드에 네트워크 코드가 없는지 확인(CLAUDE.md 규칙 3)
- `-X main.version=<버전>`으로 빌드 → `tuidesk --version`에 표시
- 맥 tar의 메타데이터(`._` 파일, xattr)를 빼고 소유자를 root:root로 묶음 → 서버의 GNU tar가 경고 없이 풂
- 커밋하지 않은 변경이 있으면 경고: 바이너리에는 들어가지만 소스 tarball(HEAD 기준)에는 안 들어감

## 서버에 설치

```bash
sha256sum -c SHA256SUMS --ignore-missing     # 반입한 압축 파일 확인
tar xzf tuidesk-<버전>-linux-amd64.tar.gz
cd tuidesk-<버전>-linux-amd64
./install.sh                  # 나만: ~/.local/bin/tuidesk
sudo ./install.sh --system    # 모두: /usr/local/bin/tuidesk
```

`install.sh`:
- 기계 종류(`uname -m`)가 패키지와 다르면 거부
- 안의 `SHA256SUMS`를 검사하고, 바이너리가 실제로 실행되는지(`--version`) 확인한 뒤 설치
- 임시 파일에 복사한 뒤 이름을 바꿔 덮어씀 → 실행 중인 tuidesk가 있어도 안전, 복사가 실패해도 깨진 바이너리가 남지 않음
- 사용자 설치면 `~/.config/tuidesk/{apps,snippets}` 폴더만 만듦. `config.toml`은 만들거나 덮어쓰지 않음(첫 실행 때 tuidesk가 만듦)
- root로 실행하면 기본이 `--system`. 시스템 설치는 홈 폴더를 건드리지 않음
- `--uninstall`: 바이너리만 지우고 설정은 남김(설치 기록도 지움)

## 설치 되돌리기 (rollback.sh)

```bash
./rollback.sh                 # ./install.sh 로 설치했을 때
sudo ./rollback.sh --system   # sudo ./install.sh --system 으로 설치했을 때
```

- **업그레이드였으면** 바뀌기 전 바이너리로 복원, **첫 설치였으면** 바이너리와 설치가 만든 빈 폴더를 지움
- `config.toml` 등 설정은 어느 경우에도 건드리지 않음(첫 설치를 되돌려도 tuidesk를 한 번 실행해 설정이 생겼으면 그 폴더는 남김)
- 한 단계만 되돌림. 같은 버전을 다시 설치해도 직전 버전 백업은 유지
- 설치 기록 위치: 사용자 `~/.local/state/tuidesk/`, 시스템 `/var/lib/tuidesk/`, `--prefix DIR`이면 `DIR/lib/tuidesk/`
  (`install.state` + 바뀌기 전 바이너리 `previous`). 패키지 폴더와 무관하므로 어느 버전 패키지의 `rollback.sh`로도 됨
- `rollback.sh`는 `install.sh --rollback`을 부르는 얇은 래퍼(옵션 해석을 한 곳에서 하기 위해)
- `~/.local/bin`이 PATH에 없으면 안내(Ubuntu는 이 폴더가 있으면 다음 로그인부터 PATH에 넣음)

## 폐쇄망 안에서 소스로 빌드

```bash
tar xzf tuidesk-<버전>-src.tar.gz && cd tuidesk-<버전>-src
make tuidesk-linux            # vendor/를 쓰므로 모듈 프록시 불필요
```

Go는 `go.mod`의 `go` 버전(현재 1.24.2) 이상이 필요합니다. 그보다 낮으면 Go가 새 툴체인을 내려받으려다 실패합니다.
버전은 tarball의 `VERSION` 파일에서 읽습니다.

## 검증 (2026-11-06)

Ubuntu 22.04.5 / 24.04 VM(`scripts/ubuntu-vm.sh`, [ubuntu-vm.md](ubuntu-vm.md))에서 `tester` 사용자로 확인:
체크섬, 압축 풀기 경고 없음, 다른 아키텍처 거부, 사용자 설치·로그인 셸 PATH, 재설치 시 `config.toml` 유지, 손상 파일 거부, sudo 없는 `--system` 실패 안내, `sudo --system` 설치(root 소유), 제거.
되돌리기: 새 사용자로 첫 설치→되돌리기(폴더까지 원상태), v1→v2→되돌리기(v1 복원, 설정 유지), 같은 버전 재설치 후 되돌리기, 제거 후 되돌리기 없음, 시스템 설치 되돌리기(sudo 없으면 거부).
VM은 arm64라 arm64 패키지로 설치했고, amd64 패키지는 거부되는 것을 확인했습니다. 소스 tarball은 빈 모듈 캐시 + `GOPROXY=off`로 빌드되는 것을 확인했습니다.

## 아직 없는 것

- `tuidesk doctor`(TERM, 색상 수, 마우스, 한글 폭, OSC52 진단) — SPEC 11에 있지만 별도 작업
- full 빌드 패키지(AI/MCP, Phase 5~6 이후)
