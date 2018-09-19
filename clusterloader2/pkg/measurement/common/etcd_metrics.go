/*
Copyright 2018 The Kubernetes Authors.

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

package common

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/common/model"
	"k8s.io/perf-tests/clusterloader2/pkg/measurement"
	measurementutil "k8s.io/perf-tests/clusterloader2/pkg/measurement/util"
	"k8s.io/perf-tests/clusterloader2/pkg/util"
)

const (
	etcdMetricsMetricName = "EtcdMetrics"
)

func init() {
	measurement.Register(etcdMetricsMetricName, createEtcdMetricsMeasurement)
}

func createEtcdMetricsMeasurement() measurement.Measurement {
	return &etcdMetricsMeasurement{
		stopCh:  make(chan struct{}),
		wg:      &sync.WaitGroup{},
		metrics: newEtcdMetrics(),
	}
}

type etcdMetricsMeasurement struct {
	stopCh  chan struct{}
	wg      *sync.WaitGroup
	metrics *etcdMetrics
}

// Execute supports two actions:
// - start - Starts collecting etcd metrics.
// - gather - Gathers and prints etcd metrics summary.
func (e *etcdMetricsMeasurement) Execute(config *measurement.MeasurementConfig) ([]measurement.Summary, error) {
	var summaries []measurement.Summary
	action, err := util.GetString(config.Params, "action")
	if err != nil {
		return summaries, err
	}
	provider, err := util.GetStringOrDefault(config.Params, "provider", measurement.ClusterConfig.Provider)
	if err != nil {
		return summaries, err
	}
	host, err := util.GetStringOrDefault(config.Params, "host", measurement.ClusterConfig.MasterIP)
	if err != nil {
		return summaries, err
	}

	switch action {
	case "start":
		glog.Infof("Starting etcd metrics collecting...")
		waitTime, err := util.GetDurationOrDefault(config.Params, "waitTime", time.Minute)
		if err != nil {
			return summaries, err
		}
		e.startCollecting(provider, host, waitTime)
		return summaries, nil
	case "gather":
		if err = e.stopAndSummarize(provider, host); err != nil {
			return summaries, err
		}
		summaries := append(summaries, e.metrics)
		return summaries, nil

	default:
		return summaries, fmt.Errorf("unknown action %v", action)
	}
}

func getEtcdMetrics(provider, host string) ([]*model.Sample, error) {
	// Etcd is only exposed on localhost level. We are using ssh method
	if provider == "gke" {
		glog.Infof("Not grabbing scheduler metrics through master SSH: unsupported for gke")
		return nil, nil
	}

	cmd := "curl http://localhost:2379/metrics"
	sshResult, err := measurementutil.SSH(cmd, host+":22", provider)
	if err != nil || sshResult.Code != 0 {
		return nil, fmt.Errorf("unexpected error (code: %d) in ssh connection to master: %#v", sshResult.Code, err)
	}
	data := sshResult.Stdout

	return measurementutil.ExtractMetricSamples(data)
}

func getEtcdDatabaseSize(provider, host string) (float64, error) {
	samples, err := getEtcdMetrics(provider, host)
	if err != nil {
		return 0, err
	}
	for _, sample := range samples {
		if sample.Metric[model.MetricNameLabel] == "etcd_debugging_mvcc_db_total_size_in_bytes" {
			return float64(sample.Value), nil
		}
	}
	return 0, fmt.Errorf("Couldn't find etcd database size metric")
}

func (e *etcdMetricsMeasurement) startCollecting(provider, host string, interval time.Duration) {
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		for {
			select {
			case <-time.After(interval):
				dbSize, err := getEtcdDatabaseSize(provider, host)
				if err != nil {
					glog.Infof("Failed to collect etcd database size")
					continue
				}
				e.metrics.MaxDatabaseSize = math.Max(e.metrics.MaxDatabaseSize, dbSize)
			case <-e.stopCh:
				return
			}
		}
	}()
}

func (e *etcdMetricsMeasurement) stopAndSummarize(provider, host string) error {
	close(e.stopCh)
	e.wg.Wait()

	// Do some one-off collection of metrics.
	samples, err := getEtcdMetrics(provider, host)
	if err != nil {
		return err
	}
	for _, sample := range samples {
		switch sample.Metric[model.MetricNameLabel] {
		case "etcd_disk_backend_commit_duration_seconds_bucket":
			measurementutil.ConvertSampleToBucket(sample, &e.metrics.BackendCommitDuration)
		case "etcd_debugging_snap_save_total_duration_seconds_bucket":
			measurementutil.ConvertSampleToBucket(sample, &e.metrics.SnapshotSaveTotalDuration)
		case "etcd_disk_wal_fsync_duration_seconds_bucket":
			measurementutil.ConvertSampleToBucket(sample, &e.metrics.WalFsyncDuration)
		case "etcd_network_peer_round_trip_time_seconds_bucket":
			measurementutil.ConvertSampleToBucket(sample, &e.metrics.PeerRoundTripTime)
		}
	}
	return nil
}

type etcdMetrics struct {
	BackendCommitDuration     measurementutil.HistogramVec `json:"backendCommitDuration"`
	SnapshotSaveTotalDuration measurementutil.HistogramVec `json:"snapshotSaveTotalDuration"`
	PeerRoundTripTime         measurementutil.HistogramVec `json:"peerRoundTripTime"`
	WalFsyncDuration          measurementutil.HistogramVec `json:"walFsyncDuration"`
	MaxDatabaseSize           float64                      `json:"maxDatabaseSize"`
}

func newEtcdMetrics() *etcdMetrics {
	return &etcdMetrics{
		BackendCommitDuration:     make(measurementutil.HistogramVec, 0),
		SnapshotSaveTotalDuration: make(measurementutil.HistogramVec, 0),
		PeerRoundTripTime:         make(measurementutil.HistogramVec, 0),
		WalFsyncDuration:          make(measurementutil.HistogramVec, 0),
	}
}

// SummaryName returns name of the summary.
func (e *etcdMetrics) SummaryName() string {
	return etcdMetricsMetricName
}

// PrintSummary returns summary as a string.
func (e *etcdMetrics) PrintSummary() (string, error) {
	return util.PrettyPrintJSON(e)
}
