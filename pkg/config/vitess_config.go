package config

import (
	"fmt"
	"strings"
)

//
// HAProxy-specific configuration
//

type VitessConfigurationSettings struct {
	API         string
	Cells       []string
	Keyspace    string
	Shard       string
	TabletType  string
	TimeoutSecs uint
}

func (settings *VitessConfigurationSettings) IsEmpty() bool {
	if settings.API == "" {
		return true
	}
	if settings.Keyspace == "" {
		return true
	}
	return false
}

func (settings *VitessConfigurationSettings) postReadAdjustments() error {
	switch strings.ToUpper(strings.TrimSpace(settings.TabletType)) {
	case "", "REPLICA":
		settings.TabletType = "REPLICA"
	case "MASTER", "PRIMARY":
		settings.TabletType = "MASTER"
	default:
		return fmt.Errorf("unsupported Vitess tablet type %q", settings.TabletType)
	}
	return nil
}
