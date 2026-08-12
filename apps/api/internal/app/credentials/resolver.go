package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"api/internal/app/clock"
)

const (
	SourceEnv   = "env"
	SourceStore = "store"
	SourceFile  = "file"

	EnvVar    = "CLAUDE_CREDENTIALS"
	StoreName = "credentials.json"

	oauthTokenURL = "https://console.anthropic.com/v1/oauth/token"
	oauthClientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"

	refreshSkew        = time.Minute
	defaultTokenWindow = 8 * time.Hour
	secretFileMode     = os.FileMode(0o600)
	secretDirMode      = os.FileMode(0o700)
)

var sourceOrder = []string{SourceEnv, SourceStore, SourceFile}

var sourceDescription = map[string]string{
	SourceEnv:   "the " + EnvVar + " environment variable",
	SourceStore: "the credentials pasted into setup",
	SourceFile:  "the claude code login on this machine",
}

type Config struct {
	EnvVar         string
	StorePath      string
	FilePath       string
	RefreshTimeout time.Duration
}

type Resolved struct {
	Source string
	Block  Block
}

type Status struct {
	Loaded           bool    `json:"loaded"`
	Source           *string `json:"source"`
	Where            *string `json:"where"`
	Stored           bool    `json:"stored"`
	ExpiresInMinutes *int64  `json:"expiresInMinutes"`
	Refreshable      bool    `json:"refreshable"`
	Error            *string `json:"error"`
}

type Resolver struct {
	config    Config
	http      *http.Client
	tokenURL  string
	clock     clock.Clock
	lookupEnv func(string) (string, bool)

	mu     sync.Mutex
	cached *Resolved
}

func NewResolver(config Config, httpClient *http.Client, source clock.Clock) *Resolver {
	if config.EnvVar == "" {
		config.EnvVar = EnvVar
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Resolver{
		config:    config,
		http:      httpClient,
		tokenURL:  oauthTokenURL,
		clock:     source,
		lookupEnv: os.LookupEnv,
	}
}

func (r *Resolver) Resolve() (Resolved, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.resolveLocked()
}

func (r *Resolver) resolveLocked() (Resolved, error) {
	if r.cached != nil {
		return *r.cached, nil
	}
	for _, source := range sourceOrder {
		block, present, err := r.read(source)
		if err != nil {
			return Resolved{}, refuse(sourceDescription[source] + " cannot be read: " + err.Error())
		}
		if !present {
			continue
		}
		resolved := Resolved{Source: source, Block: block}
		r.cached = &resolved
		return resolved, nil
	}
	return Resolved{}, ErrMissing
}

func (r *Resolver) read(source string) (Block, bool, error) {
	if source == SourceEnv {
		raw, present := r.lookupEnv(r.config.EnvVar)
		if !present || strings.TrimSpace(raw) == "" {
			return Block{}, false, nil
		}
		block, err := Parse(raw, r.config.EnvVar)
		return block, err == nil, err
	}
	raw, err := os.ReadFile(r.pathFor(source))
	switch {
	case errors.Is(err, os.ErrNotExist):
		return Block{}, false, nil
	case err != nil:
		return Block{}, false, fmt.Errorf("read %s: %w", source, err)
	}
	block, parseErr := Parse(string(raw), r.config.EnvVar)
	return block, parseErr == nil, parseErr
}

func (r *Resolver) pathFor(source string) string {
	if source == SourceStore {
		return r.config.StorePath
	}
	return r.config.FilePath
}

func (r *Resolver) AccessToken(ctx context.Context) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	resolved, err := r.resolveLocked()
	if err != nil {
		return "", err
	}
	if !r.expiring(resolved.Block) {
		return resolved.Block.AccessToken, nil
	}
	if fresh, present, readErr := r.read(resolved.Source); readErr == nil && present {
		if !r.expiring(fresh) {
			r.cached = &Resolved{Source: resolved.Source, Block: fresh}
			return fresh.AccessToken, nil
		}
		resolved.Block = fresh
	}
	refreshed, err := r.refresh(ctx, resolved.Block)
	if err != nil {
		return "", err
	}
	if err := r.persistLocked(refreshed, resolved.Source); err != nil {
		return "", err
	}
	return refreshed.AccessToken, nil
}

