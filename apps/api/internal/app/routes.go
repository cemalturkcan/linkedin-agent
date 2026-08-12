package app

import (
	"github.com/gofiber/fiber/v3"

	"api/internal/app/httpx"
	credentialroutes "api/internal/routes/credentials"
	eventroutes "api/internal/routes/events"
	"api/internal/routes/indexer"
	"api/internal/routes/planner"
	"api/internal/routes/plugin"
	"api/internal/routes/postings"
	previewroutes "api/internal/routes/preview"
	"api/internal/routes/resumes"
	"api/internal/routes/screening"
	"api/internal/routes/settings"
	setuproutes "api/internal/routes/setup"
	traceroutes "api/internal/routes/traces"
)

type liveness struct {
	OK bool `json:"ok"`
}

func registerRoutes(server *fiber.App, runtime *Runtime) {
	api := server.Group(httpx.PathPrefix)

	api.Get("/health", func(c fiber.Ctx) error {
		return httpx.Respond(c, liveness{OK: true})
	})
	api.Get("/events", runtime.stream)

	api.Get("/credentials", runtime.credentials.Get)
	api.Put("/credentials", runtime.credentials.Put)
	api.Delete("/credentials", runtime.credentials.Delete)

	api.Get("/traces", runtime.traces.List)
	api.Get("/traces/:id", runtime.traces.Get)

	api.Get("/settings", runtime.settings.Get)
	api.Put("/settings", runtime.settings.Put)

	api.Get("/setup", runtime.setup.Get)
	api.Post("/setup", runtime.setup.Post)

	api.Get("/resumes", runtime.resumes.List)
	api.Get("/resumes/:code/:lang/file", runtime.resumes.File)
	api.Get("/profile", runtime.indexer.Profile)
	api.Post("/index", runtime.indexer.Index)

	api.Post("/plugin/hello", runtime.plugin.Hello)
	api.Get("/plugin", runtime.plugin.Get)

	api.Post("/cycle/start", runtime.planner.Start)
	api.Post("/cycle/widen", runtime.planner.Widen)
	api.Post("/cycle/skip", runtime.planner.Skip)
	api.Post("/cycle/finish", runtime.planner.Finish)
	api.Get("/plan", runtime.planner.Plan)
	api.Get("/notes", runtime.planner.Notes)
	api.Delete("/notes/:id", runtime.planner.DeleteNote)
	api.Get("/prompt/preview", runtime.preview.Get)

	api.Post("/jobs", runtime.postings.Ingest)
	api.Get("/jobs", runtime.postings.List)
	api.Post("/jobs/opened", runtime.postings.Opened)
	api.Get("/places", runtime.postings.Places)
	api.Post("/places", runtime.postings.SavePlaces)

	api.Post("/jobs/screen", runtime.screening.Screen)
	api.Get("/jobs/pending-descriptions", runtime.screening.Pending)
	api.Post("/jobs/descriptions", runtime.screening.Descriptions)
	api.Get("/jobs/stale", runtime.screening.Stale)
	api.Post("/jobs/rescreen", runtime.screening.Rescreen)
	api.Post("/jobs/:id/outcome", runtime.screening.Outcome)
	api.Post("/jobs/:id/:transition", runtime.screening.Move)
	api.Post("/reflect", runtime.screening.Reflect)
}

func newScreeningHandler(runtime *Runtime) *screening.Handler {
	return screening.NewHandler(runtime.Screening, screening.HandlerConfig{
		OperationTimeout: runtime.Config.ScreeningOperationTimeout,
		ScreenBudget:     runtime.Config.ScreenBudget,
		PendingLimit:     runtime.Config.ScreeningPendingLimit,
	})
}

func newPreviewHandler(runtime *Runtime) *previewroutes.Handler {
	return previewroutes.NewHandler(runtime.Planner, runtime.Screening, previewroutes.Config{
		OperationTimeout: runtime.Config.ScreeningOperationTimeout,
	})
}

func newSettingsHandler(runtime *Runtime) *settings.Handler {
	return settings.NewHandler(runtime.Settings, settings.Config{
		OperationTimeout: runtime.Config.SettingsOperationTimeout,
	})
}

func newSetupHandler(runtime *Runtime) *setuproutes.Handler {
	return setuproutes.NewHandler(
		runtime.Resumes,
		runtime.Indexer,
		runtime.Credentials,
		runtime.Hub,
		setuproutes.Config{
			OperationTimeout: runtime.Config.SetupOperationTimeout,
			IndexBudget:      runtime.Config.IndexBudget,
		},
	)
}

func newResumeHandler(runtime *Runtime) *resumes.Handler {
	return resumes.NewHandler(runtime.Resumes, resumes.Config{
		OperationTimeout: runtime.Config.ResumesOperationTimeout,
	})
}

func newIndexerHandler(runtime *Runtime) *indexer.Handler {
	return indexer.NewHandler(runtime.Indexer, indexer.HandlerConfig{
		OperationTimeout: runtime.Config.ResumesOperationTimeout,
		IndexBudget:      runtime.Config.IndexBudget,
	})
}

func newPluginHandler(runtime *Runtime) *plugin.Handler {
	return plugin.NewHandler(runtime.Plugin, plugin.HandlerConfig{
		OperationTimeout: runtime.Config.PluginOperationTimeout,
	})
}

func newPostingsHandler(runtime *Runtime) *postings.Handler {
	return postings.NewHandler(runtime.Postings, postings.HandlerConfig{
		OperationTimeout: runtime.Config.PostingsOperationTimeout,
	})
}

func newPlannerHandler(runtime *Runtime) *planner.Handler {
	return planner.NewHandler(runtime.Planner, planner.Config{
		HistoryRounds:    runtime.Config.PlannerHistoryRounds,
		NoteCap:          runtime.Config.PlannerNoteCap,
		KnownIDWindow:    runtime.Config.PlannerKnownIDWindow,
		KnownIDCap:       runtime.Config.PlannerKnownIDCap,
		OperationTimeout: runtime.Config.PlannerOperationTimeout,
		PlanBudget:       runtime.Config.PlannerBudget,
	})
}

func newCredentialHandler(runtime *Runtime) *credentialroutes.Handler {
	return credentialroutes.NewHandler(runtime.Credentials, runtime.Hub)
}

func newTraceHandler(runtime *Runtime) *traceroutes.Handler {
	return traceroutes.NewHandler(runtime.Traces, traceroutes.Config{
		OperationTimeout: runtime.Config.TracesOperationTimeout,
		PageLimitDefault: runtime.Config.TracesPageLimitDefault,
		PageLimitMax:     runtime.Config.TracesPageLimitMax,
	})
}

func newStreamHandler(runtime *Runtime) fiber.Handler {
	return eventroutes.NewStreamHandler(runtime.Hub, eventroutes.Config{
		Retry:     runtime.Config.StreamRetry,
		Heartbeat: runtime.Config.StreamHeartbeat,
	})
}
