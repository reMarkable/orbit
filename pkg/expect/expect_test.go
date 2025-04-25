package expect

import (
	"testing"
)

func TestValue(t *testing.T) {
	v := Value(42)
	if v.exp != 42 {
		t.Errorf("expected exp to be 42, got %v", v.exp)
	}
}

func TestGot(t *testing.T) {
	v := Value("hello")
	v.Got("world")
	if v.got != "world" {
		t.Errorf("expected got to be 'world', got %v", v.got)
	}
}

func TestValidate_Success(t *testing.T) {
	v := Value(100)
	v.Got(100)
	v.Validate(t, "test-value")
}

func TestValidate_Failure(t *testing.T) {
	v := Value(100)
	v.Got(200)

	// Use a subtest to isolate the failure case
	t.Run("failure case", func(t *testing.T) {
		// Use a helper to capture test output
		helper := &testing.T{}
		v.Validate(helper, "test-value")

		if !helper.Failed() {
			t.Errorf("expected Validate to fail, but it did not")
		}
	})
}
