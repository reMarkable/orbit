package envconfig

import (
	"os"
	"testing"
)

type Config struct {
	Port     int    `envconfig:"PORT" default:"8080"`
	Host     string `envconfig:"HOST" required:"true"`
	LogLevel string `envconfig:"LOG_LEVEL" default:"info"`
}

func TestProcess_WithDefaults(t *testing.T) {
	os.Clearenv() // Ensure no environment variables are set

	os.Setenv("HOST", "localhost")
	var cfg Config
	err := Process(&cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != 8080 {
		t.Errorf("expected Port to be 8080, got %d", cfg.Port)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected LogLevel to be 'info', got %s", cfg.LogLevel)
	}
}

func TestProcess_WithEnvironmentVariables(t *testing.T) {
	os.Setenv("PORT", "9090")
	os.Setenv("HOST", "localhost")
	defer os.Clearenv()

	var cfg Config
	err := Process(&cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != 9090 {
		t.Errorf("expected Port to be 9090, got %d", cfg.Port)
	}
	if cfg.Host != "localhost" {
		t.Errorf("expected Host to be 'localhost', got %s", cfg.Host)
	}
}

func TestProcess_MissingRequiredField(t *testing.T) {
	os.Clearenv() // Ensure no environment variables are set

	var cfg Config
	err := Process(&cfg)
	if err == nil {
		t.Fatal("expected an error for missing required field, got nil")
	}

	if err.Error() != "HOST: missing required value" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestProcess_InvalidSpec(t *testing.T) {
	// Test with a non-pointer value
	var cfg Config
	err := Process(cfg)
	if err == nil || err.Error() != "not a pointer: invalid spec" {
		t.Errorf("expected 'not a pointer: invalid spec' error, got %v", err)
	}

	// Test with a non-struct pointer
	var invalid int
	err = Process(&invalid)
	if err == nil || err.Error() != "not a struct: invalid spec" {
		t.Errorf("expected 'not a struct: invalid spec' error, got %v", err)
	}
}

func TestProcess_UnsupportedFieldType(t *testing.T) {
	type UnsupportedConfig struct {
		UnsupportedField complex64 `envconfig:"UNSUPPORTED"`
	}

	os.Clearenv()
	os.Setenv("UNSUPPORTED", "1+2i")

	var cfg UnsupportedConfig
	err := Process(&cfg)
	if err == nil || err.Error() != "UNSUPPORTED: unknown field type" {
		t.Errorf("expected 'unknown field type' error, got %v", err)
	}
}

func TestProcess_DefaultsAndRequired(t *testing.T) {
	type NestedConfig struct {
		InnerField string `envconfig:"INNER_FIELD" default:"default-value"`
	}

	type ConfigWithDefaults struct {
		Nested NestedConfig `envconfig:"NESTED"`
	}

	os.Clearenv()
	var cfg ConfigWithDefaults
	err := Process(&cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Nested.InnerField != "default-value" {
		t.Errorf("expected InnerField to be 'default-value', got %q", cfg.Nested.InnerField)
	}
}

func TestProcess_SliceField(t *testing.T) {
	type SliceConfig struct {
		Values []string `envconfig:"VALUES"`
	}

	os.Clearenv()
	os.Setenv("VALUES", "one,two,three")

	var cfg SliceConfig
	err := Process(&cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"one", "two", "three"}
	if len(cfg.Values) != len(expected) {
		t.Fatalf("expected %d values, got %d", len(expected), len(cfg.Values))
	}
	for i, v := range cfg.Values {
		if v != expected[i] {
			t.Errorf("expected value %q, got %q", expected[i], v)
		}
	}
}

func TestProcess_MapField(t *testing.T) {
	type MapConfig struct {
		Values map[string]string `envconfig:"VALUES"`
	}

	os.Clearenv()
	os.Setenv("VALUES", "key1:value1;key2:value2")

	var cfg MapConfig
	err := Process(&cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := map[string]string{"key1": "value1", "key2": "value2"}
	if len(cfg.Values) != len(expected) {
		t.Fatalf("expected %d map entries, got %d", len(expected), len(cfg.Values))
	}
	for k, v := range expected {
		if cfg.Values[k] != v {
			t.Errorf("expected key %q to have value %q, got %q", k, v, cfg.Values[k])
		}
	}
}
