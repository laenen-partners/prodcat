// Showcase server for the prodcat UI components backed by a real Postgres database.
// The UI talks to an embedded ConnectRPC server, exercising the full RPC path.
//
// Prerequisites: docker compose up -d (or tilt up)
// Run with: go run ./cmd/showcase (or: task showcase)
// Then open http://localhost:3335
package main

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/laenen-partners/dsx/showcase"
	"github.com/laenen-partners/dsx/stream"
	"github.com/laenen-partners/entitystore"
	"github.com/laenen-partners/identity"
	"github.com/laenen-partners/prodcat"
	prodcatrpc "github.com/laenen-partners/prodcat/connectrpc"
	"github.com/laenen-partners/prodcat/connectrpc/gen/prodcat/rpc/v1/prodcatrpcv1connect"
	esstore "github.com/laenen-partners/prodcat/entitystore"
	prodcatui "github.com/laenen-partners/prodcat/ui"
	"github.com/laenen-partners/pubsub"
)

const defaultDSN = "postgres://showcase:showcase@localhost:5488/prodcat_showcase?sslmode=disable"

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
	if err := showcase.Run(showcase.Config{
		Port: 3335,
		Identities: []showcase.Identity{
			{Name: "Admin", TenantID: "showcase", WorkspaceID: "ws-1", PrincipalID: "admin-1", Roles: []string{"admin"}},
			{Name: "Member", TenantID: "showcase", WorkspaceID: "ws-1", PrincipalID: "user-1", Roles: []string{"member"}},
			{Name: "Viewer", TenantID: "showcase", WorkspaceID: "ws-1", PrincipalID: "viewer-1", Roles: []string{"viewer"}},
		},
		Setup: func(ctx context.Context, r chi.Router, bus *pubsub.Bus, relay *stream.Relay) error {
			result, err := setupRPC(ctx)
			if err != nil {
				return err
			}

			if err := seedData(ctx, result.seedClient, result.tracker); err != nil {
				slog.Error("seed data failed", "error", err)
			}

			h := prodcatui.NewHandlers(result.rpcClient, func(id identity.Context) prodcatui.AccessScope {
				if id.HasRole("admin") {
					return prodcatui.AccessScope{
						ListTags:  []string{},
						CanEdit:   true,
						CanImport: true,
					}
				}
				if id.HasRole("member") {
					return prodcatui.AccessScope{
						ListTags: []string{},
						CanEdit:  true,
					}
				}
				return prodcatui.AccessScope{
					ListTags: []string{},
				}
			})

			h.RegisterRoutes(r)
			return nil
		},
		Pages: map[string]templ.Component{
			"/": showcasePage(),
		},
	}); err != nil {
		slog.Error("showcase failed", "error", err)
		os.Exit(1)
	}
}

// setupResult holds the clients produced by setupRPC.
type setupResult struct {
	rpcClient      *prodcatrpc.Client // scoped — for UI/RPC handlers
	seedClient     *prodcat.Client    // scoped — for seeding (auto-tags tenant)
	unscopedClient *prodcat.Client    // unscoped — if needed for system ops
	tracker        prodcat.ImportTracker
}

func setupRPC(ctx context.Context) (*setupResult, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = defaultDSN
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, err
	}
	if err := entitystore.Migrate(ctx, pool); err != nil {
		return nil, err
	}

	es, err := entitystore.New(entitystore.WithPgStore(pool))
	if err != nil {
		return nil, err
	}

	// Scope the store to the showcase tenant — all reads are filtered
	// and all creates are auto-tagged with tenant:showcase.
	scoped := es.Scoped(entitystore.ScopeConfig{
		RequireTags: []string{"tenant:showcase"},
		AutoTags:    []string{"tenant:showcase"},
	})
	scopedStore := esstore.NewStore(scoped)
	scopedClient := prodcat.NewClient(scopedStore)

	tracker := esstore.NewImportTracker(es) // unscoped (system-level)

	// Mount ConnectRPC handlers using the scoped client.
	mux := http.NewServeMux()
	handler := prodcatrpc.NewHandler(scopedClient, tracker)
	mux.Handle(prodcatrpcv1connect.NewProductCatalogQueryServiceHandler(handler))
	mux.Handle(prodcatrpcv1connect.NewProductCatalogCommandServiceHandler(handler))
	ts := httptest.NewServer(mux)

	slog.Info("embedded ConnectRPC server started", "url", ts.URL)

	return &setupResult{
		rpcClient:  prodcatrpc.NewClientFromHTTP(ts.Client(), ts.URL),
		seedClient: scopedClient, // seeding through scoped client = auto-tagged
		tracker:    tracker,
	}, nil
}

// seedData imports the real catalogue definition files from catalog/.
// After import, it creates a suspended product and disables a ruleset
// to showcase all UI states.
func seedData(ctx context.Context, client *prodcat.Client, tracker prodcat.ImportTracker) error {
	// Import all catalogue files in order (they're timestamped).
	catalogDir := "../catalog"
	files, err := filepath.Glob(filepath.Join(catalogDir, "*.yaml"))
	if err != nil {
		return err
	}
	sort.Strings(files)

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			slog.Warn("skip catalogue file", "file", f, "error", err)
			continue
		}
		// Pass nil tracker so catalogue files are always re-imported (upserted).
		// This ensures the showcase always has the latest catalogue content.
		if err := client.Import(ctx, filepath.Base(f), data, nil); err != nil {
			slog.Warn("import catalogue file", "file", f, "error", err)
		}
	}

	prov := prodcat.Provenance{SourceURN: "showcase:seed", Reason: "showcase extra data"}

	// Add a disabled product to showcase the disabled state.
	client.RegisterProduct(ctx, prodcat.Product{
		ProductID:    "usd-account",
		Name:         "USD Account",
		Description:  "Multi-currency USD account — disabled pending regulatory review",
		Tags:         []string{"family:casa", "type:current_account", "currency:usd"},
		CurrencyCode: "USD",
		Availability: prodcat.GeoAvailability{Mode: prodcat.AvailabilityModeGlobal},
	}, prov)
	client.DisableProduct(ctx, "usd-account", prodcat.DisabledReasonRegulatoryHold, prov)

	slog.Info("showcase data seeded", "catalog_files", len(files))
	return nil
}
