package main

import "testing"

func TestCountProseUnits(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"你好", 2},
		{"Hello World", 2},
		{"你好Hello", 3},
		{"3.14", 1},
		{"3,141", 1},
		{"U.S.A", 1},
		{"U.S.A.", 1},
		{"A#123", 1},
		{"0734-v4", 1},
		{"ＡＢＣ", 1},
		{"价格是3.14元", 5},
		{"他说。Hello", 3},
		{"", 0},
		{"   ", 0},
		{"。，！？", 0},
		{"a", 1},
		{"#123", 1},
		{".5", 1},
	}
	for _, tt := range tests {
		got := countProseUnits(tt.in)
		if got != tt.want {
			t.Errorf("countProseUnits(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}
