package monitor

import (
	"context"
	"sync"
	"time"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	jujujujutesting "github.com/juju/juju/testing"
	jujutesting "github.com/juju/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	names "gopkg.in/juju/names.v3"

	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

func newJEMShimWithUpdateNotify(j jemInterface) jemShimWithUpdateNotify {
	return jemShimWithUpdateNotify{
		changed:      make(chan string, 100),
		jemInterface: j,
	}
}

type jemShimWithUpdateNotify struct {
	changed chan string
	jemInterface
}

// await waits for the given function to return the expected value,
// retrying after any of the shim notification channels have
// received a value.
func (s jemShimWithUpdateNotify) await(c *gc.C, f func() interface{}, want interface{}) {
	timeout := time.After(jujujujutesting.LongWait)
	for {
		got := f()
		ok, _ := jc.DeepEqual(got, want)
		if ok {
			break
		}
		select {
		case <-s.changed:
		case <-timeout:
			c.Assert(got, jc.DeepEquals, want, gc.Commentf("timed out waiting for value"))
		}
	}
	// We've got the expected value; now check that it remains stable
	// as long as events continue to arrive.
	for {
		event := s.waitAny(jujujujutesting.ShortWait)
		got := f()
		c.Assert(got, jc.DeepEquals, want, gc.Commentf("value changed after waiting (last event %q)", event))
		if event == "" {
			return
		}
	}
}

func (s jemShimWithUpdateNotify) Clone() jemInterface {
	s.jemInterface = s.jemInterface.Clone()
	return s
}

func (s jemShimWithUpdateNotify) assertNoEvent(c *gc.C) {
	if event := s.waitAny(jujutesting.ShortWait); event != "" {
		c.Fatalf("unexpected event received: %v", event)
	}
}

// waitAny waits for any update to happen, waiting for a maximum
// of the given time. If nothing happens, it returns the empty string,
// otherwise it returns the name of the event that happened.
func (s jemShimWithUpdateNotify) waitAny(maxWait time.Duration) string {
	select {
	case what := <-s.changed:
		return what
	case <-time.After(jujutesting.ShortWait):
		return ""
	}
}

func (s jemShimWithUpdateNotify) SetControllerStats(ctx context.Context, ctlPath params.EntityPath, stats *mongodoc.ControllerStats) error {
	if err := s.jemInterface.SetControllerStats(ctx, ctlPath, stats); err != nil {
		return err
	}
	s.changed <- "controller stats"
	return nil
}

func (s jemShimWithUpdateNotify) SetControllerUnavailableAt(ctx context.Context, ctlPath params.EntityPath, t time.Time) error {
	if err := s.jemInterface.SetControllerUnavailableAt(ctx, ctlPath, t); err != nil {
		return err
	}
	s.changed <- "controller availability"
	return nil
}

func (s jemShimWithUpdateNotify) SetControllerAvailable(ctx context.Context, ctlPath params.EntityPath) error {
	if err := s.jemInterface.SetControllerAvailable(ctx, ctlPath); err != nil {
		return err
	}
	s.changed <- "controller availability"
	return nil
}

func (s jemShimWithUpdateNotify) SetControllerVersion(ctx context.Context, ctlPath params.EntityPath, v version.Number) error {
	if err := s.jemInterface.SetControllerVersion(ctx, ctlPath, v); err != nil {
		return err
	}
	s.changed <- "controller version"
	return nil
}

func (s jemShimWithUpdateNotify) SetModelInfo(ctx context.Context, ctlPath params.EntityPath, uuid string, info *mongodoc.ModelInfo) error {
	if err := s.jemInterface.SetModelInfo(ctx, ctlPath, uuid, info); err != nil {
		return err
	}
	s.changed <- "model info"
	return nil
}

func (s jemShimWithUpdateNotify) DeleteModelWithUUID(ctx context.Context, ctlPath params.EntityPath, uuid string) error {
	if err := s.jemInterface.DeleteModelWithUUID(ctx, ctlPath, uuid); err != nil {
		return err
	}
	s.changed <- "delete model"
	return nil
}

func (s jemShimWithUpdateNotify) UpdateModelCounts(ctx context.Context, ctlPath params.EntityPath, uuid string, counts map[params.EntityCount]int, now time.Time) error {
	if err := s.jemInterface.UpdateModelCounts(ctx, ctlPath, uuid, counts, now); err != nil {
		return err
	}
	s.changed <- "model counts"
	return nil
}

func (s jemShimWithUpdateNotify) UpdateMachineInfo(ctx context.Context, ctlPath params.EntityPath, info *jujuparams.MachineInfo) error {
	if err := s.jemInterface.UpdateMachineInfo(ctx, ctlPath, info); err != nil {
		return err
	}
	s.changed <- "machine info"
	return nil
}

func (s jemShimWithUpdateNotify) AcquireMonitorLease(ctx context.Context, ctlPath params.EntityPath, oldExpiry time.Time, oldOwner string, newExpiry time.Time, newOwner string) (time.Time, error) {
	t, err := s.jemInterface.AcquireMonitorLease(ctx, ctlPath, oldExpiry, oldOwner, newExpiry, newOwner)
	if err != nil {
		return time.Time{}, err
	}
	s.changed <- "lease acquired"
	return t, err
}

type jemShimWithAPIOpener struct {
	// openAPI is called when the OpenAPI method is called.
	openAPI func(path params.EntityPath) (jujuAPI, error)
	jemInterface
}

func (s jemShimWithAPIOpener) OpenAPI(ctx context.Context, path params.EntityPath) (jujuAPI, error) {
	return s.openAPI(path)
}

func (s jemShimWithAPIOpener) Clone() jemInterface {
	s.jemInterface = s.jemInterface.Clone()
	return s
}

type jemShimWithMonitorLeaseAcquirer struct {
	// acquireMonitorLease is called when the AcquireMonitorLease
	// method is called.
	acquireMonitorLease func(ctxt context.Context, ctlPath params.EntityPath, oldExpiry time.Time, oldOwner string, newExpiry time.Time, newOwner string) (time.Time, error)
	jemInterface
}

func (s jemShimWithMonitorLeaseAcquirer) AcquireMonitorLease(ctx context.Context, ctlPath params.EntityPath, oldExpiry time.Time, oldOwner string, newExpiry time.Time, newOwner string) (time.Time, error) {
	return s.acquireMonitorLease(ctx, ctlPath, oldExpiry, oldOwner, newExpiry, newOwner)
}

func (s jemShimWithMonitorLeaseAcquirer) Clone() jemInterface {
	s.jemInterface = s.jemInterface.Clone()
	return s
}

type machineId struct {
	modelUUID string
	id        string
}

type applicationId struct {
	modelUUID string
	name      string
}

type jemShimInMemory struct {
	mu                          sync.Mutex
	refCount                    int
	controllers                 map[params.EntityPath]*mongodoc.Controller
	models                      map[params.EntityPath]*mongodoc.Model
	machines                    map[string]map[machineId]*mongodoc.Machine
	applications                map[string]map[applicationId]*mongodoc.Application
	controllerUpdateCredentials map[params.EntityPath]bool
	cloudRegions                map[string]*mongodoc.CloudRegion
}

var _ jemInterface = (*jemShimInMemory)(nil)

func newJEMShimInMemory() *jemShimInMemory {
	return &jemShimInMemory{
		controllers:                 make(map[params.EntityPath]*mongodoc.Controller),
		models:                      make(map[params.EntityPath]*mongodoc.Model),
		controllerUpdateCredentials: make(map[params.EntityPath]bool),
		machines:                    make(map[string]map[machineId]*mongodoc.Machine),
		applications:                make(map[string]map[applicationId]*mongodoc.Application),
		cloudRegions:                make(map[string]*mongodoc.CloudRegion),
	}
}

func (s *jemShimInMemory) Controller(ctx context.Context, p params.EntityPath) (*mongodoc.Controller, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := *s.controllers[p]
	return &c, nil
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

func (s *jemShimInMemory) SetControllerStats(ctx context.Context, ctlPath params.EntityPath, stats *mongodoc.ControllerStats) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctl, ok := s.controllers[ctlPath]
	if !ok {
		return errgo.WithCausef(nil, params.ErrNotFound, "")
	}
	ctl.Stats = *stats
	return nil
}

func (s *jemShimInMemory) SetControllerAvailable(ctx context.Context, ctlPath params.EntityPath) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctl, ok := s.controllers[ctlPath]
	if ok {
		ctl.UnavailableSince = time.Time{}
	}
	return nil
}

