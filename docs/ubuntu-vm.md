# Ubuntu 서버 테스트 VM (Lima)

실제 폐쇄망 서버처럼 **일반 사용자로 ssh 접속**해서 tuidesk를 써 보기 위한 VM입니다. 스크립트는 `scripts/ubuntu-vm.sh`입니다.
리눅스 기본 콘솔(TERM=linux) 확인은 이 VM이 아니라 `scripts/linux-console-vm.sh`(QEMU Alpine)로 합니다.

## 왜 컨테이너가 아니라 VM인가

| 항목 | Docker 컨테이너 | 이 VM |
|---|---|---|
| 접속 | `docker exec` (ssh 아님) | 진짜 sshd로 ssh 접속, 맥 터미널의 TERM이 그대로 전달됨 |
| 사용자·권한 | 기본 root | 일반 사용자 `tester`, sudo는 비밀번호를 물음 |
| init·커널 | systemd 없음, 호스트 커널 공유 | Ubuntu 커널과 systemd |
| 로케일 | `LANG` 없음 | en_US.UTF-8(기본), ko_KR.UTF-8 |

## 준비

- Apple Silicon 맥, Lima 2.x: `brew install lima`
- Rosetta: `softwareupdate --install-rosetta` (없으면 arm64 바이너리로 대신 실행)

## 사용법

```bash
scripts/ubuntu-vm.sh up 24.04       # 처음엔 이미지 다운로드 포함 1~2분
scripts/ubuntu-vm.sh ssh 24.04      # tester로 접속
~/bin/tuidesk                       # VM 안에서 실행
```

- 코드를 고친 뒤에는 `scripts/ubuntu-vm.sh deploy 24.04`로 다시 빌드·복사합니다.
- 22.04도 같은 방식입니다(`up 22.04`). 버전별로 VM이 따로 생깁니다(`tuidesk-ubuntu-2204`, `tuidesk-ubuntu-2404`).
- 끝나면 `stop`, 지울 때는 `delete`. 목록은 `list`.

## VM 구성

- 맥 디렉터리를 마운트하지 않습니다(`--mount-none`). 파일은 VM 안에만 있습니다. containerd도 끕니다.
  `--plain`은 쓰지 않습니다. Rosetta도 마운트로 붙기 때문에 `--plain`을 쓰면 Rosetta까지 꺼집니다.
- 사용자
  - `tester` / 비밀번호 `tuidesk`(테스트 전용): sudo가 비밀번호를 물음 → 실제 서버와 같은 sudo 저장 흐름
  - Lima 기본 사용자(맥 사용자 이름): 비밀번호 없는 sudo(NOPASSWD) → `scripts/ubuntu-vm.sh ssh 24.04 <맥 사용자 이름>`
- 바이너리: `~tester/bin/tuidesk`는 Rosetta로 실행하는 **amd64** 바이너리(서버에 올리는 파일과 같은 것)를 가리킵니다. 같은 폴더에 arm64도 있습니다.
- ssh는 `limactl shell`이나 `ssh -F`가 아니라 맥의 기본 ssh 설정으로 접속합니다(`SendEnv LANG LC_*` 포함).
  맥의 `LANG`이 전달되긴 하지만, Ubuntu의 `pam_env`가 `/etc/default/locale`(en_US.UTF-8) 값으로 덮어씁니다. 기본 설치된 Ubuntu 서버에서도 같습니다.
  다른 로케일로 보려면 VM 안에서 `LANG=ko_KR.UTF-8 ~/bin/tuidesk`, `LANG=C ~/bin/tuidesk`처럼 실행합니다.

## 알려진 문제

- **Lima 2.2.0 이미지 변환 멈춤**: 터미널(TTY) 없이 `limactl start`를 실행하면, 이미지를 처음 받은 직후의 qcow2→raw 변환에서 CPU 100%로 멈춥니다(2026-11-06 확인, 40분 넘게 진행 없음). 터미널에서 실행하면 10~20초면 끝납니다. 스크립트는 TTY가 없을 때 `script -q /dev/null`로 감싸서 실행하므로 이 문제를 피합니다. 이미 멈췄다면 `limactl start` 프로세스를 `kill -9`로 끝내고(SIGTERM으로는 안 죽음) `delete` 후 다시 `up` 하세요.

## 확인해 볼 것

- 한글 파일명 만들기·이름 바꾸기·휴지통(`~/.local/share/Trash`)
- 루트 소유 파일(예: `/etc/hosts`) 편집 후 저장 → sudo 비밀번호 입력 → `.bak` 생성
- Terminal.app 마우스·키 동작([compat-matrix.md](compat-matrix.md)의 S 열)
- 로케일을 바꿔 실행했을 때 한글 표시
