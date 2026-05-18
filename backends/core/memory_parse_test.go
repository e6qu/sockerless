package core

import "testing"

func TestParseMemoryMiB(t *testing.T) {
	cases := []struct {
		in   string
		want int
		err  bool
	}{
		{"512Mi", 512, false},
		{"1Gi", 1024, false},
		{"2Gi", 2048, false},
		{"4Gi", 4096, false},
		{"1024", 1024, false},
		{"1024M", 1024, false},
		{"2G", 2048, false},
		{"", 0, true},
		{"abc", 0, true},
		{"-5Gi", 0, true},
		{"0Mi", 0, true},
	}
	for _, c := range cases {
		got, err := ParseMemoryMiB(c.in)
		if c.err {
			if err == nil {
				t.Errorf("ParseMemoryMiB(%q) = %d, want error", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseMemoryMiB(%q): unexpected error %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("ParseMemoryMiB(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}
