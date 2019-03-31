package config

//
// General-store configuration
//

type AppSettings struct {
	Deprioritized []string // list of deprioritized app names
}

// Hook to implement adjustments after reading each configuration file.
func (settings *AppSettings) postReadAdjustments() error {
	return nil
}