func (s *jemShimInMemory) SetControllerUnavailableAt(ctx context.Context, ctlPath params.EntityPath, t time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctl, ok := s.controllers[ctlPath]
	if ok && ctl.UnavailableSince.IsZero() {
		ctl.UnavailableSince = mongodoc.Time(t)
	}
	return nil
}

func (s *jemShimInMemory) SetControllerVersion(ctx context.Context, ctlPath params.EntityPath, v version.Number) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctl, ok := s.controllers[ctlPath]
	if ok {
		ctl.Version = &v
	}
	return nil
}

func (s *jemShimInMemory) SetModelInfo(ctx context.Context, ctlPath params.EntityPath, uuid string, info *mongodoc.ModelInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.models {
		if m.Controller == ctlPath && m.UUID == uuid {
			m.Info = info
		}
	}
	return nil
}

func (s *jemShimInMemory) DeleteModelWithUUID(ctx context.Context, ctlPath params.EntityPath, uuid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, m := range s.models {
		if m.Controller == ctlPath && m.UUID == uuid {
			delete(s.models, k)
		}
	}
	return nil
}

func (s *jemShimInMemory) UpdateModelCounts(ctx context.Context, ctlPath params.EntityPath, uuid string, counts map[params.EntityCount]int, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var model *mongodoc.Model
	for _, m := range s.models {
		if m.UUID == uuid && m.Controller == ctlPath {
			model = m
			break
		}
	}
	if model == nil {
		return params.ErrNotFound
	}
	if model.Counts == nil {
		model.Counts = make(map[params.EntityCount]params.Count)
	}
	for name, n := range counts {
		count := model.Counts[name]
		jem.UpdateCount(&count, n, now)
		model.Counts[name] = count
	}
	return nil
}

