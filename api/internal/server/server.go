package server

import (
	"net/http"

	"github.com/erhan/falcon/api/internal/auth"
	"github.com/erhan/falcon/api/internal/handlers"
	"github.com/erhan/falcon/api/internal/jobs"
	"github.com/erhan/falcon/api/internal/scope"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jmoiron/sqlx"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

type Deps struct {
	DB           *sqlx.DB
	Auth         *auth.Service
	Jobs         *jobs.Client
	ScopeCache   *scope.Cache
	ArtifactsDir string
}

func New(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60_000_000_000)) // 60s
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization", "X-Worker-Token"},
		AllowCredentials: false,
	}))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	// Swagger UI is mounted publicly so the API surface is browsable without auth.
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	authH := &handlers.AuthHandler{DB: d.DB, Auth: d.Auth}
	progH := &handlers.ProgramsHandler{DB: d.DB}
	tgtH := handlers.NewScopeHandler(d.DB, d.Jobs)
	scH := &handlers.OOSHandler{DB: d.DB, Cache: d.ScopeCache}
	finH := &handlers.ReportsHandler{DB: d.DB}
	runH := &handlers.RunsHandler{DB: d.DB}
	asH := &handlers.AssetsHandler{DB: d.DB, Cache: d.ScopeCache}
	vrtH := &handlers.VRTHandler{}
	intRunH := &handlers.InternalRunsHandler{DB: d.DB, ArtifactsDir: d.ArtifactsDir}

	r.Route("/api", func(r chi.Router) {
		r.Post("/auth/login", authH.Login)

		r.Group(func(r chi.Router) {
			r.Use(d.Auth.RequireAuth)
			r.Get("/auth/me", authH.Me)

			r.Get("/programs", progH.List)
			r.Post("/programs", progH.Create)
			r.Get("/programs/{id}", progH.Get)
			r.Patch("/programs/{id}", progH.Update)
			r.Delete("/programs/{id}", progH.Delete)

			// In-scope items (formerly "targets")
			r.Get("/programs/{id}/scope", tgtH.List)
			r.Post("/programs/{id}/scope", tgtH.Create)
			r.Post("/programs/{id}/scope/run-all", tgtH.RunAll)
			r.Patch("/scope/{id}", tgtH.Update)
			r.Delete("/scope/{id}", tgtH.Delete)
			r.Post("/scope/{id}/run", tgtH.Run)

			// Out-of-scope rules
			r.Get("/programs/{id}/oos", scH.List)
			r.Post("/programs/{id}/oos", scH.Create)
			r.Delete("/oos/{id}", scH.Delete)


			r.Get("/programs/{id}/reports", finH.List)
			r.Post("/programs/{id}/reports", finH.Create)
			r.Get("/reports/{id}", finH.Get)
			r.Patch("/reports/{id}", finH.Update)
			r.Delete("/reports/{id}", finH.Delete)

			r.Get("/programs/{id}/runs", runH.ListByProgram)
			r.Delete("/programs/{id}/runs", runH.DeleteAllForProgram)
			r.Get("/runs/{id}", runH.Get)
			r.Delete("/runs/{id}", runH.Delete)

			r.Get("/programs/{id}/hosts", asH.ListHosts)
			r.Get("/programs/{id}/hosts/{hostId}", asH.HostDetail)
			r.Patch("/programs/{id}/hosts/{hostId}", asH.UpdateHost)
			r.Get("/programs/{id}/services", asH.ListServices)
			r.Get("/programs/{id}/endpoints", asH.ListEndpoints)
			r.Get("/search", asH.Search)
			r.Get("/vrt", vrtH.Get)
			r.Post("/vrt/refresh", vrtH.Refresh)

			// Authenticated artifact serving so the run detail UI can fetch
			// step output files (jsonl, txt, log) for inline preview.
			r.Get("/artifacts", intRunH.Artifact)
		})
	})

	r.Route("/internal", func(r chi.Router) {
		r.Use(d.Auth.RequireWorker)
		r.Post("/runs/{id}/start", intRunH.Start)
		r.Post("/runs/{id}/steps", intRunH.Step)
		r.Post("/runs/{id}/finish", intRunH.Finish)
		r.Post("/programs/{id}/assets:bulk", asH.BulkUpsert)
		r.Get("/artifacts", intRunH.Artifact)
	})

	return r
}
