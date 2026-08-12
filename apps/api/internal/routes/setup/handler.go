package setup

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	appcredentials "api/internal/app/credentials"
	"api/internal/app/events"
	"api/internal/app/httpx"
	"api/internal/routes/indexer"
	appresumes "api/internal/routes/resumes"
)

const (
	missingCredentials = "claude credentials, which nothing is read or screened without"
	missingFolder      = "a folder holding your cv pdfs"
	missingFirstRead   = "a first read of those cvs"
)

type Config struct {
	OperationTimeout time.Duration
	IndexBudget      time.Duration
}

type Handler struct {
	resumes     *appresumes.Service
	indexer     *indexer.Service
	credentials *appcredentials.Resolver
	hub         *events.Hub
	config      Config
}

func NewHandler(
	folder *appresumes.Service,
	index *indexer.Service,
	resolver *appcredentials.Resolver,
	hub *events.Hub,
	config Config,
) *Handler {
	return &Handler{
		resumes:     folder,
		indexer:     index,
		credentials: resolver,
		hub:         hub,
		config:      config,
	}
}

type state struct {
	Configured  bool                   `json:"configured"`
	Ready       bool                   `json:"ready"`
	Missing     []string               `json:"missing"`
	IndexState  string                 `json:"indexState"`
	Indexing    bool                   `json:"indexing"`
	ResumeDir   string                 `json:"resumeDir"`
	Candidates  []appresumes.Candidate `json:"candidates"`
	Resumes     []appresumes.Resume    `json:"resumes"`
	Credentials appcredentials.Status  `json:"credentials"`
	Progress    indexer.Progress       `json:"progress"`
	Error       string                 `json:"error,omitempty"`
}

type folderRequest struct {
	Dir string `json:"dir"`
}

type announcement struct {
	ResumeDir  string   `json:"resumeDir"`
	Resumes    int      `json:"resumes"`
	Configured bool     `json:"configured"`
	Ready      bool     `json:"ready"`
	Indexing   bool     `json:"indexing"`
	Missing    []string `json:"missing"`
}

func (h *Handler) Get(c fiber.Ctx) error {
	ctx, cancel := httpx.BoundedRequestContext(c, c.Context(), h.config.OperationTimeout)
	defer cancel()

	return httpx.Respond(c, h.state(ctx))
}

func (h *Handler) Post(c fiber.Ctx) error {
	var body folderRequest
	if appErr := httpx.DecodeObject(c, &body); appErr != nil {
		return appErr
	}
	if strings.TrimSpace(body.Dir) == "" {
		return httpx.ErrBadRequest.WithMessage("dir must name the folder holding your cv pdfs")
	}
	ctx, cancel := httpx.BoundedRequestContext(c, c.Context(), h.config.OperationTimeout)
	defer cancel()

	if _, _, err := h.resumes.SetFolder(ctx, body.Dir); err != nil {
		return refusal(err)
	}
	h.indexer.Start(context.WithoutCancel(c.Context()), false)

	current := h.state(ctx)
	h.hub.Emit(events.TypeSetup, announcement{
		ResumeDir:  current.ResumeDir,
		Resumes:    len(current.Resumes),
		Configured: current.Configured,
		Ready:      current.Ready,
		Indexing:   current.Indexing,
		Missing:    current.Missing,
	})
	return httpx.Respond(c, current)
}

func (h *Handler) state(ctx context.Context) state {
	credentials := h.credentials.Status()
	profile := h.indexer.State(ctx)
	_, found, err := h.resumes.List(ctx)

	missing := make([]string, 0, 3)
	if !credentials.Loaded {
		missing = append(missing, missingCredentials)
	}
	switch {
	case len(found) == 0:
		missing = append(missing, missingFolder)
	case profile.IndexState == indexer.StateNever && !profile.Indexing:
		missing = append(missing, missingFirstRead)
	}

	current := state{
		Configured:  len(found) > 0,
		Ready:       len(missing) == 0 && !profile.Indexing,
		Missing:     missing,
		IndexState:  profile.IndexState,
		Indexing:    profile.Indexing,
		ResumeDir:   profile.ResumeDir,
		Candidates:  appresumes.Candidates(),
		Resumes:     found,
		Credentials: credentials,
		Progress:    profile.Progress,
	}
	if current.Resumes == nil {
		current.Resumes = []appresumes.Resume{}
	}
	if err != nil && !errors.Is(err, appresumes.ErrNoFolder) {
		current.Error = err.Error()
	}
	return current
}

func refusal(err error) error {
	var folder *appresumes.FolderError
	switch {
	case errors.As(err, &folder):
		return httpx.ErrBadRequest.WithMessage(folder.Error()).WithCause(err)
	case errors.Is(err, appresumes.ErrNoPDFs):
		return httpx.ErrBadRequest.WithMessage(err.Error()).WithCause(err)
	}
	return httpx.ErrInternal.WithCause(err)
}
