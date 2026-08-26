package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type sshKeyView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Algorithm string `json:"algorithm"`
	PublicKey string `json:"public_key"`
}

func sshKeyCommand(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Println("Usage: rc ssh-key add [--name NAME] [PUBLIC_KEY_FILE]")
		fmt.Println("       rc ssh-key list")
		fmt.Println("       rc ssh-key remove ID")
		return nil
	}
	switch args[0] {
	case "add":
		return addSshKey(args[1:])
	case "list":
		return listSshKeyCommand(args[1:])
	case "remove":
		return removeSshKey(args[1:])
	default:
		return fmt.Errorf("unknown ssh-key command %q", args[0])
	}
}

func cliAccountForSsh() (string, accountSession, ed25519.PrivateKey, error) {
	dir := resolveStateDir("")
	account, err := loadAccountSession(dir)
	if err != nil || account.Token == "" || account.ControlClientID == "" {
		return "", account, nil, errors.New("run rc login first")
	}
	privateKey, err := base64.RawURLEncoding.DecodeString(account.ControlPrivateKey)
	if err != nil || len(privateKey) != ed25519.PrivateKeySize {
		return "", account, nil, errors.New("CLI control key unavailable; run rc login again")
	}
	return strings.TrimRight(account.Server, "/"), account, ed25519.PrivateKey(privateKey), nil
}

func defaultSshPublicKeyPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ssh", "id_ed25519.pub")
}

func addSshKey(args []string) error {
	flags := flag.NewFlagSet("rc ssh-key add", flag.ContinueOnError)
	name := flags.String("name", "", "SSH key name")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 1 {
		return errors.New("usage: rc ssh-key add [--name NAME] [PUBLIC_KEY_FILE]")
	}
	path := defaultSshPublicKeyPath()
	if flags.NArg() == 1 {
		path = flags.Arg(0)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	publicKey := strings.TrimSpace(string(data))
	server, account, privateKey, err := cliAccountForSsh()
	if err != nil {
		return err
	}
	payload := "rc-ssh-key-v1\n" + account.ControlClientID + "\n" + publicKey
	signature := base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, []byte(payload)))
	keyName := strings.TrimSpace(*name)
	if keyName == "" {
		keyName = filepath.Base(strings.TrimSuffix(path, ".pub"))
	}
	resp, err := accountJSONRequest(server, account.Token, http.MethodPost, "/api/v1/ssh/keys", map[string]any{
		"name": keyName, "publicKey": publicKey, "clientId": account.ControlClientID, "signature": signature,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	fmt.Printf("Added SSH key %s (%s)\n", keyName, result.ID)
	return nil
}

func listSshKeyCommand(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: rc ssh-key list")
	}
	server, account, _, err := cliAccountForSsh()
	if err != nil {
		return err
	}
	resp, err := accountRequest(server, account.Token, http.MethodGet, "/api/v1/ssh/keys")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var result struct {
		Keys []sshKeyView `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	for _, key := range result.Keys {
		fmt.Printf("%s  %s  %s\n", key.ID, key.Name, key.Algorithm)
	}
	return nil
}

func removeSshKey(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: rc ssh-key remove ID")
	}
	server, account, _, err := cliAccountForSsh()
	if err != nil {
		return err
	}
	resp, err := accountRequest(server, account.Token, http.MethodDelete, "/api/v1/ssh/keys/"+args[0])
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	fmt.Println("Removed SSH key", args[0])
	return nil
}
