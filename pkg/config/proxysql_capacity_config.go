package config

import (
	"fmt"
	"math"
)

const ProxySQLDefaultAdminPort = 6032

type ProxySQLBackendLimit struct {
	Host           string
	Port           int
	MaxConnections int64
}

// ProxySQLCapacityConfigurationSettings uses explicitly verified limits because
// ProxySQL's stats-only account cannot read the runtime configuration tables.
type ProxySQLCapacityConfigurationSettings struct {
	Metric                 string
	FrontendMaxConnections int64
	WriterHostgroupID      int
	BackendLimits          []ProxySQLBackendLimit
}

func (settings *ProxySQLCapacityConfigurationSettings) Validate() error {
	if settings.Metric != "frontend" && settings.Metric != "writer-backend" {
		return fmt.Errorf("Metric must be frontend or writer-backend")
	}
	if settings.FrontendMaxConnections <= 0 || settings.WriterHostgroupID <= 0 {
		return fmt.Errorf("FrontendMaxConnections and WriterHostgroupID must be positive")
	}
	if len(settings.BackendLimits) == 0 {
		return fmt.Errorf("BackendLimits must identify the expected writer backends")
	}
	seen := make(map[string]bool)
	for _, backend := range settings.BackendLimits {
		if backend.Host == "" || backend.Port < 0 || backend.Port > 65535 || backend.MaxConnections <= 0 {
			return fmt.Errorf("BackendLimits must contain a host, valid port (0 for a Unix socket), and positive MaxConnections")
		}
		key := fmt.Sprintf("%s:%d", backend.Host, backend.Port)
		if seen[key] {
			return fmt.Errorf("duplicate BackendLimits endpoint %q", key)
		}
		seen[key] = true
	}
	return nil
}

func (settings *MySQLClusterConfigurationSettings) adjustProxySQLCapacity() error {
	if err := settings.ProxySQLCapacitySettings.Validate(); err != nil {
		return err
	}
	if settings.User == "" || settings.Password == "" {
		return fmt.Errorf("explicit stats-only User and Password are required; MySQL credentials are not inherited")
	}
	if settings.MetricQuery != "" {
		return fmt.Errorf("MetricQuery must be empty; capacity probes only execute built-in ProxySQL stats queries")
	}
	if settings.Port == 0 {
		settings.Port = ProxySQLDefaultAdminPort
	}
	if settings.Port < 1 || settings.Port > 65535 || settings.Port == 3306 || settings.Port == 6033 {
		return fmt.Errorf("Port must identify a ProxySQL admin listener, not a MySQL or ProxySQL application port")
	}
	if len(settings.ProxySQLSettings.Addresses) > 0 || settings.ProxySQLSettings.HostgroupID != 0 {
		return fmt.Errorf("ProxySQLSettings discovers MySQL backends, not ProxySQL capacity endpoints")
	}
	discoverySources := 0
	for _, present := range []bool{
		!settings.HAProxySettings.IsEmpty(),
		!settings.VitessSettings.IsEmpty(),
		!settings.StaticHostsSettings.IsEmpty(),
	} {
		if present {
			discoverySources++
		}
	}
	if discoverySources != 1 {
		return fmt.Errorf("exactly one HAProxy, Vitess, or static proxy-host discovery source is required")
	}
	if math.IsNaN(settings.ThrottleThreshold) || math.IsInf(settings.ThrottleThreshold, 0) ||
		settings.ThrottleThreshold <= 0 || settings.ThrottleThreshold > 100 {
		return fmt.Errorf("ThrottleThreshold must be a finite percentage in (0, 100]")
	}
	if settings.RecoveryThreshold == nil || math.IsNaN(*settings.RecoveryThreshold) ||
		math.IsInf(*settings.RecoveryThreshold, 0) || *settings.RecoveryThreshold < 0 ||
		*settings.RecoveryThreshold >= settings.ThrottleThreshold {
		return fmt.Errorf("RecoveryThreshold must be a finite non-negative percentage below ThrottleThreshold")
	}
	if settings.RecoveryDurationMillis < 10000 {
		return fmt.Errorf("RecoveryDurationMillis must be at least 10000 (two sampling intervals)")
	}
	if settings.IgnoreHostsCount != 0 || settings.IgnoreHostsThreshold != 0 || len(settings.IgnoreHosts) != 0 {
		return fmt.Errorf("capacity probes must not ignore hosts or errors")
	}
	settings.FailOnNoHosts = true
	settings.HttpCheckPort = -1
	settings.HttpCheckPath = ""
	settings.CacheMillis = 0 // The sampler shares a bounded success/error cache across both capacity metrics.
	return nil
}
