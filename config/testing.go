package config

// NewForTest creates a Config with the decoded password set, for use in tests.
func NewForTest(port int, username, password string) *Config {
	return &Config{
		Port:     port,
		Username: username,
		password: password,
	}
}
