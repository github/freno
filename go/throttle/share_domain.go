package throttle

import (
	"net/http"

	"github.com/github/freno/go/base"
)

type ShareDomainService struct {
	getServicesFunc func() ([]string, error)
	httpClient      *http.Client
}

func NewShareDomainService(getServicesFunc func() ([]string, error)) *ShareDomainService {
	return &ShareDomainService{
		getServicesFunc: getServicesFunc,
		httpClient:      base.SetupHttpClient(0),
	}
}
