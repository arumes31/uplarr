package sftpclient

import "testing"

func TestTunable(t *testing.T) {
	const env = "UPLARR_TEST_TUNABLE"

	tests := []struct {
		name string
		set  bool
		val  string
		want int
	}{
		{name: "unset falls back", set: false, want: 100},
		{name: "valid value is used", set: true, val: "50", want: 50},
		{name: "lower bound is inclusive", set: true, val: "10", want: 10},
		{name: "upper bound is inclusive", set: true, val: "200", want: 200},
		{name: "below range falls back", set: true, val: "9", want: 100},
		{name: "above range falls back", set: true, val: "201", want: 100},
		{name: "non-numeric falls back", set: true, val: "128k", want: 100},
		{name: "empty falls back", set: true, val: "", want: 100},
		{name: "negative falls back", set: true, val: "-5", want: 100},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv(env, tc.val)
			}
			if got := tunable(env, 100, 10, 200); got != tc.want {
				t.Errorf("tunable(%q=%q) = %d, want %d", env, tc.val, got, tc.want)
			}
		})
	}
}

// The packet ceiling has to stay under pkg/sftp's 256 KiB message limit, which
// the payload shares with the packet header. Asking for the full 256 KiB makes
// the server drop the connection mid-transfer.
func TestMaxPacketCeilingLeavesHeaderRoom(t *testing.T) {
	const sftpMaxMsgLength = 256 * 1024
	if maxPacketCeiling >= sftpMaxMsgLength {
		t.Errorf("maxPacketCeiling %d must be below the %d message limit", maxPacketCeiling, sftpMaxMsgLength)
	}
	if defaultMaxPacket > maxPacketCeiling {
		t.Errorf("defaultMaxPacket %d exceeds ceiling %d", defaultMaxPacket, maxPacketCeiling)
	}
	// 32 KiB is the size every server must accept, so it is the only safe default.
	if defaultMaxPacket != 32768 {
		t.Errorf("defaultMaxPacket = %d, want the spec-guaranteed 32768", defaultMaxPacket)
	}
}
