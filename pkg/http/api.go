package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/github/freno/pkg/config"
	"github.com/github/freno/pkg/group"
	"github.com/github/freno/pkg/throttle"
	metrics "github.com/rcrowley/go-metrics"
	"github.com/rcrowley/go-metrics/exp"

	"github.com/julienschmidt/httprouter"
)

// API exposes the contract for the throttler's web API
type API interface {
	LbCheck(w http.ResponseWriter, _ *http.Request, _ httprouter.Params)
	LeaderCheck(w http.ResponseWriter, _ *http.Request, _ httprouter.Params)
	ConsensusLeader(w http.ResponseWriter, _ *http.Request, _ httprouter.Params)
	ConsensusState(w http.ResponseWriter, _ *http.Request, _ httprouter.Params)
	ConsensusStatus(w http.ResponseWriter, _ *http.Request, _ httprouter.Params)
	Hostname(w http.ResponseWriter, _ *http.Request, _ httprouter.Params)
	WriteCheck(w http.ResponseWriter, r *http.Request, _ httprouter.Params)
	WriteCheckIfExists(w http.ResponseWriter, r *http.Request, _ httprouter.Params)
	ReadCheck(w http.ResponseWriter, r *http.Request, _ httprouter.Params)
	ReadCheckIfExists(w http.ResponseWriter, r *http.Request, _ httprouter.Params)
	AggregatedMetrics(w http.ResponseWriter, r *http.Request, _ httprouter.Params)
	MetricsHealth(w http.ResponseWriter, r *http.Request, _ httprouter.Params)
	ThrottleApp(w http.ResponseWriter, r *http.Request, _ httprouter.Params)
	UnthrottleApp(w http.ResponseWriter, r *http.Request, _ httprouter.Params)
	ThrottledApps(w http.ResponseWriter, r *http.Request, _ httprouter.Params)
	RecentApps(w http.ResponseWriter, r *http.Request, _ httprouter.Params)
	Help(w http.ResponseWriter, r *http.Request, _ httprouter.Params)
	MemcacheConfig(w http.ResponseWriter, r *http.Request, _ httprouter.Params)
}

var endpoints = []string{} // known API URIs

var okIfNotExistsFlags = &throttle.CheckFlags{OKIfNotExists: true}

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
	if api.consensusService.IsLeader() {
		statusCode = http.StatusOK
	}
	w.WriteHeader(statusCode)
	if r.Method == http.MethodGet {
		json.NewEncoder(w).Encode(fmt.Sprintf("HTTP %d", statusCode))
	}
}

