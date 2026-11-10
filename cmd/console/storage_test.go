package main

import "testing"

func TestStorageSelectionNeverFallsBackFromConfiguredPostgres(t *testing.T) {
	for _, test := range []struct {
		name, engine, dsn, path, want string
		invalid                       bool
	}{
		{name: "default", want: "sqlite"},
		{name: "legacy URL", dsn: "postgres://unavailable/console", want: "postgres"},
		{name: "explicit postgres", engine: "postgres", dsn: "postgres://unavailable/console", want: "postgres"},
		{name: "missing URL", engine: "postgres", invalid: true},
		{name: "conflicting URL", engine: "sqlite", dsn: "postgres://unavailable/console", invalid: true},
		{name: "conflicting path", dsn: "postgres://unavailable/console", path: "state.db", invalid: true},
		{name: "unknown driver", engine: "other", invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("CONSOLE_STORAGE", test.engine)
			t.Setenv("CONSOLE_DATABASE_URL", test.dsn)
			t.Setenv("CONSOLE_SQLITE_PATH", test.path)
			config, err := storageConfiguration()
			if (err != nil) != test.invalid {
				t.Fatal("unexpected validation result", err)
			}
			if err == nil && config.engine != test.want {
				t.Fatalf("backend %s, want %s", config.engine, test.want)
			}
		})
	}
}
