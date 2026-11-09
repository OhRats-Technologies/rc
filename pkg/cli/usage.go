package cli

import "fmt"

func PrintUsage() {
	fmt.Printf(`RC %s — Remote control and device node

Usage:
  rc <command> [arguments]

Commands:
  version               Print the installed RC version
  status                Show local enrollment and remote Node status
  devices               List devices available to the signed-in account
  config                Read or change local Node configuration
  enroll                Enroll this machine as an RC Node
  run                   Run the foreground Node presence loop
  service               Install or manage the per-user background Node
  login                 Save an RC API key for CLI requests
  logout                Remove saved CLI account authorization
  logs                  Show the local service or component diagnostics
  ps                    Inspect live processes in the local RC service
  doctor                Show bounded local component and runtime diagnostics
  update                Update managed core components from the release
  upgrade               Replace the native RC platform from the release
  repair                Reinstall core components and revalidate the runtime
  commands              List commands exported by installed components
  crypto-check          Verify the installed control crypto component
  help                  Show this help message

Unknown commands are dispatched to an installed component that declares them.

"rc shell" opens a local shell on the enrolled device. "rc shell DEVICE" uses
the authenticated WebRTC control session and saved API key authorization.
`, Version)
}
