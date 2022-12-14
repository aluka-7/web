package web

import (
	"fmt"
	"github.com/aluka-7/metric"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
)

const (
	serverNamespace = "http_server"
)

var (
	_metricServerReqDur = metric.NewHistogramVec(&metric.HistogramVecOpts{
		Namespace: serverNamespace,
		Subsystem: "requests",
		Name:      "duration_ms",
		Help:      "http server requests duration(ms).",
		Labels:    []string{"path", "caller", "method"},
		Buckets:   []float64{5, 10, 25, 50, 100, 250, 500, 1000},
	})
	_metricServerReqCodeTotal = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: serverNamespace,
		Subsystem: "requests",
		Name:      "code_total",
		Help:      "http server requests error count.",
		Labels:    []string{"path", "caller", "method", "code"},
	})
)

// 基于prometheus实现指标收集功能
func metrics() {
	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		h := promhttp.Handler()
		h.ServeHTTP(w, r)
	})
	fmt.Println("WEB即将开启metrics服务,访问地址 http://ip:7070")
	if err := http.ListenAndServe(":7070", nil); err != nil {
		fmt.Printf("RPC开启metrics服务错误:%+v\n", err)
	}
}
