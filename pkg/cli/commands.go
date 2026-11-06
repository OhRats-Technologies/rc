package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/OhRats-Technologies/rc/pkg/kernel"
	"github.com/OhRats-Technologies/rc/pkg/platform"
)

func runCommands(componentDir string) error {
	dir := componentDir
	if dir == "" {
		var err error
		dir, err = platform.ComponentDir()
		if err != nil {
			return err
		}
	}
	engine, err := kernel.NewEngine()
	if err != nil {
		return err
	}
	defer engine.Close()
	commands, err := engine.Commands(dir)
	if err != nil {
		return err
	}
	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, command := range commands {
		usage := command.Usage
		if usage == "" {
			usage = "rc " + command.Name
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", command.Name, command.Summary, usage, command.ComponentID)
	}
	return writer.Flush()
}
