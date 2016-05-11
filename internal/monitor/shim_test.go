package monitor

import (
	"sync"
	"time"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/multiwatcher"
	jujutesting "github.com/juju/juju/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

func newJEMShimWithUpdateNotify(j jemInterface) jemShimWithUpdateNotify {
	return jemShimWithUpdateNotify{
		controllerStatsSet:        make(chan struct{}, 10),
		modelLifeSet:              make(chan struct{}, 10),
		leaseAcquired:             make(chan struct{}, 10),
		controllerAvailabilitySet: make(chan struct{}, 10),
		jemInterface:              j,
	}
}

type jemShimWithUpdateNotify struct {
	controllerStatsSet        chan struct{}
	modelLifeSet              chan struct{}
	leaseAcquired             chan struct{}
	controllerAvailabilitySet chan struct{}
	jemInterface
}

func (s jemShimWithUpdateNotify) Clone() jemInterface {
	s.jemInterface = s.jemInterface.Clone()
	return s
}

func (s jemShimWithUpdateNotify) assertNoEvent(c *gc.C) {
	var event string
	select {
	case <-s.controllerStatsSet:
		event = "controller stats"
	case <-s.modelLifeSet:
		event = "model life"
	case <-s.leaseAcquired:
		event = "lease acquired"

	case <-time.After(jujutesting.ShortWait):
		return
	}
	c.Fatalf("unexpected event received: %v", event)
}

func (s jemShimWithUpdateNotify) SetControllerStats(ctlPath params.EntityPath, stats *mongodoc.ControllerStats) error {
	if err := s.jemInterface.SetControllerStats(ctlPath, stats); err != nil {
		return err
	}
	s.controllerStatsSet <- struct{}{}
	return nil
}

func (s jemShimWithUpdateNotify) SetControllerUnavailableAt(ctlPath params.EntityPath, t time.Time) error {
	if err := s.jemInterface.SetControllerUnavailableAt(ctlPath, t); err != nil {
		return err
	}
	s.controllerAvailabilitySet <- struct{}{}
	return nil
}

func (s jemShimWithUpdateNotify) SetControllerAvailable(ctlPath params.EntityPath) error {
	if err := s.jemInterface.SetControllerAvailable(ctlPath); err != nil {
		return err
	}
	s.controllerAvailabilitySet <- struct{}{}
	return nil
}

func (s jemShimWithUpdateNotify) SetModelLife(ctlPath params.EntityPath, uuid string, life string) error {
	if err := s.jemInterface.SetModelLife(ctlPath, uuid, life); err != nil {
		return err
	}
	s.modelLifeSet <- struct{}{}
	return nil
}

func (s jemShimWithUpdateNotify) AcquireMonitorLease(ctlPath params.EntityPath, oldExpiry time.Time, oldOwner string, newExpiry time.Time, newOwner string) (time.Time, error) {
	t, err := s.jemInterface.AcquireMonitorLease(ctlPath, oldExpiry, oldOwner, newExpiry, newOwner)
	if err != nil {
		return time.Time{}, err
	}
	s.leaseAcquired <- struct{}{}
	return t, err
}

type jemShimWithAPIOpener struct {
	// openAPI is called when the OpenAPI method is called.
	openAPI func(path params.EntityPath) (jujuAPI, error)
	jemInterface
}

func (s jemShimWithAPIOpener) OpenAPI(path params.EntityPath) (jujuAPI, error) {
	return s.openAPI(path)
}

func (s jemShimWithAPIOpener) Clone() jemInterface {
	s.jemInterface = s.jemInterface.Clone()
	return s
}

type jemShimWithMonitorLeaseAcquirer struct {
	// acquireMonitorLease is called when the AcquireMonitorLease
	// method is called.
	acquireMonitorLease func(ctlPath params.EntityPath, oldExpiry time.Time, oldOwner string, newExpiry time.Time, newOwner string) (time.Time, error)
	jemInterface
}

func (s jemShimWithMonitorLeaseAcquirer) AcquireMonitorLease(ctlPath params.EntityPath, oldExpiry time.Time, oldOwner string, newExpiry time.Time, newOwner string) (time.Time, error) {
	return s.acquireMonitorLease(ctlPath, oldExpiry, oldOwner, newExpiry, newOwner)
}

func (s jemShimWithMonitorLeaseAcquirer) Clone() jemInterface {
	s.jemInterface = s.jemInterface.Clone()
	return s
}

type jemShimInMemory struct {
	mu          sync.Mutex
	refCount    int
	controllers map[params.EntityPath]*mongodoc.Controller
	models      map[params.EntityPath]*mongodoc.Model
}

var _ jemInterface = (*jemShimInMemory)(nil)

func newJEMShimInMemory() *jemShimInMemory {
	return &jemShimInMemory{
		controllers: make(map[params.EntityPath]*mongodoc.Controller),
		models:      make(map[params.EntityPath]*mongodoc.Model),
	}
}

func (s *jemShimInMemory) AddController(ctl *mongodoc.Controller) {
	if ctl.Path == (params.EntityPath{}) {
		panic("no path in controller")
	}
	ctl.Id = ctl.Path.String()
	s.mu.Lock()
	defer s.mu.Unlock()
	ctl1 := *ctl
	s.controllers[ctl.Path] = &ctl1
}

func (s *jemShimInMemory) AddModel(m *mongodoc.Model) {
	if m.Path.IsZero() {
		panic("no path in model")
	}
	if m.Controller.IsZero() {
		panic("no controller in model")
	}
	m.Id = m.Path.String()
	s.mu.Lock()
	defer s.mu.Unlock()
	m1 := *m
	s.models[m.Path] = &m1
}

func (s *jemShimInMemory) Clone() jemInterface {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refCount++
	return s
}

func (s *jemShimInMemory) SetControllerStats(ctlPath params.EntityPath, stats *mongodoc.ControllerStats) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctl, ok := s.controllers[ctlPath]
	if !ok {
		return errgo.WithCausef(nil, params.ErrNotFound, "")
	}
	ctl.Stats = *stats
	return nil
}

