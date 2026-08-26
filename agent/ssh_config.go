package main

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

func sshConfigCommand(args []string) error {
	flags, server, token, err := accountFlags("rc ssh-config", args)
	if err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("usage: rc ssh-config [--url URL]")
	}
	devices, err := listAccountDevices(*server, *token)
	if err != nil {
		return err
	}
	u, err := url.Parse(*server)
	if err != nil || u.Hostname() == "" {
		return errors.New("invalid RC server URL")
	}
	for index, device := range devices {
		if index > 0 {
			fmt.Println()
		}
		fmt.Printf("# %s — %s\n", strings.ReplaceAll(device.Name, "\n", " "), strings.ReplaceAll(device.Workspace, "\n", " "))
		fmt.Printf("Host rc-%s\n", device.ID)
		fmt.Printf("  HostName %s\n", u.Hostname())
		fmt.Println("  User rc")
		fmt.Printf("  HostKeyAlias %s\n", u.Hostname())
		fmt.Printf("  SetEnv RC_DEVICE_ID=%s\n", device.ID)
		fmt.Printf("  ProxyCommand rc ssh-proxy --url %s\n", strings.TrimRight(*server, "/"))
	}
	return nil
}
