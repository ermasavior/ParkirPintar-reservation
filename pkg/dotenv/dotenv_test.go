package dotenv

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetEnv(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue string
		envValue     string
		expected     string
	}{
		{
			name:         "existing env variable",
			key:          "TEST_KEY",
			defaultValue: "default",
			envValue:     "actual",
			expected:     "actual",
		},
		{
			name:         "non-existing env variable",
			key:          "NON_EXISTING_KEY",
			defaultValue: "default",
			envValue:     "",
			expected:     "default",
		},
		{
			name:         "empty env variable",
			key:          "EMPTY_KEY",
			defaultValue: "default",
			envValue:     "",
			expected:     "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				_ = os.Setenv(tt.key, tt.envValue)
				defer func() {
					_ = os.Unsetenv(tt.key)
				}()
			}

			result := GetEnv(tt.key, tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLoadEnv(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		setup    func() string
		teardown func(string)
	}{
		{
			name: "load default .env file",
			args: []string{},
			setup: func() string {
				content := "TEST_VAR=test_value\n"
				_ = os.WriteFile(".env", []byte(content), 0644)
				return ".env"
			},
			teardown: func(file string) {
				_ = os.Remove(file)
			},
		},
		{
			name: "load custom env file",
			args: []string{"custom.env"},
			setup: func() string {
				content := "CUSTOM_VAR=custom_value\n"
				_ = os.WriteFile("custom.env", []byte(content), 0644)
				return "custom.env"
			},
			teardown: func(file string) {
				_ = os.Remove(file)
			},
		},
		{
			name: "file not found",
			args: []string{"nonexistent.env"},
			setup: func() string {
				return ""
			},
			teardown: func(file string) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := tt.setup()
			defer tt.teardown(file)

			LoadEnv(tt.args...)

			if file != "" && file != "nonexistent.env" {
				assert.NotEmpty(t, os.Getenv("TEST_VAR"))
			}
		})
	}
}

func TestLoadEnv_InvalidFile(t *testing.T) {
	LoadEnv("invalid.env")
	// Should not panic, just log error
}

func TestLoadEnv_DefaultLocation(t *testing.T) {
	content := "DEFAULT_VAR=default_value\n"
	_ = os.WriteFile(".env", []byte(content), 0644)
	defer func() {
		_ = os.Remove(".env")
	}()

	LoadEnv()
	// Should load from default .env location
}

func TestLoadEnv_WithMultipleArgs(t *testing.T) {
	content := "MULTI_VAR=multi_value\n"
	_ = os.WriteFile("test.env", []byte(content), 0644)
	defer func() {
		_ = os.Remove("test.env")
	}()

	LoadEnv("test.env", "extra_arg")
	// Should use first arg as location
}

func TestLoadEnv_InvalidContent(t *testing.T) {
	// Create directory instead of file to trigger error
	_ = os.Mkdir(".env_dir", 0755)
	defer func() {
		_ = os.RemoveAll(".env_dir")
	}()

	// Create invalid .env file
	_ = os.WriteFile(".env", []byte("INVALID LINE WITHOUT EQUALS"), 0644)
	defer func() {
		_ = os.Remove(".env")
	}()

	LoadEnv()
	// Should handle invalid content gracefully
}
