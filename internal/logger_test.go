package internal

import (
	"testing"
)

func TestNewLogger(t *testing.T) {
	logger := NewLogger("info")
	if logger == nil {
		t.Error("NewLogger returned nil")
	}
}
