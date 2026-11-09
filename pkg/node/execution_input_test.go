package node

import "testing"

func TestNormalizeTerminalInput(t *testing.T) {
	cases := []struct {
		name string
		crlf bool
		in   string
		want string
	}{
		{name: "unix lone cr", in: "echo ok\r", want: "echo ok\n"},
		{name: "unix crlf", in: "echo ok\r\n", want: "echo ok\n"},
		{name: "windows lone cr", crlf: true, in: "echo ok\r", want: "echo ok\r\n"},
		{name: "windows crlf unchanged", crlf: true, in: "echo ok\r\n", want: "echo ok\r\n"},
		{name: "other bytes preserved", crlf: true, in: "a\x03b", want: "a\x03b"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := string(normalizeTerminalInput([]byte(test.in), test.crlf)); got != test.want {
				t.Fatalf("normalizeTerminalInput(%q, %v) = %q, want %q", test.in, test.crlf, got, test.want)
			}
		})
	}
}
