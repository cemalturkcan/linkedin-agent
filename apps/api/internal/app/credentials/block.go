package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
)

const (
	oauthPrefix  = "sk-ant-oat"
	apiKeyPrefix = "sk-ant-api"

	fieldAccessToken  = "accessToken"
	fieldRefreshToken = "refreshToken"
	fieldExpiresAt    = "expiresAt"
	fieldOAuthBlock   = "claudeAiOauth"
)

type RefusalError struct {
	Message string
}

func (r *RefusalError) Error() string {
	return r.Message
}

func refuse(message string) error {
	return &RefusalError{Message: message}
}

var ErrMissing = errors.New(
	"no claude credentials. paste them into setup, or sign in to claude code on this machine",
)

type Block struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    int64
	carried      map[string]json.RawMessage
}

func (b *Block) MarshalJSON() ([]byte, error) {
	fields := make(map[string]json.RawMessage, len(b.carried)+3)
	maps.Copy(fields, b.carried)
	setField(fields, fieldAccessToken, b.AccessToken != "", b.AccessToken)
	setField(fields, fieldRefreshToken, b.RefreshToken != "", b.RefreshToken)
	setField(fields, fieldExpiresAt, b.ExpiresAt != 0, b.ExpiresAt)
	encoded, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("encode credential block: %w", err)
	}
	return encoded, nil
}

func setField[T string | int64](
	fields map[string]json.RawMessage,
	name string,
	present bool,
	value T,
) {
	if !present {
		delete(fields, name)
		return
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		delete(fields, name)
		return
	}
	fields[name] = encoded
}

func (b *Block) UnmarshalJSON(raw []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return fmt.Errorf("decode credential block: %w", err)
	}
	block := Block{carried: make(map[string]json.RawMessage, len(fields))}
	for name, value := range fields {
		switch name {
		case fieldAccessToken:
			if err := json.Unmarshal(value, &block.AccessToken); err != nil {
				return fmt.Errorf("decode %s: %w", fieldAccessToken, err)
			}
		case fieldRefreshToken:
			if err := json.Unmarshal(value, &block.RefreshToken); err != nil {
				return fmt.Errorf("decode %s: %w", fieldRefreshToken, err)
			}
		case fieldExpiresAt:
			if err := json.Unmarshal(value, &block.ExpiresAt); err != nil {
				return fmt.Errorf("decode %s: %w", fieldExpiresAt, err)
			}
		default:
			block.carried[name] = value
		}
	}
	*b = block
	return nil
}

func (b *Block) Refreshable() bool {
	return b.RefreshToken != ""
}

func (b *Block) withTokens(access, refresh string, expiresAt int64) Block {
	updated := *b
	updated.AccessToken = access
	updated.RefreshToken = refresh
	updated.ExpiresAt = expiresAt
	return updated
}

func looksLikePath(value string) bool {
	return strings.HasPrefix(value, "/") ||
		strings.HasPrefix(value, "~") ||
		strings.HasPrefix(value, "./")
}

func Parse(raw string, envVar string) (Block, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return Block{}, refuse("nothing was pasted")
	}
	if !strings.HasPrefix(value, "{") {
		return bareToken(value, envVar)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(value), &fields); err != nil {
		return Block{}, refuse("that starts like json but does not parse. paste the whole file, unedited")
	}

	body := []byte(value)
	if nested, wrapped := fields[fieldOAuthBlock]; wrapped {
		body = nested
	}
	var block Block
	if err := json.Unmarshal(body, &block); err != nil {
		return Block{}, refuse("the credentials json has to be an object")
	}
	block.AccessToken = strings.TrimSpace(block.AccessToken)
	block.RefreshToken = strings.TrimSpace(block.RefreshToken)
	if block.AccessToken == "" {
		return Block{}, refuse("that json carries no accessToken, so it is not the claude credentials file")
	}
	if strings.HasPrefix(block.AccessToken, apiKeyPrefix) {
		return Block{}, refuse(
			"that json carries an api key. this service signs in as claude code, so it needs the claude code oauth credentials instead",
		)
	}
	return block, nil
}

func bareToken(value string, envVar string) (Block, error) {
	if looksLikePath(value) {
		return Block{}, refuse(
			envVar + " carries the credentials themselves, not the path to a file holding them",
		)
	}
	if strings.HasPrefix(value, apiKeyPrefix) {
		return Block{}, refuse(
			"that is an anthropic api key. this service signs in as claude code, so it needs the claude code oauth credentials instead",
		)
	}
	if !strings.HasPrefix(value, oauthPrefix) {
		return Block{}, refuse("that is neither the credentials json nor a claude code oauth token")
	}
	return Block{AccessToken: value}, nil
}
