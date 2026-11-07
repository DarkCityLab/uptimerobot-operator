/*
Copyright 2026 DarkCityLab.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package fake provides an in-memory implementation of uptimerobot.Client for
// use in controller and reconciler tests. It is concurrency-safe and lets tests
// inject errors for specific operations.
package fake

import (
	"context"
	"sync"

	ur "github.com/DarkCityLab/uptimerobot-operator/internal/uptimerobot"
)

// Client is an in-memory uptimerobot.Client. The zero value is ready to use.
type Client struct {
	mu       sync.Mutex
	monitors map[int64]*ur.Monitor
	nextID   int64

	// Hooks let tests simulate API behaviour. Any non-nil hook, if it returns a
	// non-nil error, short-circuits the corresponding method.
	CreateHook func(m *ur.Monitor) error
	UpdateHook func(id int64, m *ur.Monitor) error
	DeleteHook func(id int64) error
	GetHook    func(id int64) error

	// Call counters for assertions.
	Creates int
	Updates int
	Deletes int
	Pauses  int
	Starts  int
	Resets  int
}

var _ ur.Client = (*Client)(nil)

// New returns an initialised fake client.
func New() *Client {
	return &Client{monitors: map[int64]*ur.Monitor{}, nextID: 1000}
}

func (c *Client) ensure() {
	if c.monitors == nil {
		c.monitors = map[int64]*ur.Monitor{}
	}
	if c.nextID == 0 {
		c.nextID = 1000
	}
}

// Seed inserts a monitor directly (bypassing hooks) and returns its ID.
func (c *Client) Seed(m ur.Monitor) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensure()
	if m.ID == 0 {
		c.nextID++
		m.ID = c.nextID
	}
	cp := m
	c.monitors[m.ID] = &cp
	return m.ID
}

// Count returns the number of stored monitors.
func (c *Client) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.monitors)
}

func (c *Client) CreateMonitor(_ context.Context, m *ur.Monitor) (*ur.Monitor, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensure()
	c.Creates++
	if c.CreateHook != nil {
		if err := c.CreateHook(m); err != nil {
			return nil, err
		}
	}
	// Emulate duplicate detection by URL + friendly name.
	for _, existing := range c.monitors {
		if existing.URL == m.URL && existing.FriendlyName == m.FriendlyName {
			return nil, &ur.APIError{Code: ur.CodeDuplicateMonitor, HTTPStatus: 409, Message: "monitor already exists"}
		}
	}
	c.nextID++
	cp := *m
	cp.ID = c.nextID
	if cp.Status == "" {
		cp.Status = "UP"
	}
	c.monitors[cp.ID] = &cp
	out := cp
	return &out, nil
}

func (c *Client) GetMonitor(_ context.Context, id int64) (*ur.Monitor, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensure()
	if c.GetHook != nil {
		if err := c.GetHook(id); err != nil {
			return nil, err
		}
	}
	m, ok := c.monitors[id]
	if !ok {
		return nil, &ur.APIError{Code: ur.CodeNotFound, HTTPStatus: 404, Message: "not found"}
	}
	out := *m
	return &out, nil
}

func (c *Client) ListMonitors(_ context.Context, opts ur.ListOptions) ([]ur.Monitor, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensure()
	out := make([]ur.Monitor, 0, len(c.monitors))
	for _, m := range c.monitors {
		if opts.Search != "" && m.URL != opts.Search && m.FriendlyName != opts.Search {
			continue
		}
		out = append(out, *m)
	}
	return out, nil
}

func (c *Client) UpdateMonitor(_ context.Context, id int64, m *ur.Monitor) (*ur.Monitor, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensure()
	c.Updates++
	if c.UpdateHook != nil {
		if err := c.UpdateHook(id, m); err != nil {
			return nil, err
		}
	}
	existing, ok := c.monitors[id]
	if !ok {
		return nil, &ur.APIError{Code: ur.CodeNotFound, HTTPStatus: 404, Message: "not found"}
	}
	cp := *m
	cp.ID = id
	cp.Type = existing.Type // type is immutable
	if cp.Status == "" {
		cp.Status = existing.Status
	}
	c.monitors[id] = &cp
	out := cp
	return &out, nil
}

func (c *Client) DeleteMonitor(_ context.Context, id int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensure()
	c.Deletes++
	if c.DeleteHook != nil {
		if err := c.DeleteHook(id); err != nil {
			return err
		}
	}
	if _, ok := c.monitors[id]; !ok {
		return &ur.APIError{Code: ur.CodeNotFound, HTTPStatus: 404, Message: "not found"}
	}
	delete(c.monitors, id)
	return nil
}

func (c *Client) PauseMonitor(_ context.Context, id int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensure()
	c.Pauses++
	m, ok := c.monitors[id]
	if !ok {
		return &ur.APIError{Code: ur.CodeNotFound, HTTPStatus: 404, Message: "not found"}
	}
	m.Paused = true
	m.Status = "PAUSED"
	return nil
}

func (c *Client) StartMonitor(_ context.Context, id int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensure()
	c.Starts++
	m, ok := c.monitors[id]
	if !ok {
		return &ur.APIError{Code: ur.CodeNotFound, HTTPStatus: 404, Message: "not found"}
	}
	m.Paused = false
	m.Status = "UP"
	return nil
}

func (c *Client) ResetStats(_ context.Context, id int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensure()
	c.Resets++
	if _, ok := c.monitors[id]; !ok {
		return &ur.APIError{Code: ur.CodeNotFound, HTTPStatus: 404, Message: "not found"}
	}
	return nil
}

func (c *Client) UptimeStats(_ context.Context, id int64, _, _ string) (*ur.UptimeStats, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensure()
	if _, ok := c.monitors[id]; !ok {
		return nil, &ur.APIError{Code: ur.CodeNotFound, HTTPStatus: 404, Message: "not found"}
	}
	return &ur.UptimeStats{UptimePercentage: 100, From: "", To: ""}, nil
}

func (c *Client) ResponseTimeStats(_ context.Context, id int64, _, _ string) (*ur.ResponseTimeStats, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensure()
	if _, ok := c.monitors[id]; !ok {
		return nil, &ur.APIError{Code: ur.CodeNotFound, HTTPStatus: 404, Message: "not found"}
	}
	avg := 142.0
	return &ur.ResponseTimeStats{AvgMS: &avg, DataPoints: 1}, nil
}
