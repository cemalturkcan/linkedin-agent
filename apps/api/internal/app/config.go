package app

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"api/internal/app/claude"
	"api/internal/app/credentials"
	"api/internal/app/trace"
	"api/internal/routes/indexer"
	"api/internal/routes/planner"
	"api/internal/routes/plugin"
)

const (
	loopbackHost     = "127.0.0.1"
	defaultPort      = 8787
	storeName        = "agent.db"
	claudeDirectory  = ".claude"
	claudeCredential = ".credentials.json"
)

type Config struct {
	Port           int
	DataDir        string
	CredentialFile string
	LogLevel       slog.Level

	HTTPBodyLimitBytes int
	HTTPReadTimeout    time.Duration
	HTTPIdleTimeout    time.Duration
	HTTPShutdownWindow time.Duration

	StoreOpenTimeout time.Duration

	TraceFlushInterval   time.Duration
	TraceMaxFrameChars   int
	TraceMaxPreviewChars int
	TraceMaxStoredChars  int
	TraceWriteTimeout    time.Duration

	TracesOperationTimeout time.Duration
	TracesPageLimitDefault int
	TracesPageLimitMax     int

	SettingsOperationTimeout time.Duration
	SetupOperationTimeout    time.Duration
	ResumesOperationTimeout  time.Duration
	PluginOperationTimeout   time.Duration
	PostingsOperationTimeout time.Duration

	ScreeningOperationTimeout time.Duration
	ScreeningPendingLimit     int
	ScreenBudget              time.Duration
	DescriptionWindow         time.Duration

	PluginConnectedWindow time.Duration

	PlannerHistoryRounds    int
	PlannerNoteCap          int
	PlannerKnownIDWindow    time.Duration
	PlannerKnownIDCap       int
	PlannerOperationTimeout time.Duration
	PlannerBudget           time.Duration

	PDFToTextBinary          string
	ExtractTimeout           time.Duration
	ExtractMaxOutputBytes    int
	ExtractMaxCharsPerResume int
	IndexMaxCharsPerCall     int
	IndexBudget              time.Duration

	StreamRetry     time.Duration
	StreamHeartbeat time.Duration

	CredentialRefreshTimeout time.Duration

	ModelRequestTimeout time.Duration
	ModelMaxAttempts    int
	ModelRetryBackoff   time.Duration
	ModelMaxNudges      int
	ModelEndpoint       string
}

func LoadConfig() (Config, error) {
	config := defaults()

	if raw, present := os.LookupEnv("PORT"); present {
		port, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || port < 1 || port > 65535 {
			return Config{}, errors.New("PORT must be a number between 1 and 65535")
		}
		config.Port = port
	}
	if raw, present := os.LookupEnv("DATA_DIR"); present && strings.TrimSpace(raw) != "" {
		config.DataDir = strings.TrimSpace(raw)
	}
	if raw, present := os.LookupEnv("CLAUDE_CREDENTIALS_FILE"); present &&
		strings.TrimSpace(raw) != "" {
		config.CredentialFile = strings.TrimSpace(raw)
	}
	if raw, present := os.LookupEnv("PDFTOTEXT_BIN"); present && strings.TrimSpace(raw) != "" {
		config.PDFToTextBinary = strings.TrimSpace(raw)
	}
	if raw, present := os.LookupEnv("LOG_LEVEL"); present && strings.TrimSpace(raw) != "" {
		if err := config.LogLevel.UnmarshalText([]byte(strings.TrimSpace(raw))); err != nil {
			return Config{}, errors.New("LOG_LEVEL must be debug, info, warn or error")
		}
	}

	absolute, err := filepath.Abs(config.DataDir)
	if err != nil {
		return Config{}, errors.New("DATA_DIR is not a usable path")
	}
	config.DataDir = absolute
	return config, nil
}

func defaults() Config {
	return Config{
		Port:           defaultPort,
		DataDir:        defaultDataDir(),
		CredentialFile: defaultCredentialFile(),
		LogLevel:       slog.LevelInfo,

		HTTPBodyLimitBytes: 4 << 20,
		HTTPReadTimeout:    10 * time.Minute,
		HTTPIdleTimeout:    30 * time.Minute,
		HTTPShutdownWindow: 5 * time.Second,

		StoreOpenTimeout: 2 * time.Minute,

		TraceFlushInterval:   125 * time.Millisecond,
		TraceMaxFrameChars:   8_000,
		TraceMaxPreviewChars: 64_000,
		TraceMaxStoredChars:  262_144,
		TraceWriteTimeout:    2 * time.Minute,

		TracesOperationTimeout: 2 * time.Minute,
		TracesPageLimitDefault: trace.ListDefault,
		TracesPageLimitMax:     trace.Retention,

		SettingsOperationTimeout: 2 * time.Minute,
		SetupOperationTimeout:    5 * time.Minute,
		ResumesOperationTimeout:  5 * time.Minute,
		PluginOperationTimeout:   2 * time.Minute,
		PostingsOperationTimeout: 5 * time.Minute,

		ScreeningOperationTimeout: 2 * time.Hour,
		ScreeningPendingLimit:     40,
		ScreenBudget:              45 * time.Minute,
		DescriptionWindow:         10 * time.Minute,

		PluginConnectedWindow: plugin.ConnectedWindow,

		PlannerHistoryRounds:    planner.HistoryRounds,
		PlannerNoteCap:          planner.NoteCap,
		PlannerKnownIDWindow:    14 * 24 * time.Hour,
		PlannerKnownIDCap:       1500,
		PlannerOperationTimeout: 1 * time.Hour,
		PlannerBudget:           20 * time.Minute,

		PDFToTextBinary:          indexer.DefaultBinary,
		ExtractTimeout:           5 * time.Minute,
		ExtractMaxOutputBytes:    8 << 20,
		ExtractMaxCharsPerResume: 20_000,
		IndexMaxCharsPerCall:     300_000,
		IndexBudget:              45 * time.Minute,

		StreamRetry:     3 * time.Second,
		StreamHeartbeat: 20 * time.Second,

		CredentialRefreshTimeout: 5 * time.Minute,

		ModelRequestTimeout: 2 * time.Hour,
		ModelMaxAttempts:    3,
		ModelRetryBackoff:   2 * time.Second,
		ModelMaxNudges:      claude.DefaultMaxNudges,
	}
}

func defaultDataDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".local", "share", "linkedin-agent")
	}
	return filepath.Join(".", "data")
}

func defaultCredentialFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(claudeDirectory, claudeCredential)
	}
	return filepath.Join(home, claudeDirectory, claudeCredential)
}

func (c Config) ListenAddr() string {
	return loopbackHost + ":" + strconv.Itoa(c.Port)
}

func (c Config) StorePath() string {
	return filepath.Join(c.DataDir, storeName)
}

func (c Config) CredentialStorePath() string {
	return filepath.Join(c.DataDir, credentials.StoreName)
}
