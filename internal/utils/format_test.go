package utils

import "testing"

func TestParseDurationRejectsInvalidOrOverflowingTimes(t *testing.T) {
	for _, input := range []string{"-1", "1:-2", "1:60", "1:60:00", "9223372036854775807", "9223372036854775807:00", "2562047788016:00:00"} {
		if got, err := ParseDuration(input); err == nil {
			t.Errorf("ParseDuration(%q) = %d; expected error", input, got)
		}
	}
}

func TestParseDurationAcceptsValidTimes(t *testing.T) {
	for input, want := range map[string]int64{"90": 90000, "1:30": 90000, "90:00": 5400000, "1:30:00": 5400000, "0": 0} {
		got, err := ParseDuration(input)
		if err != nil || got != want {
			t.Errorf("ParseDuration(%q) = %d, %v; want %d", input, got, err, want)
		}
	}
}

func TestTruncatePreservesUnicode(t *testing.T) {
	for _, tc := range []struct {
		s    string
		max  int
		want string
	}{{"éééééé", 5, "éé..."}, {"🎵🎵🎵", 2, "🎵🎵"}, {"abc", -1, ""}} {
		t.Run(tc.s, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Truncate panicked: %v", r)
				}
			}()
			if got := Truncate(tc.s, tc.max); got != tc.want {
				t.Errorf("Truncate = %q; want %q", got, tc.want)
			}
		})
	}
}
