package store

import (
	"errors"
	"time"
)

// ErrNotFound is returned when an operation targets an app ID that is not tracked.
var ErrNotFound = errors.New("app not found")

// ErrAlreadyExists is returned when adding an app that is already tracked.
var ErrAlreadyExists = errors.New("app already tracked")

// PollerStatus represents the current state of the background poller for an app.
type PollerStatus string

const (
	PollerStatusPending PollerStatus = "pending" // added but not yet polled
	PollerStatusOK      PollerStatus = "ok"
	PollerStatusError   PollerStatus = "error"
)

// PollerMeta holds per-app poller state persisted alongside app metadata.
// Intentionally minimal: the failure reason is NOT exposed here because the
// error string can leak filesystem paths / internal details over the wire.
// Operators check logs to diagnose a red poller status.
type PollerMeta struct {
	Status        PollerStatus `json:"status"`
	LastRefreshAt time.Time    `json:"lastRefreshAt"`
}

type App struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	RSSUrl       string     `json:"rssUrl"`
	Poller       PollerMeta `json:"poller"`
	TotalReviews int        `json:"totalReviews"`
}

type Review struct {
	ID          string    `json:"id"`
	AppID       string    `json:"appId"`
	Score       int       `json:"score"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	Author      string    `json:"author"`
	Version     string    `json:"version"`
	SubmittedAt time.Time `json:"submittedAt"`
}

// AppResponse is the shape returned by the API. Currently a thin wrapper
// over App — kept so future per-response computed fields have a home that
// doesn't pollute the persisted struct.
type AppResponse struct {
	App
}
