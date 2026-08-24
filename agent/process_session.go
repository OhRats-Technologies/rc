package main

import "syscall"

func signalSession(sessionID int, signal syscall.Signal) {
	for _, pid := range sessionProcessIDs(sessionID) {
		_ = syscall.Kill(pid, signal)
	}
}
