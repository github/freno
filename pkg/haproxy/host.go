package haproxy

type BackendHostStatus string

const (
	StatusDown    BackendHostStatus = "DOWN"
	StatusNOLB    BackendHostStatus = "NOLB"
	StatusUp      BackendHostStatus = "UP"
	StatusNoCheck BackendHostStatus = "no check"
	StatusUnknown BackendHostStatus = "unkown"
)

func ToBackendHostStatus(status string) BackendHostStatus {
	switch status {
	case "DOWN":
		return StatusDown
	case "NOLB":
		return StatusNOLB
	case "UP":
		return StatusUp
	case "no check":
		return StatusNoCheck
	default:
		return StatusUnknown
	}
}

type BackendHost struct {
	Hostname        string
	Status          BackendHostStatus
	IsTransitioning bool
}

func NewBackendHost(hostname string, status BackendHostStatus, isTransitioning bool) *BackendHost {
	return &BackendHost{Hostname: hostname, Status: status, IsTransitioning: isTransitioning}
}
