package security

import "testing"

// The apps hash with react-native-argon2 (iOS: the same parameters). This vector comes from
// lomo-mobile's AuthService.hashPassword for user "alice", password "alice-pw-1"; the server
// must produce the identical string to check plain-text passwords (WebDAV Basic auth).
func TestEncryptPasswordMatchesTheApps(t *testing.T) {
	const want = "246172676f6e32696424763d3139246d3d343039362c743d332c703d312459577870593256416247397462334a685a3255756247397462336468636d5524346b6c3266717a444576757459565448576c4a5a62755a685733447169714f73376a4c726243384763493000"
	if got := EncryptPassword("alice", "alice-pw-1"); got != want {
		t.Fatalf("EncryptPassword = %s, want %s", got, want)
	}
}
