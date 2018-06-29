package jem

import (
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/net/context"
	errgo "gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/internal/servermon"
	"github.com/CanonicalLtd/jem/internal/zapctx"
	"github.com/CanonicalLtd/jem/internal/zaputil"
	"github.com/CanonicalLtd/jem/params"
)

// ModelStats implements a Prometheus collector that provides information
// about JIMM statistics.
type ModelStats struct {
	pool    *Pool
	context context.Context
	descs   []*prometheus.Desc
}

// statItem represents a single metric result from the Stats collector.
type statItem int

// These constants each represent a single integer metric result. See
// statsDescs for a description of what each one means.
const (
	statApplicationsRunning statItem = iota
	statControllersRunning
	statMachinesRunning
	statModelsRunning
	statUnitsRunning
	statActiveModelsRunning
	numStats
)

// currentModelStats holds a snapshot of the statistics gathers for a
// single Stats.Collect call.
type currentModelStats struct {
	// values holds a value for each statItem constant,
	// indexed by that constant.
	values [numStats]int
}

// getModelStat returns a function that returns a metric for the given
// stat item from a snapshot of the current statistics.
func getModelStat(c statItem) func(*currentModelStats) float64 {
	return func(s *currentModelStats) float64 {
		return float64(s.values[c])
	}
}

// modelStatsDescs holds information about all the stats suitable for
// making *prometheus.Desc and prometheus.Metric instances from.
var modelStatsDescs = []struct {
	// name holds the name of the metric as seen by prometheus.
	name string
	// help holds the help text for the metric.
	help string
	// get returns the actual metric value from a snapshot
	// of the current stats.
	get func(*currentModelStats) float64
}{{
	name: "active_models_running",
	help: "The current number of running models with at least one machine.",
	get:  getModelStat(statActiveModelsRunning),
}, {
	name: "applications_running",
	help: "The current number of running applications.",
	get:  getModelStat(statApplicationsRunning),
}, {
	name: "controllers_running",
	help: "The current number of running controllers.",
	get:  getModelStat(statControllersRunning),
}, {
	name: "machines_running",
	help: "The current number of running machines.",
	get:  getModelStat(statMachinesRunning),
}, {
	name: "models_running",
	help: "The current number of running models.",
	get:  getModelStat(statModelsRunning),
}, {
	name: "units_running",
	help: "The current number of running units.",
	get:  getModelStat(statUnitsRunning),
}}

// ModelStats returns an implementation of prometheus.Collector that
// returns information on statistics obtained from the pool.
func (p *Pool) ModelStats(ctx context.Context) *ModelStats {
	s := &ModelStats{
		context: ctx,
		pool:    p,
	}
	for _, d := range modelStatsDescs {
		s.descs = append(s.descs, prometheus.NewDesc(
			prometheus.BuildFQName("jem", "health", d.name),
			d.help,
			nil,
			nil,
		))
	}
	return s
}

// Describe implements prometheus.Collector.Describe by describing all
// the statistics that can be obtained from JIMM.
func (s *ModelStats) Describe(c chan<- *prometheus.Desc) {
	for _, d := range s.descs {
		c <- d
	}
}

// Collect implements prometheus.Collector.Collect by collecting all the
// model statistics from JIMM.
func (s *ModelStats) Collect(c chan<- prometheus.Metric) {
	jem := s.pool.JEM(s.context)
	defer jem.Close()
	current, err := s.collectStats(jem)
	if err != nil {
		zapctx.Error(s.context, "cannot collect statistics", zaputil.Error(err))
		servermon.StatsCollectFailCount.Inc()
		return
	}
	for i, d := range modelStatsDescs {
		c <- prometheus.MustNewConstMetric(s.descs[i], prometheus.GaugeValue, d.get(current))
	}
}

// collectStats returns a snapshot of the current statistics from JIMM.
func (s *ModelStats) collectStats(jem *JEM) (*currentModelStats, error) {
	var cs currentModelStats
	iter := jem.DB.Models().Find(nil).Iter()
	var m mongodoc.Model
	for iter.Next(&m) {
		cs.values[statModelsRunning]++
		machineCount := m.Counts[params.MachineCount].Current
		if machineCount > 0 {
			cs.values[statActiveModelsRunning]++
			cs.values[statMachinesRunning] += machineCount
		}
		cs.values[statApplicationsRunning] += m.Counts[params.ApplicationCount].Current
		cs.values[statUnitsRunning] += m.Counts[params.UnitCount].Current
	}
	if err := iter.Err(); err != nil {
		return nil, errgo.Notef(err, "cannot gather stats")
	}
	var err error
	cs.values[statControllersRunning], err = jem.DB.Controllers().Count()
	if err != nil {
		return nil, errgo.Notef(err, "cannot gather stats")
	}
	return &cs, nil
}
