package config

import "testing"

func TestEnvStr(t *testing.T) {
	t.Run("returns env value when set", func(t *testing.T) {
		t.Setenv("_SEARCH_TEST_STR", "from_env")
		if got := envStr("_SEARCH_TEST_STR", "default"); got != "from_env" {
			t.Fatalf("expected from_env, got %q", got)
		}
	})
	t.Run("returns default when not set", func(t *testing.T) {
		if got := envStr("_SEARCH_TEST_UNSET", "default"); got != "default" {
			t.Fatalf("expected default, got %q", got)
		}
	})
	t.Run("returns default when empty string", func(t *testing.T) {
		t.Setenv("_SEARCH_TEST_EMPTY", "")
		if got := envStr("_SEARCH_TEST_EMPTY", "default"); got != "default" {
			t.Fatalf("expected default for empty env var, got %q", got)
		}
	})
}

func TestEnvInt(t *testing.T) {
	t.Run("returns env value when set", func(t *testing.T) {
		t.Setenv("_SEARCH_TEST_INT", "8080")
		if got := envInt("_SEARCH_TEST_INT", 3000); got != 8080 {
			t.Fatalf("expected 8080, got %d", got)
		}
	})
	t.Run("returns default when not set", func(t *testing.T) {
		if got := envInt("_SEARCH_TEST_UNSET_INT", 3000); got != 3000 {
			t.Fatalf("expected 3000, got %d", got)
		}
	})
	t.Run("returns default when not a number", func(t *testing.T) {
		t.Setenv("_SEARCH_TEST_BADINT", "notanumber")
		if got := envInt("_SEARCH_TEST_BADINT", 3000); got != 3000 {
			t.Fatalf("expected 3000 for non-numeric env var, got %d", got)
		}
	})
}

func TestDatabaseURL(t *testing.T) {
	cfg := &DBConfig{
		DatabaseUser:     "alice",
		DatabasePassword: "s3cr3t",
		DatabaseHost:     "db.example.com",
		DatabasePort:     "5432",
		DatabaseName:     "myapp",
	}
	want := "postgresql://alice:s3cr3t@db.example.com:5432/myapp?sslmode=disable"
	if got := cfg.DatabaseURL(); got != want {
		t.Fatalf("DatabaseURL() = %q, want %q", got, want)
	}
}

func TestLoadUsesHardcodedDefaults(t *testing.T) {
	// Empty-string env vars cause envStr/envInt to fall back to hardcoded defaults.
	for _, key := range []string{"DB_NAME", "DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD",
		"LOG_LEVEL", "LOG_DIR", "LOG_FILE", "SERVER_PORT", "JWT_SECRET"} {
		t.Setenv(key, "")
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	cases := []struct {
		field string
		got   string
		want  string
	}{
		{"DB.DatabaseName", cfg.DB.DatabaseName, "search_service"},
		{"DB.DatabaseHost", cfg.DB.DatabaseHost, "127.0.0.1"},
		{"DB.DatabasePort", cfg.DB.DatabasePort, "5432"},
		{"DB.DatabaseUser", cfg.DB.DatabaseUser, "postgres"},
		{"Log.Level", cfg.Log.Level, "info"},
		{"Log.LogFile", cfg.Log.LogFile, "api.log"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: expected %q, got %q", c.field, c.want, c.got)
		}
	}
	if cfg.Server.Port != 3000 {
		t.Errorf("Server.Port: expected 3000, got %d", cfg.Server.Port)
	}
}

func TestLoadReadsEnvVars(t *testing.T) {
	t.Setenv("DB_NAME", "integration_db")
	t.Setenv("DB_HOST", "postgres.internal")
	t.Setenv("SERVER_PORT", "9090")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.DB.DatabaseName != "integration_db" {
		t.Errorf("DB.DatabaseName: expected integration_db, got %q", cfg.DB.DatabaseName)
	}
	if cfg.DB.DatabaseHost != "postgres.internal" {
		t.Errorf("DB.DatabaseHost: expected postgres.internal, got %q", cfg.DB.DatabaseHost)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("Server.Port: expected 9090, got %d", cfg.Server.Port)
	}
}
