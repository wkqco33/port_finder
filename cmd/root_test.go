package cmd

import (
	"testing"
)

func TestParsePortArg(t *testing.T) {
	tests := []struct {
		input     string
		wantStart uint16
		wantEnd   uint16
		wantErr   bool
	}{
		{"8080", 8080, 8080, false},
		{" 8080 ", 8080, 8080, false},
		{"3000-4000", 3000, 4000, false},
		{" 3000 - 4000 ", 3000, 4000, false},
		{"0", 0, 0, true},
		{"70000", 0, 0, true},
		{"3000-2000", 0, 0, true}, // lo > hi
		{"abc", 0, 0, true},
		{"3000-abc", 0, 0, true},
		{"abc-4000", 0, 0, true},
		{"3000-4000-5000", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			start, end, err := ParsePortArg(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParsePortArg(%q) error = %v, wantErr = %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr {
				if start != tt.wantStart || end != tt.wantEnd {
					t.Errorf("ParsePortArg(%q) = (%d, %d), want (%d, %d)", tt.input, start, end, tt.wantStart, tt.wantEnd)
				}
			}
		})
	}
}
