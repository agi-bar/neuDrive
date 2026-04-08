package serverapp

import (
	"testing"

	"github.com/agi-bar/agenthub/internal/app/appcore"
	"github.com/agi-bar/agenthub/internal/services"
)

func TestSchedulerTokenServiceAllowsSQLiteBackends(t *testing.T) {
	app := &appcore.App{
		Storage:       "sqlite",
		MemoryService: &services.MemoryService{},
		TokenService:  &services.TokenService{},
		InboxService:  &services.InboxService{},
		SyncService:   &services.SyncService{},
	}

	tokenSvc, cfg, ok := schedulerConfigForApp(app)
	if !ok || tokenSvc == nil {
		t.Fatal("expected scheduler to start for sqlite-backed app when deps are present")
	}
	if cfg.CleanExpiredScratch.Enabled || cfg.GenerateDailyScratch.Enabled {
		t.Fatal("expected scratch maintenance jobs to be disabled when memory service lacks db maintenance support")
	}
}

func TestSchedulerTokenServiceRejectsMissingDeps(t *testing.T) {
	app := &appcore.App{
		Storage:      "postgres",
		TokenService: &services.TokenService{},
	}

	if tokenSvc, _, ok := schedulerConfigForApp(app); ok || tokenSvc != nil {
		t.Fatal("expected scheduler deps check to fail when services are missing")
	}
}
