package config

// NewForTest creates a Config with the decoded password set, for use in tests.
func NewForTest(port int, username, password string) *Config {
	return &Config{
		Port:                  port,
		Username:              username,
		SessionTimeoutMinutes: 30,
		password:              password,
	}
}

// NewForTestWithTimeout creates a Config with a custom session timeout.
func NewForTestWithTimeout(port int, username, password string, sessionTimeoutMinutes int) *Config {
	return &Config{
		Port:                  port,
		Username:              username,
		SessionTimeoutMinutes: sessionTimeoutMinutes,
		password:              password,
	}
}
