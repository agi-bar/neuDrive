package cli

import (
	"testing"

	"github.com/agi-bar/neudrive/internal/app/appcore"
)

func TestShouldUseLocalSyncDefaults(t *testing.T) {
	if shouldUseLocalSyncDefaults([]string{"--help"}) {
		t.Fatal("--help should not default to local sync env injection")
	}
	if shouldUseLocalSyncDefaults([]string{"login"}) {
		t.Fatal("login should not default to local sync env injection")
	}
	if !shouldUseLocalSyncDefaults([]string{"push", "--bundle", "backup.ndrvz"}) {
		t.Fatal("push should default to local sync env injection")
	}
	if shouldUseLocalSyncDefaults([]string{"push", "--profile", "official"}) {
		t.Fatal("explicit profile should disable local sync env injection")
	}
}

func TestChooseStorageBackend(t *testing.T) {
	t.Run("explicit storage wins", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://env")
		if got := chooseStorageBackend(appcore.DefaultServerStorage, "sqlite", "", ""); got != "sqlite" {
			t.Fatalf("got %q want sqlite", got)
		}
	})

	t.Run("explicit database url selects postgres", func(t *testing.T) {
		if got := chooseStorageBackend(appcore.DefaultLocalStorage, "", "", "postgres://flag"); got != "postgres" {
			t.Fatalf("got %q want postgres", got)
		}
	})

	t.Run("explicit sqlite path selects sqlite", func(t *testing.T) {
		if got := chooseStorageBackend(appcore.DefaultServerStorage, "", "/tmp/neudrive.db", ""); got != "sqlite" {
			t.Fatalf("got %q want sqlite", got)
		}
	})

	t.Run("server mode defaults postgres", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://env")
		if got := chooseStorageBackend(appcore.DefaultServerStorage, "", "", ""); got != "postgres" {
			t.Fatalf("got %q want postgres", got)
		}
	})

	t.Run("local mode defaults sqlite even with database url env", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://env")
		if got := chooseStorageBackend(appcore.DefaultLocalStorage, "", "", ""); got != "sqlite" {
			t.Fatalf("got %q want sqlite", got)
		}
	})
}