func (s *jemShimInMemory) SetControllerAvailable(ctlPath params.EntityPath) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctl, ok := s.controllers[ctlPath]
	if ok {
		ctl.UnavailableSince = time.Time{}
	}
	return nil
}

func (s *jemShimInMemory) SetControllerUnavailableAt(ctlPath params.EntityPath, t time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctl, ok := s.controllers[ctlPath]
	if ok && ctl.UnavailableSince.IsZero() {
		ctl.UnavailableSince = mongodoc.Time(t)
	}
	return nil
}

func (s *jemShimInMemory) SetModelLife(ctlPath params.EntityPath, uuid string, life string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.models {
		if m.Controller == ctlPath && m.UUID == uuid {
			m.Life = life
		}
	}
	return nil
}

func (s *jemShimInMemory) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refCount--
	if s.refCount < 0 {
		panic("close too many times")
	}
}

func (s *jemShimInMemory) AllControllers() ([]*mongodoc.Controller, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var r []*mongodoc.Controller
	for _, c := range s.controllers {
		c1 := *c
		r = append(r, &c1)
	}
	return r, nil
}

func (s *jemShimInMemory) OpenAPI(params.EntityPath) (jujuAPI, error) {
	return nil, errgo.New("jemShimInMemory doesn't implement OpenAPI")
}

func (s *jemShimInMemory) AcquireMonitorLease(ctlPath params.EntityPath, oldExpiry time.Time, oldOwner string, newExpiry time.Time, newOwner string) (time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctl, ok := s.controllers[ctlPath]
	if !ok {
		return time.Time{}, errgo.WithCausef(nil, params.ErrNotFound, "")
	}
	if ctl.MonitorLeaseOwner != oldOwner || !ctl.MonitorLeaseExpiry.UTC().Equal(oldExpiry.UTC()) {
		return time.Time{}, errgo.WithCausef(nil, jem.ErrLeaseUnavailable, "")
	}
	ctl.MonitorLeaseOwner = newOwner
	if newOwner == "" {
		ctl.MonitorLeaseExpiry = time.Time{}
	} else {
		ctl.MonitorLeaseExpiry = mongodoc.Time(newExpiry)
	}
	return ctl.MonitorLeaseExpiry, nil
}

var _ jujuAPI = (*jemAPIShim)(nil)

type jemAPIShim struct {
	mu           sync.Mutex
	watcherCount int
	closed       bool
	initial      []multiwatcher.Delta
}

// newJEMAPIShim returns an implementation of the jujuAPI interface
// that, when WatchAllModels is called, returns the given initial
// deltas and then nothing.
func newJEMAPIShim(initial []multiwatcher.Delta) *jemAPIShim {
	return &jemAPIShim{
		initial: initial,
	}
}

func (s *jemAPIShim) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

// WatcherCount returns the number of currently open API watchers.
func (s *jemAPIShim) WatcherCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.watcherCount
}

func (s *jemAPIShim) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *jemAPIShim) WatchAllModels() (allWatcher, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watcherCount++
	return &watcherShim{
		jemAPIShim: s,
		stopped:    make(chan struct{}),
		initial:    s.initial,
	}, nil
}

type watcherShim struct {
	jemAPIShim *jemAPIShim
	mu         sync.Mutex
	stopped    chan struct{}
	initial    []multiwatcher.Delta
}

func (s *watcherShim) Next() ([]multiwatcher.Delta, error) {
	s.mu.Lock()
	d := s.initial
	s.initial = nil
	s.mu.Unlock()
	if len(d) > 0 {
		return d, nil
	}
	<-s.stopped
	return nil, &jujuparams.Error{
		Message: "fake watcher was stopped",
		Code:    jujuparams.CodeStopped,
	}
}

func (s *watcherShim) Stop() error {
	close(s.stopped)
	s.jemAPIShim.mu.Lock()
	s.jemAPIShim.watcherCount--
	s.jemAPIShim.mu.Unlock()
	return nil
}
