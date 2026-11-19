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
		hostIsRelevant := false
		switch backendHost.Status {
		case StatusUp:
			if !backendHost.IsTransitioning {
				hostIsRelevant = true
			}
		case StatusDown:
			hostIsRelevant = true
		case StatusNoCheck:
			hostIsRelevant = true
		}
		if hostIsRelevant {
			hosts = append(hosts, backendHost)
		}
	}
	return hosts
}
