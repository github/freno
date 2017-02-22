package http

import (
	"fmt"
	"net/http"

	"github.com/github/freno/go/group"

	"github.com/julienschmidt/httprouter"
)

// API exposes the contract for the throttler's web API
type API interface {
	LbCheck(w http.ResponseWriter, _ *http.Request, _ httprouter.Params)
	LeaderCheck(w http.ResponseWriter, _ *http.Request, _ httprouter.Params)
}

// APIImpl implements the API
type APIImpl struct {
}

func NewAPIImpl() *APIImpl {
	return &APIImpl{}
}

// LbCheck responds to LbCheck with HTTP 200
func (api *APIImpl) LbCheck(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	w.WriteHeader(http.StatusOK)
}

// LeaderCheck responds with HTTP 200 when this node is a raft leader, otherwise 404
// This is a useful check for HAProxy routing
func (api *APIImpl) LeaderCheck(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	responseCode := http.StatusNotFound
	if group.IsLeader() {
		responseCode = http.StatusOK
	}
	w.WriteHeader(responseCode)
	if r.Method == http.MethodGet {
		fmt.Fprintf(w, "HTTP %d", responseCode)
	}
}

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
	return router
}
