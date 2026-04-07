package cli

import "testing"

func TestShouldUseLocalSyncDefaults(t *testing.T) {
	if shouldUseLocalSyncDefaults([]string{"--help"}) {
		t.Fatal("--help should not default to local sync env injection")
	}
	if shouldUseLocalSyncDefaults([]string{"login"}) {
		t.Fatal("login should not default to local sync env injection")
	}
	if !shouldUseLocalSyncDefaults([]string{"push", "--bundle", "backup.ahubz"}) {
		t.Fatal("push should default to local sync env injection")
	}
	if shouldUseLocalSyncDefaults([]string{"push", "--profile", "official"}) {
		t.Fatal("explicit profile should disable local sync env injection")
	}
}
