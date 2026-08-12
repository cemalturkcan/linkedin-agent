package plugin

import (
	"time"

	"api/internal/routes/settings"
)

const (
	ConnectedWindow = 15 * time.Minute

	defaultPageSize  = 25
	defaultMaxPages  = 8
	maxPageSize      = 100
	maxPagesPerQuery = 20
	unknownVersion   = "unknown"
)

var (
	KnownRanges = []string{"r3600", "r21600", "r86400", "r604800"}
	KnownSorts  = []string{"DD"}
	KnownRings  = settings.PlaceRings

	RangeLabels = map[string]string{
		"r3600":   "the last hour",
		"r21600":  "the last 6 hours",
		"r86400":  "the last 24 hours",
		"r604800": "the last week",
	}
)

type Fetch struct {
	Place            bool     `json:"place"`
	Rings            []string `json:"rings"`
	Ranges           []string `json:"ranges"`
	EasyApply        bool     `json:"easyApply"`
	RemoteOnly       bool     `json:"remoteOnly"`
	Sorts            []string `json:"sorts"`
	Keyword          bool     `json:"keyword"`
	PageSize         int      `json:"pageSize"`
	MaxPagesPerQuery int      `json:"maxPagesPerQuery"`
}

type Capabilities struct {
	Fetch      Fetch `json:"fetch"`
	SeenSet    bool  `json:"seenSet"`
	AutoAttach bool  `json:"autoAttach"`
}

type State struct {
	EverSeen     bool          `json:"everSeen"`
	Connected    bool          `json:"connected"`
	ID           string        `json:"id"`
	Version      *string       `json:"version"`
	FirstSeen    *string       `json:"firstSeen"`
	LastSeen     *string       `json:"lastSeen"`
	Hellos       int           `json:"hellos"`
	Capabilities *Capabilities `json:"capabilities"`
}

type Hello struct {
	ID           string          `json:"id"`
	Version      string          `json:"version"`
	Capabilities RawCapabilities `json:"capabilities"`
}

type RawCapabilities struct {
	Fetch      RawFetch `json:"fetch"`
	SeenSet    *bool    `json:"seenSet"`
	AutoAttach *bool    `json:"autoAttach"`
}

type RawFetch struct {
	Place            *bool    `json:"place"`
	Rings            []string `json:"rings"`
	Ranges           []string `json:"ranges"`
	EasyApply        *bool    `json:"easyApply"`
	RemoteOnly       *bool    `json:"remoteOnly"`
	Sorts            []string `json:"sorts"`
	Keyword          *bool    `json:"keyword"`
	PageSize         *int     `json:"pageSize"`
	MaxPagesPerQuery *int     `json:"maxPagesPerQuery"`
}
