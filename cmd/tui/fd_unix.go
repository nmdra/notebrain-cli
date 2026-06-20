//go:build linux || darwin

package tui

import "golang.org/x/sys/unix"

func dupFd(fd int) (int, error)   { return unix.Dup(fd) }
func dup2(oldfd, newfd int) error { return unix.Dup2(oldfd, newfd) }
func closeFd(fd int) error        { return unix.Close(fd) }
