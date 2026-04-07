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

func TestChooseStorageBackend(t *testing.T) {
	t.Run("explicit storage wins", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://env")
		if got := chooseStorageBackend("sqlite", ""); got != "sqlite" {
			t.Fatalf("got %q want sqlite", got)
		}
	})

	t.Run("explicit database url selects postgres", func(t *testing.T) {
		if got := chooseStorageBackend("", "postgres://flag"); got != "postgres" {
			t.Fatalf("got %q want postgres", got)
		}
	})

	t.Run("database url env selects postgres", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://env")
		if got := chooseStorageBackend("", ""); got != "postgres" {
			t.Fatalf("got %q want postgres", got)
		}
	})

	t.Run("no postgres hint defaults sqlite", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "")
		if got := chooseStorageBackend("", ""); got != "sqlite" {
			t.Fatalf("got %q want sqlite", got)
		}
	})
}
