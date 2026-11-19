package haproxy

func FilterThrotllerHosts(backendHosts [](*BackendHost)) (hosts []string) {
	for _, backendHost := range FilterThrottlerBackendHosts(backendHosts) {
		hosts = append(hosts, backendHost.Hostname)
	}
	return hosts
}

// FilterThrottlerBackendHosts retains eligible servers, including their discovered ports.
func FilterThrottlerBackendHosts(backendHosts []*BackendHost) (hosts []*BackendHost) {
	for _, backendHost := range backendHosts {
		if backendHost.isRelevantForThrottling() {
			hosts = append(hosts, backendHost)
		}
	}
	return hosts
}

func (host *BackendHost) isRelevantForThrottling() bool {
	switch host.Status {
	case StatusUp:
		return !host.IsTransitioning
	case StatusDown, StatusNoCheck:
		return true
	default:
		return false
	}
}
