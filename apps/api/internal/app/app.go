package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gofiber/fiber/v3"

	"api/internal/app/claude"
	"api/internal/app/clock"
	"api/internal/app/credentials"
	"api/internal/app/events"
	"api/internal/app/store"
	"api/internal/app/trace"
	credentialroutes "api/internal/routes/credentials"
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

type Runtime struct {
	Config      Config
	Clock       clock.Clock
	Store       *store.Store
	Hub         *events.Hub
	Recorder    *trace.Recorder
	Credentials *credentials.Resolver
	Claude      *claude.Client
	Resumes     *resumes.Service
	Settings    *settings.Service
	Indexer     *indexer.Service
	Plugin      *plugin.Service
	Postings    *postings.Service
	Planner     *planner.Service
	Screening   *screening.Service
	Traces      *traceroutes.Service
	Server      *fiber.App

	credentials *credentialroutes.Handler
	traces      *traceroutes.Handler
	settings    *settings.Handler
	setup       *setuproutes.Handler
	resumes     *resumes.Handler
	indexer     *indexer.Handler
	plugin      *plugin.Handler
	postings    *postings.Handler
	planner     *planner.Handler
	screening   *screening.Handler
	preview     *previewroutes.Handler
	stream      fiber.Handler
}

func Build(ctx context.Context, config Config) (*Runtime, error) {
	storeCtx, cancel := context.WithTimeout(ctx, config.StoreOpenTimeout)
	defer cancel()
	opened, err := store.Open(storeCtx, config.StorePath())
	if err != nil {
		return nil, err
	}

	source := clock.Clock(clock.System{})
	hub := events.NewHub(source)
	recorder := trace.NewRecorder(trace.Dependencies{
		Reads:        opened.Reads(),
		Transactions: opened,
		Hub:          hub,
		Clock:        source,
		Config: trace.Config{
			FlushInterval:   config.TraceFlushInterval,
			MaxFrameChars:   config.TraceMaxFrameChars,
			MaxPreviewChars: config.TraceMaxPreviewChars,
			MaxStoredChars:  config.TraceMaxStoredChars,
			WriteTimeout:    config.TraceWriteTimeout,
		},
	})
	if err := recorder.ResolveInterrupted(storeCtx); err != nil {
		return nil, errors.Join(err, opened.Close())
	}

	resolver := credentials.NewResolver(credentials.Config{
		EnvVar:         credentials.EnvVar,
		StorePath:      config.CredentialStorePath(),
		FilePath:       config.CredentialFile,
		RefreshTimeout: config.CredentialRefreshTimeout,
	}, &http.Client{Timeout: config.CredentialRefreshTimeout}, source)

	runtime := &Runtime{
		Config:      config,
		Clock:       source,
		Store:       opened,
		Hub:         hub,
		Recorder:    recorder,
		Credentials: resolver,
		Claude: claude.NewClient(resolver, recorder, &http.Client{}, claude.Config{
			Model:           claude.Model,
			MaxOutputTokens: claude.MaxOutputTokens,
			RequestTimeout:  config.ModelRequestTimeout,
			MaxAttempts:     config.ModelMaxAttempts,
			RetryBackoff:    config.ModelRetryBackoff,
			MaxNudges:       config.ModelMaxNudges,
			Endpoint:        config.ModelEndpoint,
		}, source),
	}

	runtime.Resumes = resumes.NewService(resumes.Dependencies{
		Reads: opened.Reads(),
		Clock: source,
	})
	runtime.Settings = settings.NewService(settings.Dependencies{
		Reads:     opened.Reads(),
		Hub:       hub,
		Languages: runtime.Resumes,
		Names:     runtime.Resumes,
		Clock:     source,
	})
	runtime.Indexer = indexer.NewService(indexer.Dependencies{
		Resumes: runtime.Resumes,
		Claude:  runtime.Claude,
		Reads:   opened.Reads(),
		Hub:     hub,
		Clock:   source,
		Config: indexer.Config{
			Binary:            config.PDFToTextBinary,
			ExtractTimeout:    config.ExtractTimeout,
			MaxOutputBytes:    config.ExtractMaxOutputBytes,
			MaxCharsPerResume: config.ExtractMaxCharsPerResume,
			MaxCharsPerCall:   config.IndexMaxCharsPerCall,
			RunBudget:         config.IndexBudget,
		},
	})
	runtime.Plugin = plugin.NewService(plugin.Dependencies{
		Reads:  opened.Reads(),
		Hub:    hub,
		Clock:  source,
		Config: plugin.Config{ConnectedWindow: config.PluginConnectedWindow},
	})
	runtime.Postings = postings.NewService(postings.Dependencies{
		Reads:        opened.Reads(),
		Transactions: opened,
		Settings:     runtime.Settings,
		Hub:          hub,
		Clock:        source,
	})
	runtime.Planner = planner.NewService(planner.Dependencies{
		Reads:        opened.Reads(),
		Transactions: opened,
		Claude:       runtime.Claude,
		Settings:     runtime.Settings,
		Plugin:       runtime.Plugin,
		Profiles:     runtime.Indexer,
		Digest:       runtime.Postings,
		Hub:          hub,
		Clock:        source,
		Config: planner.Config{
			HistoryRounds:    config.PlannerHistoryRounds,
			NoteCap:          config.PlannerNoteCap,
			KnownIDWindow:    config.PlannerKnownIDWindow,
			KnownIDCap:       config.PlannerKnownIDCap,
			OperationTimeout: config.PlannerOperationTimeout,
			PlanBudget:       config.PlannerBudget,
		},
	})
	runtime.Screening = screening.NewService(screening.Dependencies{
		Reads:        opened.Reads(),
		Transactions: opened,
		Claude:       runtime.Claude,
		Settings:     runtime.Settings,
		Profiles:     runtime.Indexer,
		Postings:     runtime.Postings,
		Rounds:       runtime.Planner,
		Hub:          hub,
		Clock:        source,
		Config: screening.Config{
			ScreenBudget:      config.ScreenBudget,
			DescriptionWindow: config.DescriptionWindow,
			PendingLimit:      config.ScreeningPendingLimit,
		},
	})
	runtime.Postings.UseRounds(runtime.Planner)
	runtime.Postings.UseScreenKeys(runtime.Screening)
	runtime.Traces = traceroutes.NewService(traceroutes.Dependencies{Reads: opened.Reads()})

	runtime.credentials = newCredentialHandler(runtime)
	runtime.traces = newTraceHandler(runtime)
	runtime.settings = newSettingsHandler(runtime)
	runtime.setup = newSetupHandler(runtime)
	runtime.resumes = newResumeHandler(runtime)
	runtime.indexer = newIndexerHandler(runtime)
	runtime.plugin = newPluginHandler(runtime)
	runtime.postings = newPostingsHandler(runtime)
	runtime.planner = newPlannerHandler(runtime)
	runtime.screening = newScreeningHandler(runtime)
	runtime.preview = newPreviewHandler(runtime)
	runtime.stream = newStreamHandler(runtime)
	runtime.Server = newServer(config)
	registerRoutes(runtime.Server, runtime)
	return runtime, nil
}

func (r *Runtime) Close() error {
	r.Plugin.Close()
	r.Hub.Close()
	return r.Store.Close()
}

func Run() error {
	config, err := LoadConfig()
	if err != nil {
		return err
	}
	setupLogger(config)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runtime, err := Build(ctx, config)
	if err != nil {
		return err
	}

	slog.Info("api listening", "addr", config.ListenAddr(), "data", config.DataDir)
	listenErr := runtime.Server.Listen(config.ListenAddr(), listenConfig(ctx, config))
	stop()
	return errors.Join(listenErr, runtime.Close())
}

func listenConfig(ctx context.Context, config Config) fiber.ListenConfig {
	return fiber.ListenConfig{
		DisableStartupMessage: true,
		GracefulContext:       ctx,
		ShutdownTimeout:       config.HTTPShutdownWindow,
	}
}

func setupLogger(config Config) {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: config.LogLevel,
	})))
}
