package config

//
// General-store configuration
//

type StoresSettings struct {
	MySQL MySQLConfigurationSettings // Any and all MySQL setups go here

	// Futuristic stores can come here.
}

// Hook to implement adjustments after reading each configuration file.
func (settings *StoresSettings) postReadAdjustments() error {
	if err := settings.MySQL.postReadAdjustments(); err != nil {
		return err
	}
	return nil
}
