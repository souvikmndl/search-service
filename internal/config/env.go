package config

import (
	"flag"
	"io"
	"os"
	"strconv"
)

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}

// Load reads env variables and CLI flags and populates Config.
// Precedence: CLI flag > env var > default value.
func Load() (*Config, error) {
	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	dbName := fs.String("db-name", envStr("DB_NAME", "search_service"), "database name")
	dbHost := fs.String("db-host", envStr("DB_HOST", "127.0.0.1"), "database host")
	dbPort := fs.String("db-port", envStr("DB_PORT", "5432"), "database port")
	dbUser := fs.String("db-user", envStr("DB_USER", "postgres"), "database user")
	dbPassword := fs.String("db-password", envStr("DB_PASSWORD", "secret"), "database password")

	logLevel := fs.String("log-level", envStr("LOG_LEVEL", "info"), "log level")
	logDir := fs.String("log-dir", envStr("LOG_DIR", "/var/log/search_service/"), "log directory")
	logFile := fs.String("log-file", envStr("LOG_FILE", "api.log"), "log file name")

	serverPort := fs.Int("server-port", envInt("SERVER_PORT", 3000), "server port")

	jwtSecret := fs.String("jwt-secret", envStr("JWT_SECRET", "secret"), "JWT signing secret")

	// Unknown flags (e.g. test binary flags) stop parsing but are not an error —
	// registered flags that were not reached simply retain their defaults.
	_ = fs.Parse(os.Args[1:])

	return &Config{
		DB: DBConfig{
			DatabaseName:     *dbName,
			DatabaseHost:     *dbHost,
			DatabasePort:     *dbPort,
			DatabaseUser:     *dbUser,
			DatabasePassword: *dbPassword,
		},
		Log: LogConfig{
			Level:   *logLevel,
			LogDir:  *logDir,
			LogFile: *logFile,
		},
		Server: ServerConfig{
			Port: *serverPort,
		},
		Auth: AuthConfig{
			JWTSecret: *jwtSecret,
		},
	}, nil
}
