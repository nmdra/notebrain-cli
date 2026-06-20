//go:build !linux && !darwin

package tui

import "fmt"

func dupFd(fd int) (int, error)   { return 0, fmt.Errorf("unsupported platform") }
func dup2(oldfd, newfd int) error { return fmt.Errorf("unsupported platform") }
func closeFd(fd int) error        { return fmt.Errorf("unsupported platform") }
