package http

import (
	"fmt"
	"net/http"
	"os"

	"github.com/github/freno/go/group"
	"github.com/github/freno/go/throttle"

	"github.com/julienschmidt/httprouter"
)

// API exposes the contract for the throttler's web API
type API interface {
	LbCheck(w http.ResponseWriter, _ *http.Request, _ httprouter.Params)
	LeaderCheck(w http.ResponseWriter, _ *http.Request, _ httprouter.Params)
	RaftLeader(w http.ResponseWriter, _ *http.Request, _ httprouter.Params)
	Hostname(w http.ResponseWriter, _ *http.Request, _ httprouter.Params)
	CheckMySQLCluster(w http.ResponseWriter, r *http.Request, _ httprouter.Params)
	AggregatedMetrics(w http.ResponseWriter, r *http.Request, _ httprouter.Params)
}

// APIImpl implements the API
type APIImpl struct {
	throttler *throttle.Throttler
}

// NewAPIImpl creates a new instance of the API implementation
func NewAPIImpl(throttler *throttle.Throttler) *APIImpl {
	return &APIImpl{
		throttler: throttler,
	}
}

// LbCheck responds to LbCheck with HTTP 200
func (api *APIImpl) LbCheck(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	w.WriteHeader(http.StatusOK)
}

// LeaderCheck responds with HTTP 200 when this node is a raft leader, otherwise 404
// This is a useful check for HAProxy routing
func (api *APIImpl) LeaderCheck(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	statusCode := http.StatusNotFound
	if group.IsLeader() {
		statusCode = http.StatusOK
	}
	w.WriteHeader(statusCode)
	if r.Method == http.MethodGet {
		fmt.Fprintf(w, "HTTP %d", statusCode)
	}
}

// RaftLeader returns the identity of the leader
func (api *APIImpl) RaftLeader(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	if leader := group.GetLeader(); leader != "" {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, leader)
	} else {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// Hostname returns the hostname this process executes on
func (api *APIImpl) Hostname(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	if hostname, err := os.Hostname(); err == nil {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, hostname)
	} else {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err.Error())
	}
}

// CheckMySQLCluster checks whether a cluster's collected metric is within its threshold
func (api *APIImpl) CheckMySQLCluster(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	clusterName := ps.ByName("clusterName")
	metricResult, threshold := api.throttler.GetMySQLClusterMetrics(clusterName)
	value, err := metricResult.Get()

	statusCode := http.StatusInternalServerError
	if err != nil {
		statusCode = http.StatusInternalServerError
	} else if value > threshold {
		statusCode = http.StatusTooManyRequests
	} else {
		statusCode = http.StatusOK
	}
	w.WriteHeader(statusCode)
	if r.Method == http.MethodGet {
		fmt.Fprintf(w, "HTTP %d\n%+v\n%+v/%+v", statusCode, err, value, threshold)
	}
}

// AggregatedMetrics returns a snapshot of all current aggregated metrics
func (api *APIImpl) AggregatedMetrics(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	aggregatedMetrics := api.throttler.AggregatedMetrics()
	for metricName, metric := range aggregatedMetrics {
		value, err := metric.Get()
		fmt.Fprintf(w, "%s: %+v, %+v\n", metricName, value, err)
	}
}

// register is a wrapper function for accepting both GET and HEAD requests
func register(router *httprouter.Router, path string, f httprouter.Handle) {
	router.HEAD(path, f)
	router.GET(path, f)
}

// ConfigureRoutes configures a set of HTTP routes to be actions dispatched by the
// given api's methods.
func ConfigureRoutes(api API) *httprouter.Router {
	router := httprouter.New()
	register(router, "/lb-check", api.LbCheck)
	register(router, "/leader-check", api.LeaderCheck)
	register(router, "/raft/leader", api.RaftLeader)
	register(router, "/hostname", api.Hostname)
	register(router, "/check/:app/mysql/:clusterName", api.CheckMySQLCluster)
	register(router, "/aggregated-metrics", api.AggregatedMetrics)
	return router
}
