// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package host

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/contracts/ops"
	"github.com/nauarch/limen/internal/contracts/prefetch"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// Engine is a scripted Docker engine: Current is what Info says, AfterRestart (when set) what it
// says once Docker restarted; images saved and loaded are noted.
type Engine struct {
	mu            sync.Mutex
	Current       host.Info
	AfterRestart  *host.Info
	ContainerList []host.Container
	ImageList     []host.Image
	Saved         []string
	SavedTo       string
	Loaded        string
	Pulled        []string
	RemovedImages []string
	// OnPull runs on each pull: a test makes it count requests at its fake limen.
	OnPull func()
	Err    error
}

// Info answers the scripted info.
func (e *Engine) Info(context.Context) (host.Info, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.Current, e.Err
}

// Containers answers the scripted containers.
func (e *Engine) Containers(context.Context) ([]host.Container, error) { return e.ContainerList, e.Err }

// Images answers the scripted images.
func (e *Engine) Images(context.Context) ([]host.Image, error) { return e.ImageList, e.Err }

// Pull records the pull and runs OnPull.
func (e *Engine) Pull(_ context.Context, ref string) error {
	e.Pulled = append(e.Pulled, ref)
	if e.OnPull != nil {
		e.OnPull()
	}
	return e.Err
}

// RemoveImage records the removal.
func (e *Engine) RemoveImage(_ context.Context, ref string) error {
	e.RemovedImages = append(e.RemovedImages, ref)
	return e.Err
}

// SaveImages records the images saved, and to where.
func (e *Engine) SaveImages(_ context.Context, refs []string, path string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Saved, e.SavedTo = refs, path
	return e.Err
}

// LoadImages records the file loaded.
func (e *Engine) LoadImages(_ context.Context, path string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Loaded = path
	return e.Err
}

// PruneImages drops the images no container uses.
func (e *Engine) PruneImages(context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ImageList = slices.DeleteFunc(e.ImageList, func(im host.Image) bool {
		return !slices.ContainsFunc(e.ContainerList, func(c host.Container) bool { return c.ImageID == im.ID })
	})
	return e.Err
}

// Files is an in-memory filesystem by absolute path, with free space per path; its files are
// executable but for those in Plain.
type Files struct {
	mu      sync.Mutex
	Data    map[string][]byte
	FreeAt  map[string]int64
	Plain   []string
	Written []string
	Removed []string
	Made    []string
}

// NewFiles is a filesystem holding data.
func NewFiles(data map[string]string) *Files {
	f := &Files{Data: map[string][]byte{}, FreeAt: map[string]int64{}}
	for p, d := range data {
		f.Data[p] = []byte(d)
	}
	return f
}

// Read reads a file.
func (f *Files) Read(path string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.Data[path]
	if !ok {
		return nil, fmt.Errorf("%s: %w", path, errkind.ErrNotFound)
	}
	return slices.Clone(d), nil
}

// Write writes a file and notes it.
func (f *Files) Write(path string, data []byte, _ uint32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Data[path] = slices.Clone(data)
	f.Written = append(f.Written, path)
	return nil
}

