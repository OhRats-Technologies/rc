package auth

import (
	"errors"
	"testing"
	"time"
)

func TestScheduleGrantProjectionAndExpiry(t *testing.T) {
	store := openStore(t)
	user, err := store.CreateUser("Schedule User")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureWorkspace("ws_schedule", "Schedule"); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureWorkspaceMembership("ws_schedule", user.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	device := Device{
		ID: "dev_schedule", WorkspaceID: "ws_schedule", Name: "Node",
		Hostname: "host", Platform: "linux", Arch: "amd64",
		Version: "0.20.6", IdentityPublicKey: "identity", TransportPublicKey: "transport",
		CreatedAt: time.Now().UTC(),
	}
	if err := store.InsertDevice(device); err != nil {
		t.Fatal(err)
	}
	grant := ScheduleGrant{
		ScheduleID: "schedule_a", WorkspaceID: "ws_schedule", DeviceID: device.ID,
		UserID: user.ID, SpecHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		MaxRuntimeMs: 60000, CreatedAt: time.Now().UTC(),
	}
	expired := grant
	expired.ScheduleID = "schedule_b"
	expired.ExpiresAt = time.Now().Add(-time.Minute)
	for _, value := range []ScheduleGrant{grant, expired} {
		if err := store.PutScheduleGrant(value); err != nil {
			t.Fatal(err)
		}
	}
	grants, err := store.ActiveScheduleGrantsForWorkspace("ws_schedule")
	if err != nil {
		t.Fatal(err)
	}
	if len(grants) != 1 || grants[0].ScheduleID != "schedule_a" ||
		grants[0].MaxRuntimeMs != 60000 || grants[0].ExpiresAt.IsZero() != true {
		t.Fatalf("active schedules = %+v", grants)
	}
	wrong := grant
	wrong.WorkspaceID = "ws_other"
	if err := store.PutScheduleGrant(wrong); !errors.Is(err, ErrScheduleGrantNotFound) {
		t.Fatalf("cross-workspace schedule = %v", err)
	}
	removed, err := store.DeleteScheduleGrant("ws_schedule", "schedule_a")
	if err != nil || !removed {
		t.Fatalf("delete schedule = %v, %v", removed, err)
	}
	grants, err = store.ActiveScheduleGrantsForWorkspace("ws_schedule")
	if err != nil || len(grants) != 0 {
		t.Fatalf("schedules after delete = %+v, %v", grants, err)
	}
}
