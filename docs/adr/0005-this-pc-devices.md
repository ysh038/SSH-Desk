# ADR 0005: This PC: 장치 목록, 마운트 방식, 핫플러그 감지

- 상태: **채택** (2026-11-12). 3번(핫플러그)은 구현하면서 실제 서버에서 확인한 뒤 확정합니다.
- 관련: SPEC 7.8, 10(Phase 5), 7.2.1(sudo 비밀번호 규칙)

## 1. 장치 목록 수집: `lsblk --json`

| 방식 | 장점 | 단점 |
|---|---|---|
| **`lsblk --json --bytes`** | util-linux라 Ubuntu에 항상 있음. 라벨·파일시스템·이동식 여부·모델을 root 없이 한 번에 줌 | 외부 명령 실행. 버전마다 열이 조금 다름 |
| `/sys/block` + `/proc/self/mountinfo` 직접 읽기 | 외부 명령 없음 | 마운트 안 된 장치의 파일시스템·라벨은 udev DB(`/run/udev/data`)를 따로 읽어야 함. lsblk를 다시 만드는 셈 |
| udisks2 D-Bus | 이벤트까지 한 번에 | 서버에 없는 경우가 많음. D-Bus 의존성 추가 |

**결정: lsblk.** 열은 `NAME,PATH,TYPE,SIZE,FSTYPE,LABEL,UUID,PARTUUID,MOUNTPOINTS,RM,HOTPLUG,TRAN,MODEL,VENDOR,RO`를 요청합니다.

- `MOUNTPOINTS` 열은 util-linux 2.37(Ubuntu 22.04)부터 있습니다. 실패하면 `MOUNTPOINT`로 다시 실행합니다(20.04).
- 구버전은 숫자·불리언을 문자열(`"1"`, `"0"`)로 내보내므로 파서는 두 형태를 모두 받습니다.
- 사용량은 lsblk가 아니라 `statfs`로 구합니다(리눅스 전용 파일, CGO 없음).
- NFS/CIFS 같은 네트워크 마운트는 블록 장치가 아니라 lsblk에 없으므로 `/proc/self/mountinfo`에서 읽습니다.
- `/etc/fstab`에 있는 항목과 `/`, `/boot*`, 스왑은 "보호됨"으로 표시해 Unmount/Eject를 내놓지 않습니다. 같은 디스크에 보호된 파티션이 하나라도 있으면 그 디스크는 Eject할 수 없습니다.
- 파싱·분류는 `internal/devices`의 순수 함수이고 픽스처로 테스트합니다. 명령 실행은 주입한 실행기(runner)로 하므로 테스트에서 가짜로 바꿉니다.

## 2. 마운트 방식: `sudo mount` 한 가지

| 방식 | 평가 |
|---|---|
| `udisksctl mount` | root가 필요 없고 마운트 위치도 알아서 정하지만, 서버(특히 최소 설치)에 udisks2가 없는 경우가 많고 ssh 세션은 polkit에서 "활성 세션"이 아니라 거부되는 일이 흔합니다 |
| **`sudo mount`** | 지금 손으로 하는 방식 그대로. sudo만 있으면 됨. 에디터의 "sudo로 저장"과 같은 비밀번호 흐름을 씀 |
| 둘 다(udisksctl 먼저, sudo로 폴백) | 경로가 둘이라 테스트와 에러 처리가 두 배. 실제 서버에서는 대부분 sudo로 떨어질 것 |

**결정: `sudo mount`만 구현합니다.** 경로가 하나라 동작을 예측하기 쉽고, 사용자가 이미 아는 명령과 같습니다. udisks2가 필요한 환경이 나오면 그때 추가합니다.

- 마운트 위치: `/media/<user>/<라벨 | UUID | 장치 이름>`. 이름은 `/`, 제어 문자, 앞쪽 `.`을 빼고 100바이트로 자릅니다. 겹치면 `name (2)`.
- 옵션: 항상 `nosuid,nodev`. vfat/exfat/ntfs/ntfs3은 `uid=,gid=`, vfat은 추가로 `utf8`(한글 파일명). 읽기 전용이면 `ro`.
- 한 번의 `sudo sh -c`로 `mkdir -p` → `mount`를 실행하고, 마운트가 실패하면 방금 만든 폴더를 `rmdir`합니다.
- 언마운트는 `umount` 후 `/media/<user>/` 아래 폴더만 `rmdir`합니다(다른 위치의 폴더는 건드리지 않음). `-f`, `-l`(강제/지연 언마운트)은 쓰지 않습니다.
- Eject는 디스크의 모든 파티션을 언마운트한 뒤 `/sys/block/<disk>/device/delete`에 `1`을 써서 전원을 끕니다.
- 비밀번호는 SPEC 7.2.1 규칙대로 처리합니다. sudo 인자·입력을 만드는 코드는 에디터에서 `internal/sudo`로 옮겨 함께 씁니다.

## 3. 핫플러그 감지: 커널 uevent (netlink)

| 방식 | 평가 |
|---|---|
| 고정 주기로 lsblk 실행 | 간단하지만 CLAUDE.md 규칙 9(바쁜 루프·고정 주기 금지)에 어긋나고 유휴 CPU 0 목표를 깹니다 |
| `/proc/self/mountinfo`를 `poll(POLLPRI)` | 마운트 변화만 알 수 있고, 꽂았지만 아직 마운트 안 한 USB는 모릅니다 |
| **`NETLINK_KOBJECT_UEVENT` 소켓** | 커널이 block 장치 add/remove를 알려 줌. root 불필요, 표준 `syscall`만 사용(CGO 없음) |

**결정(잠정): netlink uevent.** This PC 창이 열려 있거나 데스크톱이 떠 있는 동안 고루틴 하나가 소켓에서 블록하고, `SUBSYSTEM=block` 이벤트가 오면 0.5초 모아서 목록을 한 번 다시 읽습니다. 마운트 변화는 `mountinfo` 알림을 함께 봅니다.

- 이 소켓은 커널과의 로컬 통신이고 외부 네트워크가 아닙니다. lite 빌드의 "네트워크 코드 없음" 규칙(`net/http` 클라이언트 금지)에 해당하지 않습니다.
- 소켓을 열 수 없는 환경(일부 컨테이너)에서는 조용히 포기하고 F5 수동 새로고침만 씁니다.
- 실제 서버와 QEMU USB 장치로 확인한 뒤 이 항목의 "잠정"을 뗍니다.
