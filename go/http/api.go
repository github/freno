package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/github/freno/go/group"
	"github.com/github/freno/go/throttle"
	metrics "github.com/rcrowley/go-metrics"
	"github.com/rcrowley/go-metrics/exp"

	"github.com/julienschmidt/httprouter"
)

// API exposes the contract for the throttler's web API
type API interface {
	LbCheck(w http.ResponseWriter, _ *http.Request, _ httprouter.Params)
	LeaderCheck(w http.ResponseWriter, _ *http.Request, _ httprouter.Params)
	RaftLeader(w http.ResponseWriter, _ *http.Request, _ httprouter.Params)
	RaftState(w http.ResponseWriter, _ *http.Request, _ httprouter.Params)
	Hostname(w http.ResponseWriter, _ *http.Request, _ httprouter.Params)
	Check(w http.ResponseWriter, r *http.Request, _ httprouter.Params)
	AggregatedMetrics(w http.ResponseWriter, r *http.Request, _ httprouter.Params)
	ThrottleApp(w http.ResponseWriter, r *http.Request, _ httprouter.Params)
	UnthrottleApp(w http.ResponseWriter, r *http.Request, _ httprouter.Params)
	ThrottledApps(w http.ResponseWriter, r *http.Request, _ httprouter.Params)
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
	throttlerCheck   *throttle.ThrottlerCheck
	consensusService group.ConsensusService
	hostname         string
}

// NewAPIImpl creates a new instance of the API implementation
func NewAPIImpl(throttlerCheck *throttle.ThrottlerCheck, consensusService group.ConsensusService) *APIImpl {
	api := &APIImpl{
		throttlerCheck:   throttlerCheck,
		consensusService: consensusService,
	}
	if hostname, err := os.Hostname(); err == nil {
		api.hostname = hostname
	}
	return api
}

// respondGeneric will generate a generic response in the form of {status, message}
// It will set response based on whether request is HEAD/GET and based on given error
func (api *APIImpl) respondGeneric(w http.ResponseWriter, r *http.Request, e error) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
	}
	var generalRespnse *GeneralResponse
	if e == nil {
		generalRespnse = NewGeneralResponse(http.StatusOK, "OK")
	} else {
		generalRespnse = NewGeneralResponse(http.StatusInternalServerError, e.Error())
	}
	w.WriteHeader(generalRespnse.StatusCode)
	if r.Method == http.MethodGet {
		json.NewEncoder(w).Encode(generalRespnse)
	}
}

// LbCheck responds to LbCheck with HTTP 200
func (api *APIImpl) LbCheck(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	api.respondGeneric(w, r, nil)
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

// RaftState returns the state of the raft node
func (api *APIImpl) RaftState(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	fmt.Fprintf(w, group.GetState().String())
}

// Hostname returns the hostname this process executes on
func (api *APIImpl) Hostname(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	if api.hostname != "" {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, api.hostname)
	} else {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (api *APIImpl) respondToCheckRequest(w http.ResponseWriter, r *http.Request, checkResult *throttle.CheckResult) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(checkResult.StatusCode)
	if r.Method == http.MethodGet {
		json.NewEncoder(w).Encode(checkResult)
	}
}

// CheckMySQLCluster checks whether a cluster's collected metric is within its threshold
func (api *APIImpl) Check(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	appName := ps.ByName("app")
	storeType := ps.ByName("storeType")
	storeName := ps.ByName("storeName")
	checkResult := api.throttlerCheck.Check(appName, storeType, storeName)
	api.respondToCheckRequest(w, r, checkResult)
}

// AggregatedMetrics returns a snapshot of all current aggregated metrics
func (api *APIImpl) AggregatedMetrics(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	w.Header().Set("Content-Type", "application/json")
	aggregatedMetrics := api.throttlerCheck.AggregatedMetrics()
	responseMap := map[string]string{}
	for metricName, metric := range aggregatedMetrics {
		value, err := metric.Get()
		responseMap[metricName] = fmt.Sprintf("%+v, %+v", value, err)
	}
	json.NewEncoder(w).Encode(responseMap)
}

// ThrottleApp forcibly marks given app as throttled. Future requests by this app may be denied.
func (api *APIImpl) ThrottleApp(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	appName := ps.ByName("app")
	var expireAt time.Time // default zero
	var ttlMinutes int64
	var ratio float64
	var err error
	if ps.ByName("ttlMinutes") == "" {
		ttlMinutes = 0
	} else if ttlMinutes, err = strconv.ParseInt(ps.ByName("ttlMinutes"), 10, 64); err != nil {
		goto response
	}
	if ttlMinutes != 0 {
		expireAt = time.Now().Add(time.Duration(ttlMinutes) * time.Minute)
	}
	// if ttlMinutes is zero, we keep expireAt as zero, which is handled in a special way
	if ps.ByName("ratio") == "" {
		ratio = throttle.DefaultThrottleRatio
	} else if ratio, err = strconv.ParseFloat(ps.ByName("ratio"), 64); err != nil {
		goto response
	}
	if ratio < 0 || ratio > 1 {
		err = fmt.Errorf("ratio must be in [0..1] range; got %+v", ratio)
		goto response
	}
	err = api.consensusService.ThrottleApp(appName, expireAt, ratio)

response:
	api.respondGeneric(w, r, err)
}

// ThrottleApp unthrottles given app.
func (api *APIImpl) UnthrottleApp(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	appName := ps.ByName("app")
	err := api.consensusService.UnthrottleApp(appName)

	api.respondGeneric(w, r, err)
}

// ThrottledApps returns a snapshot of all currently throttled apps
func (api *APIImpl) ThrottledApps(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	w.Header().Set("Content-Type", "application/json")
	throttledApps := api.consensusService.ThrottledAppsMap()
	json.NewEncoder(w).Encode(throttledApps)
}

// register is a wrapper function for accepting both GET and HEAD requests
func register(router *httprouter.Router, path string, f httprouter.Handle) {
	router.HEAD(path, f)
	router.GET(path, f)
}

func metricsHandle(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	handler := exp.ExpHandler(metrics.DefaultRegistry)
	handler.ServeHTTP(w, r)
}

// ConfigureRoutes configures a set of HTTP routes to be actions dispatched by the
// given api's methods.
func ConfigureRoutes(api API) *httprouter.Router {
	router := httprouter.New()
	register(router, "/lb-check", api.LbCheck)
	register(router, "/_ping", api.LbCheck)
	register(router, "/status", api.LbCheck)

	register(router, "/leader-check", api.LeaderCheck)
	register(router, "/raft/leader", api.RaftLeader)
	register(router, "/raft/state", api.RaftState)
	register(router, "/hostname", api.Hostname)

	register(router, "/check/:app/:storeType/:storeName", api.Check)
	register(router, "/aggregated-metrics", api.AggregatedMetrics)

	register(router, "/throttle-app/:app/:ttlMinutes/:ratio", api.ThrottleApp)
	register(router, "/throttle-app/:app/:ttlMinutes", api.ThrottleApp)
	register(router, "/throttle-app/:app", api.ThrottleApp)
	register(router, "/unthrottle-app/:app", api.UnthrottleApp)
	register(router, "/throttled-apps", api.ThrottledApps)

	router.GET("/debug/vars", metricsHandle)
	router.GET("/debug/metrics", metricsHandle)

	return router
}
