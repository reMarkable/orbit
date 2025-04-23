package envconfig

import (
	"encoding/base64"
	"reflect"
	"testing"
	"time"
)

func TestSetBool(t *testing.T) {
	var v bool
	rv := reflect.ValueOf(&v).Elem()

	err := setBool(rv, "true")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != true {
		t.Errorf("expected true, got %v", v)
	}
}

func TestSetDuration(t *testing.T) {
	var v time.Duration
	rv := reflect.ValueOf(&v).Elem()

	err := setDuration(rv, "5s")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 5*time.Second {
		t.Errorf("expected 5s, got %v", v)
	}
}

func TestSetInt(t *testing.T) {
	var v int
	rv := reflect.ValueOf(&v).Elem()

	err := setInt(rv, "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 42 {
		t.Errorf("expected 42, got %v", v)
	}
}

func TestSetUint(t *testing.T) {
	var v uint
	rv := reflect.ValueOf(&v).Elem()

	err := setUint(rv, "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 42 {
		t.Errorf("expected 42, got %v", v)
	}
}

func TestSetFloat(t *testing.T) {
	var v float64
	rv := reflect.ValueOf(&v).Elem()

	err := setFloat(rv, "3.14")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 3.14 {
		t.Errorf("expected 3.14, got %v", v)
	}
}

func TestSetBytes(t *testing.T) {
	var v []byte
	rv := reflect.ValueOf(&v).Elem()

	data := base64.StdEncoding.EncodeToString([]byte("hello"))
	err := setBytes(rv, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(v) != "hello" {
		t.Errorf("expected 'hello', got %s", string(v))
	}
}
