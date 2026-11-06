package webshell

import (
	"errors"
	"fmt"
)

// Invoke dispatches the shell component commands.
func Invoke(command string, args []string) (uint32, error) {
	if command == "webui-config" {
		return InvokeConfig(args)
	}
	if command != "ui-pages" || len(args) != 0 {
		return 0, errors.New("usage: rc ui-pages")
	}
	for _, page := range Pages() {
		fmt.Printf("%s\t%s\t%s\n", page.Id, page.Path, page.Title)
		fmt.Printf("  %s\n", page.Summary)
	}
	return 0, nil
}
