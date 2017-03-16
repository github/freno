package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/github/freno/go/base"
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
	ThrottleApp(w http.ResponseWriter, r *http.Request, _ httprouter.Params)
	UnthrottleApp(w http.ResponseWriter, r *http.Request, _ httprouter.Params)
}

type CheckResponse struct {
	StatusCode int
	Message    string
	Value      float64
	Threshold  float64
}

func NewCheckResponse(statusCode int, err error, value float64, threshold float64) *CheckResponse {
	response := &CheckResponse{
		StatusCode: statusCode,
		Value:      value,
		Threshold:  threshold,
	}
	if err != nil {
		response.Message = err.Error()
	}
	return response
}

type GeneralResponse struct {
	StatusCode int
	Message    string
}

func NewGeneralResponse(statusCode int, message string) *GeneralResponse {
	return &GeneralResponse{StatusCode: statusCode, Message: message}
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

func (api *APIImpl) respondOK(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
	}
	statusCode := http.StatusOK
	w.WriteHeader(statusCode)
	if r.Method == http.MethodGet {
		json.NewEncoder(w).Encode(NewGeneralResponse(statusCode, "OK"))
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

func (api *APIImpl) checkAppMetricResult(w http.ResponseWriter, r *http.Request, ps httprouter.Params, metricResultFunc base.MetricResultFunc) {
	appName := ps.ByName("app")
	metricResult, threshold := api.throttler.AppRequestMetricResult(appName, metricResultFunc)
	value, err := metricResult.Get()

	statusCode := http.StatusInternalServerError
	if err == base.AppDeniedError {
		// app specifically not allowed to get metrics
		statusCode = http.StatusExpectationFailed
	} else if err != nil {
		// any error
		statusCode = http.StatusInternalServerError
	} else if value > threshold {
		// casual throttling
		statusCode = http.StatusTooManyRequests
		err = base.ThresholdExceededError
	} else {
		// all good!
		statusCode = http.StatusOK
	}
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(statusCode)
	if r.Method == http.MethodGet {
		json.NewEncoder(w).Encode(NewCheckResponse(statusCode, err, value, threshold))
	}
}

// CheckMySQLCluster checks whether a cluster's collected metric is within its threshold
func (api *APIImpl) CheckMySQLCluster(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	clusterName := ps.ByName("clusterName")
	var metricResultFunc base.MetricResultFunc = func() (metricResult base.MetricResult, threshold float64) {
		return api.throttler.GetMySQLClusterMetrics(clusterName)
	}
	api.checkAppMetricResult(w, r, ps, metricResultFunc)
}

// AggregatedMetrics returns a snapshot of all current aggregated metrics
func (api *APIImpl) AggregatedMetrics(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	w.Header().Set("Content-Type", "application/json")
	aggregatedMetrics := api.throttler.AggregatedMetrics()
	responseMap := map[string]string{}
	for metricName, metric := range aggregatedMetrics {
		value, err := metric.Get()
		responseMap[metricName] = fmt.Sprintf("%+v, %+v", value, err)
	}
	json.NewEncoder(w).Encode(responseMap)
}

// ThrottleApp forcibly marks given app as throttled. Future requests by this app will be denied.
func (api *APIImpl) ThrottleApp(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	appName := ps.ByName("app")
	api.throttler.ThrottleApp(appName)

	api.respondOK(w, r)
}

// ThrottleApp unthrottles given app.
func (api *APIImpl) UnthrottleApp(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	appName := ps.ByName("app")
	api.throttler.UnthrottleApp(appName)

	api.respondOK(w, r)
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
	register(router, "/throttle-app/:app", api.ThrottleApp)
	register(router, "/unthrottle-app/:app", api.UnthrottleApp)
	return router
}
