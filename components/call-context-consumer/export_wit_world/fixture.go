package export_wit_world

import (
	"fmt"

	"wit_component/ohrats_rc_plugin_greeter"
)

func invoke(command string) (uint32, error) {
	if command != "caller-alt" {
		return 0, fmt.Errorf("unsupported command %q", command)
	}
	fmt.Println(ohrats_rc_plugin_greeter.Greet("__caller__"))
	return 0, nil
}