func (s *jemShimInMemory) RemoveControllerMachines(ctx context.Context, ctlPath params.EntityPath) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.machines[ctlPath.String()] = make(map[machineId]*mongodoc.Machine)
	return nil
}

func (s *jemShimInMemory) RemoveControllerApplications(ctx context.Context, ctlPath params.EntityPath) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.applications[ctlPath.String()] = make(map[applicationId]*mongodoc.Application)
	return nil
}

func (s *jemShimInMemory) UpdateApplicationInfo(ctx context.Context, ctlPath params.EntityPath, info *jujuparams.ApplicationInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	info1 := *info
	s.applications[ctlPath.String()][applicationId{info1.ModelUUID, info1.Name}] = &mongodoc.Application{
		Id: info1.ModelUUID + " " + info1.Name,
		Info: &mongodoc.ApplicationInfo{
			ModelUUID:       info1.ModelUUID,
			Name:            info1.Name,
			Exposed:         info1.Exposed,
			CharmURL:        info1.CharmURL,
			OwnerTag:        info1.OwnerTag,
			Life:            info1.Life,
			Subordinate:     info1.Subordinate,
			Status:          info1.Status,
			WorkloadVersion: info1.WorkloadVersion,
		},
	}
	return nil
}

func (s *jemShimInMemory) UpdateMachineInfo(ctx context.Context, ctlPath params.EntityPath, info *jujuparams.MachineInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	info1 := *info
	s.machines[ctlPath.String()][machineId{info1.ModelUUID, info1.Id}] = &mongodoc.Machine{
		Id:   info1.ModelUUID + " " + info1.Id,
		Info: &info1,
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

func (s *jemShimInMemory) AllControllers(ctx context.Context) ([]*mongodoc.Controller, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var r []*mongodoc.Controller
	for _, c := range s.controllers {
		c1 := *c
		r = append(r, &c1)
	}
	return r, nil
}

func (s *jemShimInMemory) ModelUUIDsForController(ctx context.Context, ctlPath params.EntityPath) (uuids []string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.models {
		uuids = append(uuids, m.UUID)
	}
	return uuids, nil
}

func (s *jemShimInMemory) ControllerUpdateCredentials(_ context.Context, ctlPath params.EntityPath) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.controllerUpdateCredentials[ctlPath] = true
	return nil
}

func (s *jemShimInMemory) OpenAPI(context.Context, params.EntityPath) (jujuAPI, error) {
	return nil, errgo.New("jemShimInMemory doesn't implement OpenAPI")
}

func (s *jemShimInMemory) AcquireMonitorLease(ctx context.Context, ctlPath params.EntityPath, oldExpiry time.Time, oldOwner string, newExpiry time.Time, newOwner string) (time.Time, error) {
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

func (s *jemShimInMemory) UpdateCloudRegions(ctx context.Context, cloudRegions []mongodoc.CloudRegion) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, cloudRegion := range cloudRegions {
		id := cloudRegion.GetId()
		if cr, ok := s.cloudRegions[id]; ok {
			cr.PrimaryControllers = append(cr.PrimaryControllers, cloudRegion.PrimaryControllers...)
			cr.SecondaryControllers = append(cr.SecondaryControllers, cloudRegion.SecondaryControllers...)
			continue
		}
		s.cloudRegions[id] = &cloudRegions[k]
	}

	return nil
}

type jujuAPIShims struct {
	mu           sync.Mutex
	openCount    int
	watcherCount int
}

func newJujuAPIShims() *jujuAPIShims {
	return &jujuAPIShims{}
}

// newJujuAPIShim returns an implementation of the jujuAPI interface
// that, when WatchAllModels is called, returns the given initial
// deltas and then nothing.
func (s *jujuAPIShims) newJujuAPIShim(initial []jujuparams.Delta) *jujuAPIShim {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.openCount++
	return &jujuAPIShim{
		initial: initial,
		shims:   s,
	}
}

var closedAttempt = &utils.AttemptStrategy{
	Total: time.Second,
	Delay: time.Millisecond,
}

// CheckAllClosed checks that all API connections and watchers
// have been closed.
func (s *jujuAPIShims) CheckAllClosed(c *gc.C) {
	// The API connections can be closed asynchronously after
	// the worker is closed down, so wait for a while to make sure
	// they are actually closed.
	for a := closedAttempt.Start(); a.Next(); {
		s.mu.Lock()
		n := s.openCount
		s.mu.Unlock()
		if n == 0 {
			break
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	c.Check(s.openCount, gc.Equals, 0)
	c.Check(s.watcherCount, gc.Equals, 0)
}

var _ jujuAPI = (*jujuAPIShim)(nil)

// jujuAPIShim implements the jujuAPI interface.
type jujuAPIShim struct {
	shims         *jujuAPIShims
	closed        bool
	initial       []jujuparams.Delta
	stack         string
	serverVersion version.Number
	clouds        map[names.CloudTag]cloud.Cloud
}

func (s *jujuAPIShim) Evict() {
	s.Close()
}

func (s *jujuAPIShim) Close() error {
	s.shims.mu.Lock()
	defer s.shims.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	s.shims.openCount--
	return nil
}

func (s *jujuAPIShim) WatchAllModels() (allWatcher, error) {
	s.shims.mu.Lock()
	defer s.shims.mu.Unlock()
	s.shims.watcherCount++
	return &watcherShim{
		jujuAPIShim: s,
		stopped:     make(chan struct{}),
		initial:     s.initial,
	}, nil
}

func (s *jujuAPIShim) ModelExists(uuid string) (bool, error) {
	panic("unexpected call to ModelExists")
}

func (s *jujuAPIShim) ServerVersion() (version.Number, bool) {
	return s.serverVersion, true
}

func (s *jujuAPIShim) Clouds() (map[names.CloudTag]cloud.Cloud, error) {
	return s.clouds, nil
}

type watcherShim struct {
	jujuAPIShim *jujuAPIShim
	mu          sync.Mutex
	stopped     chan struct{}
	initial     []jujuparams.Delta
}

func (s *watcherShim) Next() ([]jujuparams.Delta, error) {
	s.jujuAPIShim.shims.mu.Lock()
	d := s.initial
	s.initial = nil
	s.jujuAPIShim.shims.mu.Unlock()
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
	s.jujuAPIShim.shims.mu.Lock()
	s.jujuAPIShim.shims.watcherCount--
	s.jujuAPIShim.shims.mu.Unlock()
	return nil
}
