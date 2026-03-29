package config

// NewForTest creates a Config with the decoded password set, for use in tests.
func NewForTest(port int, username, password string) *Config {
	return &Config{
		Port:                  port,
		Username:              username,
		SessionTimeoutMinutes: 30,
		RestartMinMinutes:     5,
		RestartMaxMinutes:     15,
		LockMinMinutes:        5,
		LockMaxMinutes:        15,
		password:              password,
	}
}

// NewForTestWithTimeout creates a Config with a custom session timeout.
func NewForTestWithTimeout(port int, username, password string, sessionTimeoutMinutes int) *Config {
	return &Config{
		Port:                  port,
		Username:              username,
		SessionTimeoutMinutes: sessionTimeoutMinutes,
		RestartMinMinutes:     5,
		RestartMaxMinutes:     15,
		LockMinMinutes:        5,
		LockMaxMinutes:        15,
		password:              password,
	}
}
