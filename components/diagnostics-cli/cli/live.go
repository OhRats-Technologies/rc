package cli

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"wit_component/ohrats_rc_local_service_host"
)

const service = "rc.service"

func liveLogs(args []string) (uint32, error) {
	usage := "usage: rc logs [--service] [--follow] [1..100]"
	follow := false
	limit := uint32(20)
	numeric := false
	for _, argument := range args {
		switch argument {
		case "--follow", "-f":
			follow = true
			continue
		case "--service":
			continue
		}
		if numeric {
			return 0, errors.New(usage)
		}
		parsed, err := strconv.ParseUint(argument, 10, 32)
		if err != nil {
			return 0, errors.New(usage)
		}
		limit = uint32(parsed)
		numeric = true
	}
	if limit < 1 || limit > 100 {
		return 0, errors.New("log limit must be between 1 and 100")
	}
	cursor := ""
	for {
		result := ohrats_rc_local_service_host.Logs(service, cursor, limit)
		if result.IsErr() {
			return 0, errors.New(result.Err())
		}
		entries := result.Ok()
		full := len(entries) == int(limit)
		for _, entry := range entries {
			cursor = entry.Cursor
			fmt.Printf("%s %s\n", journalTimestamp(entry.Timestamp), escaped(entry.Message))
		}
		if !follow {
			return 0, nil
		}
		if !full {
			ohrats_rc_local_service_host.Wait(1000)
		}
	}
}

// Processes inspects live processes in the local RC user service.
func Processes(args []string) (uint32, error) {
	for _, argument := range args {
		if argument != "--watch" && argument != "--commands" {
			return 0, errors.New("usage: rc ps [--watch] [--commands]")
		}
	}
	watch := containsArgument(args, "--watch")
	commands := containsArgument(args, "--commands")
	for {
		result := ohrats_rc_local_service_host.Processes(service, commands)
		if result.IsErr() {
			return 0, errors.New(result.Err())
		}
		processes := result.Ok()
		if watch && isTerminal(os.Stdout) {
			fmt.Print("\x1b[H\x1b[2J")
		}
		fmt.Print("PID\tPROCESS")
		if commands {
			fmt.Print("\tARGV (local, live)")
		}
		fmt.Println()
		for _, process := range processes {
			fmt.Printf("%d\t%s", process.Pid, escaped(process.Name))
			if commands {
				fmt.Printf("\t%s", debugStrings(process.Argv))
				if process.Truncated {
					fmt.Print(" [truncated]")
				}
			}
			fmt.Println()
		}
		fmt.Printf("%d processes in the RC user service.", len(processes))
		if watch {
			fmt.Print(" Ctrl-C stops watching.")
		}
		fmt.Println()
		if !watch {
			return 0, nil
		}
		ohrats_rc_local_service_host.Wait(2000)
	}
}

func containsArgument(args []string, expected string) bool {
	for _, argument := range args {
		if argument == expected {
			return true
		}
	}
	return false
}

// isTerminal reports whether the file is attached to a character device. The
// WASI ABI exposes no terminal ioctl, so redirected character devices such as
// /dev/null are indistinguishable from a terminal here.
func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
