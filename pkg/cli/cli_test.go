package cli

import (
	"flag"
	"reflect"
	"testing"
)

func TestSplitEnrollTokenAcceptsLeadingDashToken(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		token string
		rest  []string
	}{
		{
			name:  "leading dash token first",
			args:  []string{"-Kq8k5s-9OyCKFEJo-nY_pMy04098pnz9Nak9hM3hV8", "--url", "https://rc.example.test"},
			token: "-Kq8k5s-9OyCKFEJo-nY_pMy04098pnz9Nak9hM3hV8",
			rest:  []string{"--url", "https://rc.example.test"},
		},
		{
			name:  "flags before token",
			args:  []string{"--url", "https://rc.example.test", "--name", "Node", "-token_with_dash"},
			token: "-token_with_dash",
			rest:  []string{"--url", "https://rc.example.test", "--name", "Node"},
		},
		{
			name:  "escaped token",
			args:  []string{"--url", "https://rc.example.test", "--", "-escaped_token"},
			token: "-escaped_token",
			rest:  []string{"--url", "https://rc.example.test"},
		},
		{
			name:  "plain token",
			args:  []string{"plain_token", "--url", "https://rc.example.test"},
			token: "plain_token",
			rest:  []string{"--url", "https://rc.example.test"},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			token, rest := splitEnrollToken(test.args)
			if token != test.token {
				t.Fatalf("token = %q, want %q", token, test.token)
			}
			if !reflect.DeepEqual(rest, test.rest) {
				t.Fatalf("rest = %v, want %v", rest, test.rest)
			}
		})
	}
}

func TestReorderArgsPreservesEscapeTerminator(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	url := fs.String("url", "", "server URL")
	args := reorderArgs(fs, []string{"--url", "https://rc.example.test", "--", "-token"})
	if err := fs.Parse(args); err != nil {
		t.Fatalf("parse = %v", err)
	}
	if *url != "https://rc.example.test" || fs.NArg() != 1 || fs.Arg(0) != "-token" {
		t.Fatalf("url = %q, args = %v", *url, fs.Args())
	}
}
