package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fakeEnv(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := load(fakeEnv(map[string]string{"DATABASE_URL": "postgres://localhost/balans"}))
	require.NoError(t, err)

	assert.Equal(t, EnvLocal, cfg.Env)
	assert.Equal(t, ":8080", cfg.HTTPAddr)
	assert.Equal(t, ProviderMock, cfg.EmailProvider)
	assert.Equal(t, 1025, cfg.SMTPPort)
	assert.Equal(t, 20*time.Second, cfg.ShutdownTimeout)
	assert.Empty(t, cfg.EnabledModules)
	assert.True(t, cfg.MockActive())
}

func TestParseModules(t *testing.T) {
	assert.Equal(t, []string{"catalog", "sales"}, ParseModules(" Catalog, sales,,catalog "))
	assert.Empty(t, ParseModules(""))
}

func TestIsEnabled(t *testing.T) {
	cfg := Config{EnabledModules: []string{"sales"}}
	assert.True(t, cfg.IsEnabled("sales"))
	assert.False(t, cfg.IsEnabled("payroll"))
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{
			name:    "database wajib",
			env:     map[string]string{},
			wantErr: "DATABASE_URL",
		},
		{
			name:    "env tidak dikenal",
			env:     map[string]string{"DATABASE_URL": "x", "APP_ENV": "dev"},
			wantErr: "APP_ENV",
		},
		{
			name: "production menolak mock untuk modul aktif",
			env: map[string]string{
				"DATABASE_URL": "x", "APP_ENV": "production", "ENABLED_MODULES": "sales",
				"EMAIL_PROVIDER": "postmark", "SHIPMENT_PROVIDER": "biteship", "MIDTRANS_SERVER_KEY": "k",
			},
			wantErr: "modul sales aktif di production",
		},
		{
			name:    "staging butuh server key untuk webhook mock",
			env:     map[string]string{"DATABASE_URL": "x", "APP_ENV": "staging"},
			wantErr: "MIDTRANS_SERVER_KEY",
		},
		{
			name:    "port SMTP harus angka",
			env:     map[string]string{"DATABASE_URL": "x", "SMTP_PORT": "abc"},
			wantErr: "SMTP_PORT",
		},
		{
			name:    "durasi shutdown harus valid",
			env:     map[string]string{"DATABASE_URL": "x", "SHUTDOWN_TIMEOUT": "lama"},
			wantErr: "SHUTDOWN_TIMEOUT",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := load(fakeEnv(tt.env))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestProductionWithoutActiveModulesMayKeepMock(t *testing.T) {
	cfg, err := load(fakeEnv(map[string]string{
		"DATABASE_URL": "x", "APP_ENV": "production", "MIDTRANS_SERVER_KEY": "k",
	}))
	require.NoError(t, err)
	assert.Equal(t, EnvProduction, cfg.Env)
}
