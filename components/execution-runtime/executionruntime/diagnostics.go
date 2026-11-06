package executionruntime

import (
	"errors"
	"strconv"

	diagnosticsReporting "wit_component/ohrats_rc_diagnostics_reporting"
	diagnosticsTypes "wit_component/ohrats_rc_diagnostics_types"
	environmentHost "wit_component/ohrats_rc_process_environment_host"
)

// Activate reports the portable execution backends selected for this host.
// Only backend names are reported; no environment values or execution content
// reach diagnostics.
func Activate() error {
	windows := environmentHost.HostPlatform() == environmentHost.PlatformKind_Windows
	submitted := diagnosticsReporting.Submit(diagnosticsTypes.Report{
		Level:   diagnosticsTypes.Level_Info,
		Source:  "rc.runtime",
		Code:    "runtime.active",
		Message: "portable execution runtime activated",
		Fields: []diagnosticsTypes.Field{
			{Name: "platform", Value: backendName(windows, "windows", "unix")},
			{Name: "process_backend", Value: backendName(windows, "job-object", "process-group")},
			{Name: "terminal_backend", Value: backendName(windows, "conpty", "pty")},
		},
	})
	if submitted.IsErr() {
		return errors.New(submitted.Err())
	}
	return nil
}

// counts reports active executions per lifetime after every start or finish.
func counts(active [3]int) {
	_ = diagnosticsReporting.Submit(diagnosticsTypes.Report{
		Level:   diagnosticsTypes.Level_Info,
		Source:  "rc.runtime",
		Code:    "runtime.executions",
		Message: "execution counts changed",
		Fields: []diagnosticsTypes.Field{
			{Name: "attached", Value: strconv.Itoa(active[0])},
			{Name: "managed", Value: strconv.Itoa(active[1])},
			{Name: "scheduled", Value: strconv.Itoa(active[2])},
		},
	})
}

func backendName(windows bool, windowsValue string, unixValue string) string {
	if windows {
		return windowsValue
	}
	return unixValue
}
