package cli

import (
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/OhRats-Technologies/rc/pkg/apiclient"
	"github.com/OhRats-Technologies/rc/pkg/node"
)

func runLogin(args []string) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	server := fs.String("url", "", "RC server URL")
	token := fs.String("token", "", "RC API key (or RC_API_TOKEN)")
	stateDir := fs.String("state-dir", "", "Override state directory")
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		return err
	}
	value := *token
	if value == "" {
		value = os.Getenv("RC_API_TOKEN")
	}
	if value == "" {
		return fmt.Errorf("rc login requires --token or RC_API_TOKEN with an rcsk_ API key")
	}
	credential, err := apiclient.ParseCredential(value)
	if err != nil {
		return err
	}
	if credential.Kind != apiclient.CredentialPop || credential.Pop == nil {
		return fmt.Errorf("rc login requires an rcsk_ API key")
	}
	dir, err := node.ResolveStateDir(*stateDir)
	if err != nil {
		return err
	}
	base := *server
	if base == "" {
		base = os.Getenv("RC_URL")
	}
	if base == "" {
		base = "https://rc.ohrats.party"
	}
	seed := base64.RawURLEncoding.EncodeToString(credential.Pop.Signing.Seed())
	if err := node.SaveAccount(dir, &node.AccountSession{
		Server:      base,
		ClientID:    credential.Pop.ID,
		SigningSeed: seed,
	}); err != nil {
		return err
	}
	fmt.Printf("Saved RC API credentials for %s (%s)\n", base, credential.Pop.ID)
	return nil
}

func runLogout(args []string) error {
	fs := flag.NewFlagSet("logout", flag.ContinueOnError)
	stateDir := fs.String("state-dir", "", "Override state directory")
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		return err
	}
	dir, err := node.ResolveStateDir(*stateDir)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "account.json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := validateStateDirectory(dir); err != nil {
		return err
	}
	fmt.Println("RC CLI account authorization removed")
	return nil
}
