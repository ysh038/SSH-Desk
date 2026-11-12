//go:build linux

package devices

import (
	"bytes"
	"context"

	"golang.org/x/sys/unix"
)

// Watch blocks until ctx is cancelled and calls onChange (from its own
// goroutine) whenever a block device is added, removed or changed, or the
// mount table changes (ADR 0005 §3). It sleeps in poll(2): no timer, no
// busy loop. Events come in bursts; the caller debounces.
//
// Two kernel sources are used, both local and unprivileged:
//   - a NETLINK_KOBJECT_UEVENT socket (kernel events and udev's processed
//     events, so labels are already known when lsblk runs)
//   - /proc/self/mountinfo, which poll reports as changed on every mount
//     and unmount
//
// It returns an error right away when neither can be opened (some
// containers); F5 still refreshes by hand.
func Watch(ctx context.Context, onChange func()) error {
	nl, nlErr := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC|unix.SOCK_NONBLOCK, unix.NETLINK_KOBJECT_UEVENT)
	if nlErr == nil {
		// Group 1: kernel, group 2: udev.
		if nlErr = unix.Bind(nl, &unix.SockaddrNetlink{Family: unix.AF_NETLINK, Groups: 1 | 2}); nlErr != nil {
			unix.Close(nl)
		}
	}
	if nlErr != nil {
		nl = -1
	}
	mi, miErr := unix.Open("/proc/self/mountinfo", unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if miErr != nil {
		mi = -1
	}
	if nl < 0 && mi < 0 {
		return nlErr
	}
	var wake [2]int
	if err := unix.Pipe2(wake[:], unix.O_CLOEXEC|unix.O_NONBLOCK); err != nil {
		closeFD(nl)
		closeFD(mi)
		return err
	}
	defer func() {
		closeFD(nl)
		closeFD(mi)
		unix.Close(wake[0])
		unix.Close(wake[1])
	}()
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			_, _ = unix.Write(wake[1], []byte{1})
		case <-stop:
		}
	}()

	// A negative fd is ignored by poll.
	fds := []unix.PollFd{
		{Fd: int32(wake[0]), Events: unix.POLLIN},
		{Fd: int32(nl), Events: unix.POLLIN},
		{Fd: int32(mi), Events: unix.POLLPRI},
	}
	buf := make([]byte, 64<<10)
	for {
		for i := range fds {
			fds[i].Revents = 0
		}
		if _, err := unix.Poll(fds, -1); err != nil {
			if err == unix.EINTR {
				continue
			}
			return err
		}
		if fds[0].Revents != 0 || ctx.Err() != nil {
			return nil
		}
		changed := false
		if fds[1].Revents&unix.POLLIN != 0 {
			for {
				n, _, err := unix.Recvfrom(nl, buf, 0)
				if err != nil || n <= 0 {
					break // EAGAIN: drained
				}
				if bytes.Contains(buf[:n], []byte("SUBSYSTEM=block")) {
					changed = true
				}
			}
		} else if fds[1].Revents != 0 {
			fds[1].Fd = -1 // socket error: stop watching it
		}
		if fds[2].Revents&(unix.POLLPRI|unix.POLLERR) != 0 {
			changed = true
		}
		if changed {
			onChange()
		}
	}
}

func closeFD(fd int) {
	if fd >= 0 {
		unix.Close(fd)
	}
}
