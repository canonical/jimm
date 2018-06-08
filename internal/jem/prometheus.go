package jem

import (
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"golang.org/x/net/context"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/internal/servermon"
	"github.com/CanonicalLtd/jem/internal/zapctx"
	"github.com/CanonicalLtd/jem/internal/zaputil"
	"github.com/CanonicalLtd/jem/params"
)

// Stats implements a Prometheus collector that provides information
// about JIMM statistics.
type Stats struct {
	pool    *Pool
	context context.Context
	descs   []*prometheus.Desc
}

// statItem represents a single metric result from the Stats
// collector.
type statItem int

// These constants each represent a single integer metric
// result. See statsDescs for a description of what each one
// means.
const (
	statApplicationsRunning statItem = iota
	statControllersRunning
	statMachinesRunning
	statModelsRunning
	statUnitsRunning
	statActiveModelsRunning
	numStats
)

// currentStats holds a snapshot of the statistics gathers
// for a single Stats.Collect call.
type currentStats struct {
	// values holds a value for each statItem constant,
	// indexed by that constant.
	values [numStats]int
}

// getStatsGauge returns a function that returns a metric
// for the given stat item from a snapshot of the current
// statistics.
func getStatsGauge(c statItem) func(*currentStats) metric {
	return func(s *currentStats) metric {
		return metric{
			kind:  dto.MetricType_GAUGE,
			value: s.values[c],
		}
	}
}

// statsDescs holds information about all the stats
// suitable for making *prometheus.Desc and prometheus.Metric
// instances from.
var statsDescs = []struct {
	// name holds the name of the metric as seen by prometheus.
	name string
	// help holds the help text for the metric.
	help string
	// get returns the actual metric value from a snapshot
	// of the current stats.
	get func(*currentStats) metric
}{{
	name: "active_models_running",
	help: "The current number of running models with at least one machine.",
	get:  getStatsGauge(statActiveModelsRunning),
}, {
	name: "applications_running",
	help: "The current number of running applications.",
	get:  getStatsGauge(statApplicationsRunning),
}, {
	name: "controllers_running",
	help: "The current number of running controllers.",
	get:  getStatsGauge(statControllersRunning),
}, {
	name: "machines_running",
	help: "The current number of running machines.",
	get:  getStatsGauge(statMachinesRunning),
}, {
	name: "models_running",
	help: "The current number of running models.",
	get:  getStatsGauge(statModelsRunning),
}, {
	name: "units_running",
	help: "The current number of running units.",
	get:  getStatsGauge(statUnitsRunning),
}}

// Stats returns an implementation of prometheus.Collector
// that returns information on statistics obtained from the pool.
func (p *Pool) Stats(ctx context.Context) *Stats {
	s := &Stats{
		context: ctx,
		pool:    p,
	}
	for _, d := range statsDescs {
		s.descs = append(s.descs, prometheus.NewDesc(
			prometheus.BuildFQName("jem", "health", d.name),
			d.help,
			nil,
			nil,
		))
	}
	return s
}

// Collect implements prometheus.Collector.Describe by describing all
// the statistics that can be obtained from JIMM.
func (s *Stats) Describe(c chan<- *prometheus.Desc) {
	for _, d := range s.descs {
		c <- d
	}
}

// Collect implements prometheus.Collector.Collect by collecting
// all the statistic from JIMM.
func (s *Stats) Collect(c chan<- prometheus.Metric) {
	jem := s.pool.JEM(s.context)
	defer jem.Close()
	current, err := s.collectStats(jem)
	if err != nil {
		zapctx.Error(s.context, "cannot collect statistics", zaputil.Error(err))
		servermon.StatsCollectFailCount.Inc()
		return
	}
	for i, d := range statsDescs {
		c <- &metricWithDesc{
			metric: d.get(current),
			desc:   s.descs[i],
		}
	}
}

// collectStats returns a snapshot of the current statistics from JIMM.
func (s *Stats) collectStats(jem *JEM) (*currentStats, error) {
	var cs currentStats
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

// metricWithDesc implements prometheus.Metric
// by combining a metric value with a description.
type metricWithDesc struct {
	metric
	desc *prometheus.Desc
}

// Desc implements prometheus.Metric.Desc.
func (m *metricWithDesc) Desc() *prometheus.Desc {
	return m.desc
}

// metric implements half of the prometheus.Metric interface.
type metric struct {
	kind  dto.MetricType
	value int
}

// Write implements prometheus.Metric.Write.
func (m metric) Write(wm *dto.Metric) error {
	switch m.kind {
	case dto.MetricType_COUNTER:
		wm.Counter = &dto.Counter{
			Value: newFloat64(float64(m.value)),
		}
	case dto.MetricType_GAUGE:
		wm.Gauge = &dto.Gauge{
			Value: newFloat64(float64(m.value)),
		}
	default:
		panic("unexpected metric type")
	}
	return nil
}

func newFloat64(f float64) *float64 {
	return &f
}