func (r *Resolver) expiring(block Block) bool {
	if block.ExpiresAt == 0 {
		return false
	}
	expiry := time.UnixMilli(block.ExpiresAt)
	return expiry.Sub(r.clock.Now()) < refreshSkew
}

func (r *Resolver) refresh(ctx context.Context, block Block) (Block, error) {
	if !block.Refreshable() {
		return Block{}, errors.New("the claude token expired and carries no refresh token")
	}
	if r.config.RefreshTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.config.RefreshTimeout)
		defer cancel()
	}

	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {block.RefreshToken},
		"client_id":     {oauthClientID},
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		r.tokenURL,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return Block{}, fmt.Errorf("build oauth refresh request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")

	response, err := r.http.Do(request)
	if err != nil {
		return Block{}, errors.New("the claude token refresh could not reach console.anthropic.com")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return Block{}, errors.New(
			"the claude token refresh was rejected with status " + strconv.Itoa(response.StatusCode),
		)
	}

	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return Block{}, errors.New("the claude token refresh returned something that is not json")
	}
	if payload.AccessToken == "" {
		return Block{}, errors.New("the claude token refresh returned no access token")
	}
	window := time.Duration(payload.ExpiresIn) * time.Second
	if window <= 0 {
		window = defaultTokenWindow
	}
	refreshToken := payload.RefreshToken
	if refreshToken == "" {
		refreshToken = block.RefreshToken
	}
	return block.withTokens(
		payload.AccessToken,
		refreshToken,
		r.clock.Now().Add(window).UnixMilli(),
	), nil
}

func (r *Resolver) persistLocked(block Block, source string) error {
	resolved := Resolved{Source: source, Block: block}
	r.cached = &resolved
	if source == SourceEnv {
		return nil
	}
	return writeSecret(r.pathFor(source), block)
}

func (r *Resolver) Store(raw string) (Status, error) {
	block, err := Parse(raw, r.config.EnvVar)
	if err != nil {
		return Status{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := writeSecret(r.config.StorePath, block); err != nil {
		return Status{}, err
	}
	r.cached = nil
	return r.statusLocked(), nil
}

func (r *Resolver) Clear() (Status, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := os.Remove(r.config.StorePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Status{}, fmt.Errorf("remove stored credentials: %w", err)
	}
	r.cached = nil
	return r.statusLocked(), nil
}

func (r *Resolver) Status() Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.statusLocked()
}

func (r *Resolver) statusLocked() Status {
	stored := fileExists(r.config.StorePath)
	resolved, err := r.resolveLocked()
	if err != nil {
		message := Scrub(err.Error())
		return Status{Stored: stored, Error: &message}
	}
	source := resolved.Source
	where := sourceDescription[source]
	status := Status{
		Loaded:      true,
		Source:      &source,
		Where:       &where,
		Stored:      stored,
		Refreshable: resolved.Block.Refreshable(),
	}
	if resolved.Block.ExpiresAt != 0 {
		minutes := int64(
			time.UnixMilli(resolved.Block.ExpiresAt).Sub(r.clock.Now()).Round(time.Minute) /
				time.Minute)
		status.ExpiresInMinutes = &minutes
	}
	return status
}

func writeSecret(path string, block Block) error {
	if err := os.MkdirAll(filepath.Dir(path), secretDirMode); err != nil {
		return fmt.Errorf("create credential directory: %w", err)
	}
	body, err := json.MarshalIndent(map[string]*Block{fieldOAuthBlock: &block}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode credentials: %w", err)
	}
	if err := os.WriteFile(path, append(body, '\n'), secretFileMode); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}
	if err := os.Chmod(path, secretFileMode); err != nil {
		return fmt.Errorf("restrict credentials: %w", err)
	}
	return nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
