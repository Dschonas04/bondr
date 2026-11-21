//go:build !windows

package main

import "syscall"

func lebt(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
