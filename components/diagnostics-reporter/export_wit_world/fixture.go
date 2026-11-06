package export_wit_world

import (
	"errors"
	"strings"

	"wit_component/ohrats_rc_diagnostics_reporting"
	"wit_component/ohrats_rc_diagnostics_types"
)

const (
	activateSource  = "rc.component"
	activateCode    = "component.active"
	activateMessage = "diagnostics reporter activated"
	cliSource       = "rc.cli"
)

func activate() error {
	result := ohrats_rc_diagnostics_reporting.Submit(ohrats_rc_diagnostics_types.Report{
		Level:   ohrats_rc_diagnostics_types.Level_Info,
		Source:  activateSource,
		Code:    activateCode,
		Message: activateMessage,
		Fields: []ohrats_rc_diagnostics_types.Field{
			{Name: "component", Value: "ohrats:diagnostics-reporter"},
		},
	})
	if result.IsErr() {
		return errors.New(result.Err())
	}
	return nil
}

func invoke(command string, args []string) (uint32, error) {
	if command != "report" || len(args) < 2 {
		return 0, errors.New("usage: rc report <code> <message>")
	}
	result := ohrats_rc_diagnostics_reporting.Submit(ohrats_rc_diagnostics_types.Report{
		Level:   ohrats_rc_diagnostics_types.Level_Info,
		Source:  cliSource,
		Code:    args[0],
		Message: strings.Join(args[1:], " "),
		Fields:  []ohrats_rc_diagnostics_types.Field{},
	})
	if result.IsErr() {
		return 0, errors.New(result.Err())
	}
	return 0, nil
}
