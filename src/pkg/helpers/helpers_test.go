package helpers

import "testing"

func TestPercentUsed(t *testing.T) {
	expectations := []struct {
		free, total uint64
		used        float64
	}{
		{20, 100, 80},
		{0, 99, 100},
		{11, 11, 0},
	}

	for _, e := range expectations {
		used := PercentUsed(e.free, e.total)
		if used != e.used {
			t.Errorf("Expected %f, got: %f", e.used, used)
		}
	}
}
