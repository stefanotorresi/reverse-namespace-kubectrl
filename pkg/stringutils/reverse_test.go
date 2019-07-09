package stringutils

import "testing"

func TestReverse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"hello", "Hello, world", "dlrow ,olleH"},
		{"hello utf8", "Hello, 世界", "界世 ,olleH"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Reverse(tt.input); got != tt.expected {
				t.Errorf("Reverse() = %v, want %v", got, tt.expected)
			}
		})
	}
}