// List names the entries directly in a folder.
func (f *Files) List(dir string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	found := false
	for p := range f.Data {
		rest, ok := strings.CutPrefix(p, strings.TrimSuffix(dir, "/")+"/")
		if !ok {
			continue
		}
		found = true
		name, _, _ := strings.Cut(rest, "/")
		if !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	if !found {
		return nil, fmt.Errorf("%s: %w", dir, errkind.ErrNotFound)
	}
	slices.Sort(out)
	return out, nil
}

// Free answers the scripted free space, -1 when none is set.
func (f *Files) Free(path string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if n, ok := f.FreeAt[path]; ok {
		return n, nil
	}
	return -1, nil
}

// Size adds up the files at or under path.
func (f *Files) Size(path string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var n int64
	found := false
	for p, d := range f.Data {
		if under(p, path) {
			n, found = n+int64(len(d)), true
		}
	}
	if !found {
		return 0, fmt.Errorf("%s: %w", path, errkind.ErrNotFound)
	}
	return n, nil
}

// MakeDir notes the folder: in memory, folders exist with what they hold.
func (f *Files) MakeDir(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Made = append(f.Made, path)
	return nil
}

// Executable tells whether a file is there and not Plain.
func (f *Files) Executable(path string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.Data[path]; !ok {
		return false, fmt.Errorf("%s: %w", path, errkind.ErrNotFound)
	}
	return !slices.Contains(f.Plain, path), nil
}

// Remove drops the files at or under path and notes it.
func (f *Files) Remove(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for p := range f.Data {
		if under(p, path) {
			delete(f.Data, p)
		}
	}
	f.Removed = append(f.Removed, path)
	return nil
}

// under reports whether p is path or lies within it.
func under(p, path string) bool {
	return p == path || strings.HasPrefix(p, strings.TrimSuffix(path, "/")+"/")
}

// Services records restarts (to Log too, as "restart <unit>"), and makes the engine answer as
// restarted.
type Services struct {
	Engine    *Engine
	Restarted []string
	Err       error
	Log       *Log
}

// Restart records the unit and, for docker, makes the engine answer as restarted.
func (s *Services) Restart(_ context.Context, unit string) error {
	s.Restarted = append(s.Restarted, unit)
	s.Log.add("restart " + unit)
	if s.Err == nil && unit == "docker" && s.Engine != nil && s.Engine.AfterRestart != nil {
		s.Engine.mu.Lock()
		s.Engine.Current = *s.Engine.AfterRestart
		s.Engine.mu.Unlock()
	}
	return s.Err
}

// Limen is a scripted limen. A prefetch it is asked for is recorded and runs until asked about
// once, then ends as Fetched says (done when unset).
type Limen struct {
	Health  error
	Seen    int64
	Served  []string
	Fetched *prefetch.Job
	Asked   []prefetch.Request
}

// Me answers the requests counted from this host so far, Seen.
func (l *Limen) Me(context.Context) (ops.Client, error) {
	return ops.Client{Address: "192.0.2.7", Requests: l.Seen}, l.Health
}

// Healthy answers the scripted health.
func (l *Limen) Healthy(context.Context) error { return l.Health }

// Registries answers the scripted registries, or the health failure.
func (l *Limen) Registries(context.Context) ([]string, error) { return l.Served, l.Health }

// Prefetch records the request and answers a running job.
func (l *Limen) Prefetch(_ context.Context, r prefetch.Request) (prefetch.Job, error) {
	l.Asked = append(l.Asked, r)
	return prefetch.Job{ID: "j1", State: prefetch.Running}, l.Health
}

// Job answers how the prefetch ended.
func (l *Limen) Job(context.Context, string) (prefetch.Job, error) {
	if l.Fetched != nil {
		return *l.Fetched, nil
	}
	return prefetch.Job{ID: "j1", State: prefetch.Done}, nil
}

// Client gives l whatever URL is asked, for options wanting a limen per URL.
func (l *Limen) Client(string) host.Limen { return l }

// Network answers scripted addresses.
type Network struct {
	Own []string
}

// Addresses answers the scripted addresses.
func (n Network) Addresses() ([]string, error) { return n.Own, nil }

// Log records what fakes were asked to do, in order, across fakes sharing it.
type Log struct {
	mu     sync.Mutex
	Events []string
}

// add records an event; a nil log records nothing.
func (l *Log) add(event string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Events = append(l.Events, event)
}

// Compose records "stop <project>", "up <project>" and "pull <project>", failing the one equal to
// Fail; Missing makes it a host without the compose plugin.
type Compose struct {
	Log     *Log
	Fail    string
	Missing bool
}

// Version answers a compose version, or fails as a host without the plugin does when Missing.
func (c *Compose) Version(context.Context) (string, error) {
	if c.Missing {
		return "", fmt.Errorf("docker: 'compose' is not a docker command: %w", errkind.ErrUpstream)
	}
	return "2.40.0", nil
}

// Stop records the stop.
func (c *Compose) Stop(_ context.Context, p host.Project) error { return c.do("stop", p) }

// Up records the start.
func (c *Compose) Up(_ context.Context, p host.Project) error { return c.do("up", p) }

// Pull records the pull.
func (c *Compose) Pull(_ context.Context, p host.Project) error { return c.do("pull", p) }

// do records a compose command, failing it when it is Fail.
func (c *Compose) do(command string, p host.Project) error {
	c.Log.add(command + " " + p.Name)
	if command+" "+p.Name == c.Fail {
		return fmt.Errorf("compose %s %s: %w", command, p.Name, errkind.ErrUpstream)
	}
	return nil
}

// Scripts records "run <path>", failing for the script at Fail.
type Scripts struct {
	Log  *Log
	Fail string
}

// Run records the script.
func (s *Scripts) Run(_ context.Context, path, _ string) error {
	s.Log.add("run " + path)
	if path == s.Fail {
		return fmt.Errorf("%s: %w", path, errkind.ErrUpstream)
	}
	return nil
}
