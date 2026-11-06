package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/OhRats-Technologies/rc/pkg/apiclient"
	"github.com/OhRats-Technologies/rc/pkg/node"
)

func runDevices(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("devices", flag.ContinueOnError)
	server := fs.String("url", "", "RC server URL")
	token := fs.String("token", "", "RC API key (or RC_API_TOKEN)")
	stateDir := fs.String("state-dir", "", "Override state directory")
	asJSON := fs.Bool("json", false, "Print JSON")
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		return err
	}
	dir, err := node.ResolveStateDir(*stateDir)
	if err != nil {
		return err
	}
	credential, err := deviceCredential(dir, *token)
	if err != nil {
		return err
	}
	base := *server
	if base == "" {
		config, _ := node.LoadConfig(dir)
		base = config.Server
	}
	if base == "" {
		base = os.Getenv("RC_URL")
	}
	if base == "" {
		base = "https://rc.ohrats.party"
	}
	client, err := apiclient.New(base, credential)
	if err != nil {
		return err
	}
	devices, err := client.Devices(ctx)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"devices": devices})
	}
	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "ID\tNAME\tVERSION\tONLINE")
	for _, device := range devices {
		versionName := device.Version
		if versionName == "" {
			versionName = "-"
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%t\n", device.ID, device.Name, versionName, device.Online)
	}
	return writer.Flush()
}

func deviceCredential(dir, token string) (apiclient.Credential, error) {
	value := token
	if value == "" {
		value = os.Getenv("RC_API_TOKEN")
	}
	if value != "" {
		return apiclient.ParseCredential(value)
	}
	if account, err := node.LoadAccount(dir); err == nil && account.SigningSeed != "" && account.ClientID != "" {
		return apiclient.CredentialFromSigningSeed(account.ClientID, account.SigningSeed)
	}
	if state, err := node.LoadState(dir); err == nil {
		return apiclient.CredentialFromSigningSeed(state.DeviceID, state.IdentitySeed)
	}
	return apiclient.Credential{}, apiclient.ErrMissingCredential
}
