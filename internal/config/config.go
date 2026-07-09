package config

import "fmt"

// Config stores configurations for the api
type Config struct {
	DB     DBConfig
	Log    LogConfig
	Server ServerConfig
	Auth   AuthConfig
}

// DBConfig represent the db config
type DBConfig struct {
	DatabaseName     string
	DatabaseHost     string
	DatabasePort     string
	DatabaseUser     string
	DatabasePassword string
}

// LogConfig stores configs for logger
type LogConfig struct {
	Level   string
	LogDir  string
	LogFile string
}

// ServerConfig stores configs for server
type ServerConfig struct {
	Port int
}

// AuthConfig stores configs for auth.
type AuthConfig struct {
	JWTSecret string
}

// New parses flags and env variables and returns a populated Config.
func New() (*Config, error) {
	return Load()
}

// DatabaseURL return PSQL DB DSN
func (c *DBConfig) DatabaseURL() string {
	return fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=disable",
		c.DatabaseUser,
		c.DatabasePassword,
		c.DatabaseHost,
		c.DatabasePort,
		c.DatabaseName,
	)
}