// ConsensusLeader returns the identity of the leader
func (api *APIImpl) ConsensusLeader(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	if leader := api.consensusService.GetLeader(); leader != "" {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(leader)
	} else {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// ConsensusLeader returns the consensus state of this node
func (api *APIImpl) ConsensusState(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	json.NewEncoder(w).Encode(api.consensusService.GetStateDescription())
}

// ConsensusLeader returns the consensus state of this node
func (api *APIImpl) ConsensusStatus(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(api.consensusService.GetStatus())
}

// Hostname returns the hostname this process executes on
func (api *APIImpl) Hostname(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	if api.hostname != "" {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(api.hostname)
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

// Check checks whether a collected metric is within its threshold
func (api *APIImpl) check(w http.ResponseWriter, r *http.Request, ps httprouter.Params, flags *throttle.CheckFlags) {
	appName := ps.ByName("app")
	storeType := ps.ByName("storeType")
	storeName := ps.ByName("storeName")
	remoteAddr := r.Header.Get("X-Forwarded-For")
	if remoteAddr == "" {
		remoteAddr = r.RemoteAddr
		remoteAddr = strings.Split(remoteAddr, ":")[0]
	}
	flags.LowPriority = (r.URL.Query().Get("p") == "low")

	checkResult := api.throttlerCheck.Check(appName, storeType, storeName, remoteAddr, flags)
	if checkResult.StatusCode == http.StatusNotFound && flags.OKIfNotExists {
		checkResult.StatusCode = http.StatusOK // 200
	}

	api.respondToCheckRequest(w, r, checkResult)
}

// WriteCheck
func (api *APIImpl) WriteCheck(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	api.check(w, r, ps, throttle.StandardCheckFlags)
}

// WriteCheckIfExists checks for a metric, but reports an OK if the metric does not exist.
// If the metric does exist, then all usual checks are made.
func (api *APIImpl) WriteCheckIfExists(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	api.check(w, r, ps, okIfNotExistsFlags)
}

func (api *APIImpl) readCheck(w http.ResponseWriter, r *http.Request, ps httprouter.Params, flags *throttle.CheckFlags) {
	if overrideThreshold, err := strconv.ParseFloat(ps.ByName("threshold"), 64); err != nil {
		api.respondGeneric(w, r, err)
	} else {
		flags.ReadCheck = true
		flags.OverrideThreshold = overrideThreshold
		api.check(w, r, ps, flags)
	}
}

// ReadCheck
func (api *APIImpl) ReadCheck(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	api.readCheck(w, r, ps, &throttle.CheckFlags{})
}

// WriteCheckIfExists checks for a metric, but reports an OK if the metric does not exist.
// If the metric does exist, then all usual checks are made.
func (api *APIImpl) ReadCheckIfExists(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	api.readCheck(w, r, ps, &throttle.CheckFlags{OKIfNotExists: true})
}

// AggregatedMetrics returns a snapshot of all current aggregated metrics
func (api *APIImpl) AggregatedMetrics(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	brief := (r.URL.Query().Get("brief") == "true")

	w.Header().Set("Content-Type", "application/json")
	aggregatedMetrics := api.throttlerCheck.AggregatedMetrics()
	responseMap := map[string]string{}
	for metricName, metric := range aggregatedMetrics {
		value, err := metric.Get()
		description := ""
		if err == nil {
			if brief {
				description = fmt.Sprintf("%.3f", value)
			} else {
				description = fmt.Sprintf("%f", value)
			}
		} else {
			description = fmt.Sprintf("error: %s", err.Error())
		}
		responseMap[metricName] = description
	}
	json.NewEncoder(w).Encode(responseMap)
}

// MetricsHealth returns the time since last "OK" check per-metric
func (api *APIImpl) MetricsHealth(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	w.Header().Set("Content-Type", "application/json")
	metricsHealth := api.throttlerCheck.MetricsHealth()
	json.NewEncoder(w).Encode(metricsHealth)
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
		ratio = -1
	} else if ratio, err = strconv.ParseFloat(ps.ByName("ratio"), 64); err != nil {
		goto response
	} else if ratio < 0 || ratio > 1 {
		err = fmt.Errorf("ratio must be in [0..1] range; got %+v", ratio)
		goto response
	}
	err = api.consensusService.ThrottleApp(appName, ttlMinutes, expireAt, ratio)

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

// ThrottledApps returns a snapshot of all currently throttled apps
func (api *APIImpl) RecentApps(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	var err error
	var lastMinutes int64
	if ps.ByName("lastMinutes") != "" {
		if lastMinutes, err = strconv.ParseInt(ps.ByName("lastMinutes"), 10, 64); err != nil {
			api.respondGeneric(w, r, err)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	recentApps := api.consensusService.RecentAppsMap()
	if lastMinutes > 0 {
		for key, recentApp := range recentApps {
			if recentApp.MinutesSinceChecked > lastMinutes {
				delete(recentApps, key)
			}
		}
	}
	json.NewEncoder(w).Encode(recentApps)
}

// ThrottledApps returns a snapshot of all currently throttled apps
func (api *APIImpl) Help(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(endpoints)
}

// MemcacheConfig outputs the memcache configuration being used, so clients can
// implement more efficient access strategies
func (api *APIImpl) MemcacheConfig(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	memcacheConfig := struct {
		MemcacheServers []string
		MemcachePath    string
	}{
		config.Settings().MemcacheServers,
		config.Settings().MemcachePath,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(memcacheConfig)
}

// register is a wrapper function for accepting both GET and HEAD requests
func register(router *httprouter.Router, path string, f httprouter.Handle) {
	router.HEAD(path, f)
	router.GET(path, f)

	endpoints = append(endpoints, path)
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
	register(router, "/raft/leader", api.ConsensusLeader)
	register(router, "/raft/state", api.ConsensusState)
	register(router, "/consensus/leader", api.ConsensusLeader)
	register(router, "/consensus/state", api.ConsensusState)
	register(router, "/consensus/status", api.ConsensusStatus)
	register(router, "/hostname", api.Hostname)

	register(router, "/check/:app/:storeType/:storeName", api.WriteCheck)
	register(router, "/check-if-exists/:app/:storeType/:storeName", api.WriteCheckIfExists)
	register(router, "/check-read/:app/:storeType/:storeName/:threshold", api.ReadCheck)
	register(router, "/check-read-if-exists/:app/:storeType/:storeName/:threshold", api.ReadCheckIfExists)

	register(router, "/aggregated-metrics", api.AggregatedMetrics)
	register(router, "/metrics-health", api.MetricsHealth)

	register(router, "/throttle-app/:app", api.ThrottleApp)
	register(router, "/throttle-app/:app/ratio/:ratio", api.ThrottleApp)
	register(router, "/throttle-app/:app/ttl/:ttlMinutes", api.ThrottleApp)
	register(router, "/throttle-app/:app/ttl/:ttlMinutes/ratio/:ratio", api.ThrottleApp)
	register(router, "/unthrottle-app/:app", api.UnthrottleApp)
	register(router, "/throttled-apps", api.ThrottledApps)
	register(router, "/recent-apps", api.RecentApps)
	register(router, "/recent-apps/:lastMinutes", api.RecentApps)

	register(router, "/debug/vars", metricsHandle)
	register(router, "/debug/metrics", metricsHandle)

	if config.Settings().EnableProfiling {
		router.HandlerFunc(http.MethodGet, "/debug/pprof/", pprof.Index)
		router.HandlerFunc(http.MethodGet, "/debug/pprof/cmdline", pprof.Cmdline)
		router.HandlerFunc(http.MethodGet, "/debug/pprof/profile", pprof.Profile)
		router.HandlerFunc(http.MethodGet, "/debug/pprof/symbol", pprof.Symbol)
		router.HandlerFunc(http.MethodGet, "/debug/pprof/trace", pprof.Trace)
		router.Handler(http.MethodGet, "/debug/pprof/goroutine", pprof.Handler("goroutine"))
		router.Handler(http.MethodGet, "/debug/pprof/heap", pprof.Handler("heap"))
		router.Handler(http.MethodGet, "/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
		router.Handler(http.MethodGet, "/debug/pprof/block", pprof.Handler("block"))
	}

	register(router, "/help", api.Help)

	router.GET("/config/memcache", api.MemcacheConfig)

	return router
}
