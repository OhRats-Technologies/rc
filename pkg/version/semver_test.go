package version

import "testing"

func TestSemverOrdering(t *testing.T) {
	cases := []struct {
		left, right string
		want        int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.1", "1.0.0", 1},
		{"1.1.0", "1.0.9", 1},
		{"2.0.0", "1.99.99", 1},
		{"1.0.0-rc.1", "1.0.0", -1},
		{"1.0.0-rc.1", "1.0.0-rc.2", -1},
		{"1.0.0-rc.10", "1.0.0-rc.9", 1},
		{"1.0.0-alpha", "1.0.0-beta", -1},
		{"1.0.0-1", "1.0.0-alpha", -1},
		{"v0.20.2", "0.20.2", 0},
		{"0.20.2+build.5", "0.20.2", 0},
	}
	for _, tc := range cases {
		got, err := CompareStrings(tc.left, tc.right)
		if err != nil {
			t.Fatalf("CompareStrings(%q, %q): %v", tc.left, tc.right, err)
		}
		if got != tc.want {
			t.Errorf("CompareStrings(%q, %q) = %d, want %d", tc.left, tc.right, got, tc.want)
		}
	}
}

func TestSemverRejectsInvalid(t *testing.T) {
	for _, value := range []string{"", "1", "1.2", "1.2.x", "01.2.3", "1.2.3-", "1.2.3-a..b"} {
		if _, err := ParseSemver(value); err == nil {
			t.Errorf("ParseSemver(%q) unexpectedly succeeded", value)
		}
	}
}
