package protocol

import "testing"

func TestStrictV2McpGrants(t *testing.T) {
	audAccount := "account"
	audOther := "device"

	// Valid V2 grant: v=2, audience="account", deviceIds empty
	v2Valid := McpGrantPayload{
		V:         2,
		ID:        "grant_v2",
		UserID:    "usr_1",
		Audience:  &audAccount,
		DeviceIDs: []string{},
	}
	if !v2Valid.ValidAudience() {
		t.Fatal("expected ValidAudience to return true for valid V2 grant")
	}
	if !v2Valid.AllowsDevice("any_device_id") {
		t.Fatal("expected AllowsDevice to return true for account-wide V2 grant")
	}

	// Deprecated V1 grant: v=1, audience=nil, deviceIds set -> must be rejected
	v1Legacy := McpGrantPayload{
		V:         1,
		ID:        "grant_v1",
		UserID:    "usr_1",
		Audience:  nil,
		DeviceIDs: []string{"dev_1"},
	}
	if v1Legacy.ValidAudience() {
		t.Fatal("expected ValidAudience to reject deprecated V1 grant")
	}
	if v1Legacy.AllowsDevice("dev_1") {
		t.Fatal("expected AllowsDevice to reject deprecated V1 grant")
	}

	// Invalid V2 grant: audience not "account"
	v2BadAud := McpGrantPayload{
		V:         2,
		ID:        "grant_v2_bad",
		UserID:    "usr_1",
		Audience:  &audOther,
		DeviceIDs: []string{},
	}
	if v2BadAud.ValidAudience() {
		t.Fatal("expected ValidAudience to reject V2 grant without 'account' audience")
	}

	// Invalid V2 grant: deviceIds non-empty
	v2NonEmptyDevices := McpGrantPayload{
		V:         2,
		ID:        "grant_v2_devices",
		UserID:    "usr_1",
		Audience:  &audAccount,
		DeviceIDs: []string{"dev_1"},
	}
	if v2NonEmptyDevices.ValidAudience() {
		t.Fatal("expected ValidAudience to reject V2 grant with deviceIds")
	}
}
