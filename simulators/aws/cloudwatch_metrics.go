package main

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/fxamacker/cbor/v2"
	sim "github.com/sockerless/simulator"
)

// CloudWatch Metrics types

type CWMetricDatum struct {
	Namespace  string          `json:"namespace" cbor:"Namespace"`
	MetricName string          `json:"metricName" cbor:"MetricName"`
	Dimensions []CWDimension   `json:"dimensions,omitempty" cbor:"Dimensions,omitempty"`
	Value      float64         `json:"value" cbor:"Value"`
	Timestamp  float64         `json:"timestamp" cbor:"Timestamp"`
	Unit       string          `json:"unit,omitempty" cbor:"Unit,omitempty"`
}

type CWDimension struct {
	Name  string `json:"Name" cbor:"Name"`
	Value string `json:"Value" cbor:"Value"`
}

// State store for metrics
var cwMetrics *sim.StateStore[[]CWMetricDatum]

func registerCloudWatchMetrics(srv *sim.Server) {
	cwMetrics = sim.NewStateStore[[]CWMetricDatum]()

	// Smithy RPCv2 CBOR uses URL path routing
	srv.HandleFunc("POST /service/GraniteServiceVersion20100801/operation/GetMetricData", handleCWGetMetricData)
	srv.HandleFunc("POST /service/GraniteServiceVersion20100801/operation/PutMetricData", handleCWPutMetricData)
}

// GetMetricData request/response types (CBOR)
type getMetricDataRequest struct {
	StartTime         float64             `cbor:"StartTime"`
	EndTime           float64             `cbor:"EndTime"`
	MetricDataQueries []metricDataQuery   `cbor:"MetricDataQueries"`
}

type metricDataQuery struct {
	Id         string      `cbor:"Id"`
	MetricStat *metricStat `cbor:"MetricStat,omitempty"`
}

type metricStat struct {
	Metric *metricRef `cbor:"Metric"`
	Period int32      `cbor:"Period"`
	Stat   string     `cbor:"Stat"`
}

type metricRef struct {
	Namespace  string        `cbor:"Namespace"`
	MetricName string        `cbor:"MetricName"`
	Dimensions []CWDimension `cbor:"Dimensions,omitempty"`
}

type getMetricDataResponse struct {
	MetricDataResults []metricDataResult `cbor:"MetricDataResults"`
}

type metricDataResult struct {
	Id         string    `cbor:"Id"`
	StatusCode string    `cbor:"StatusCode"`
	Values     []float64 `cbor:"Values"`
	Timestamps []float64 `cbor:"Timestamps"`
}

func handleCWGetMetricData(w http.ResponseWriter, r *http.Request) {
	var req getMetricDataRequest
	if err := cbor.NewDecoder(r.Body).Decode(&req); err != nil {
		sim.AWSError(w, "InvalidParameterValue", "Invalid CBOR request", http.StatusBadRequest)
		return
	}

	now := time.Now().UTC()
	var results []metricDataResult

	for _, q := range req.MetricDataQueries {
		result := metricDataResult{
			Id:         q.Id,
			StatusCode: "Complete",
		}

		if q.MetricStat != nil && q.MetricStat.Metric != nil {
			m := q.MetricStat.Metric

			// For ECS/ContainerInsights, compute metrics from running task state
			if m.Namespace == "ECS/ContainerInsights" {
				var taskID, clusterName string
				for _, d := range m.Dimensions {
					switch d.Name {
					case "TaskId":
						taskID = d.Value
					case "ClusterName":
						clusterName = d.Value
					}
				}

				if taskID != "" {
					value := computeECSMetric(m.MetricName, clusterName, taskID)
					if value >= 0 {
						result.Values = []float64{value}
						result.Timestamps = []float64{float64(now.Unix())}
					}
				}
			} else {
				// Look up stored metrics
				key := metricsKey(m.Namespace, m.MetricName, m.Dimensions)
				if data, ok := cwMetrics.Get(key); ok {
					startSec := req.StartTime
					endSec := req.EndTime
					for _, d := range data {
						if d.Timestamp >= startSec && d.Timestamp <= endSec {
							result.Values = append(result.Values, d.Value)
							result.Timestamps = append(result.Timestamps, d.Timestamp)
						}
					}
				}
			}
		}

		results = append(results, result)
	}

	resp := getMetricDataResponse{MetricDataResults: results}
	data, err := cbor.Marshal(resp)
	if err != nil {
		sim.AWSError(w, "InternalFailure", "Failed to encode response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/cbor")
	w.Header().Set("Smithy-Protocol", "rpc-v2-cbor")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// computeECSMetric returns a realistic metric value for a running ECS task.
func computeECSMetric(metricName, clusterName, taskID string) float64 {
	// Look up the task to see if it's running
	task, ok := ecsTasks.Get(taskID)
	if !ok || task.LastStatus != "RUNNING" {
		return -1
	}

	// Compute realistic values based on task definition
	cpuUnits := 256.0
	memMB := 512.0
	if task.Cpu != "" {
		if v, err := strconv.ParseFloat(task.Cpu, 64); err == nil {
			cpuUnits = v
		}
	}
	if task.Memory != "" {
		if v, err := strconv.ParseFloat(task.Memory, 64); err == nil {
			memMB = v
		}
	}

	switch metricName {
	case "CpuUtilized":
		// Return a fraction of allocated CPU (10-30% typical for idle containers)
		return math.Round(cpuUnits*0.15*100) / 100
	case "MemoryUtilized":
		// Return a fraction of allocated memory (20-40% typical)
		return math.Round(memMB*0.25*100) / 100
	case "RunningTaskCount":
		return 1
	default:
		return 0
	}
}

// PutMetricData request type
type putMetricDataRequest struct {
	Namespace  string          `cbor:"Namespace"`
	MetricData []putMetricItem `cbor:"MetricData"`
}

type putMetricItem struct {
	MetricName string        `cbor:"MetricName"`
	Dimensions []CWDimension `cbor:"Dimensions,omitempty"`
	Value      float64       `cbor:"Value"`
	Timestamp  float64       `cbor:"Timestamp"`
	Unit       string        `cbor:"Unit,omitempty"`
}

func handleCWPutMetricData(w http.ResponseWriter, r *http.Request) {
	var req putMetricDataRequest
	if err := cbor.NewDecoder(r.Body).Decode(&req); err != nil {
		sim.AWSError(w, "InvalidParameterValue", "Invalid CBOR request", http.StatusBadRequest)
		return
	}

	for _, item := range req.MetricData {
		key := metricsKey(req.Namespace, item.MetricName, item.Dimensions)
		datum := CWMetricDatum{
			Namespace:  req.Namespace,
			MetricName: item.MetricName,
			Dimensions: item.Dimensions,
			Value:      item.Value,
			Timestamp:  item.Timestamp,
			Unit:       item.Unit,
		}
		cwMetrics.Update(key, func(existing *[]CWMetricDatum) {
			*existing = append(*existing, datum)
		})
		if _, ok := cwMetrics.Get(key); !ok {
			cwMetrics.Put(key, []CWMetricDatum{datum})
		}
	}

	// Empty CBOR response
	data, _ := cbor.Marshal(map[string]any{})
	w.Header().Set("Content-Type", "application/cbor")
	w.Header().Set("Smithy-Protocol", "rpc-v2-cbor")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func metricsKey(namespace, metricName string, dims []CWDimension) string {
	key := namespace + "/" + metricName
	for _, d := range dims {
		key += fmt.Sprintf("/%s=%s", d.Name, d.Value)
	}
	return key
}
