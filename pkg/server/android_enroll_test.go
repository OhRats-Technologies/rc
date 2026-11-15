package server

import (
	"strings"
	"testing"
)

func TestAndroidEnrollmentCommands(t *testing.T) {
	install := androidInstallCommand("https://rc.example", "tok123")
	if !strings.Contains(install, "android-enroll.sh") || !strings.Contains(install, "'tok123'") {
		t.Fatalf("android install command: %s", install)
	}
	if !strings.Contains(install, deviceNamePlaceholder) {
		t.Fatalf("android install must carry a name placeholder: %s", install)
	}

	enroll := androidEnrollCommand("https://rc.example", "tok123")
	for _, want := range []string{"PROOT_LOADER=", "proot -r", "/root/rc enroll", "--name " + deviceNamePlaceholder} {
		if !strings.Contains(enroll, want) {
			t.Fatalf("android enroll command missing %q: %s", want, enroll)
		}
	}
}

func TestInstallerCommandsCarryDeviceName(t *testing.T) {
	if !strings.Contains(installCommand("https://rc.example", "tok123"), deviceNamePlaceholder) {
		t.Fatal("install command must carry a name placeholder")
	}
	if !strings.Contains(enrollCommand("https://rc.example", "tok123"), "--name '"+deviceNamePlaceholder+"'") {
		t.Fatal("enroll command must carry a name placeholder")
	}
}
