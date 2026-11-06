package cli

import (
	"errors"
	"fmt"
	"strconv"

	"wit_component/ohrats_rc_diagnostics_query"
)

// Doctor prints the bounded component diagnostics of this CLI invocation.
func Doctor(args []string) (uint32, error) {
	if len(args) != 0 {
		return 0, errors.New("usage: rc doctor")
	}
	value := ohrats_rc_diagnostics_query.Status()
	fmt.Println("RC diagnostics")
	fmt.Printf("retained %d\n", value.Retained)
	fmt.Printf("warnings %d\n", value.Warnings)
	fmt.Printf("errors %d\n", value.Errors)
	fmt.Printf("newest sequence %d\n", value.NewestSequence)
	if value.Errors > 0 {
		return 1, nil
	}
	return 0, nil
}

// Logs reads the local service journal, or this invocation's component
// diagnostics when --components is requested.
func Logs(args []string) (uint32, error) {
	if len(args) == 0 || args[0] != "--components" {
		return liveLogs(args)
	}
	fmt.Println("Component diagnostics from this CLI invocation (not the background Node).")
	rest := args[1:]
	limit := uint32(20)
	switch len(rest) {
	case 0:
	case 1:
		parsed, err := strconv.ParseUint(rest[0], 10, 32)
		if err != nil {
			return 0, errors.New("log limit must be an integer")
		}
		limit = uint32(parsed)
	default:
		return 0, errors.New("usage: rc logs --components [limit]")
	}
	result := ohrats_rc_diagnostics_query.Recent(limit)
	if result.IsErr() {
		return 0, errors.New(result.Err())
	}
	for _, event := range result.Ok() {
		fmt.Printf(
			"%d %s %s %s: %s\n",
			event.Sequence,
			levelName(event.Level),
			event.Source,
			event.Code,
			event.Message,
		)
		for _, field := range event.Fields {
			fmt.Printf("  %s=%s\n", field.Name, field.Value)
		}
	}
	return 0, nil
}
