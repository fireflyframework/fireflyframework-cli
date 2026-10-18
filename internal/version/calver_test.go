package version

import "testing"

func TestMonthlyCalVerAllowsReleaseCountersBeyondCalendarDays(t *testing.T) {
	for _, value := range []string{"26.02.32", "26.10.99", "v26.10.17"} {
		if _, err := Parse(value); err != nil {
			t.Errorf("valid monthly release %q rejected: %v", value, err)
		}
	}
}

func TestMonthlyCalVerRejectsValuesOutsideTheReleaseFormat(t *testing.T) {
	for _, value := range []string{"26.00.17", "26.13.17", "100.10.17", "26.10.100", "-1.10.17", "26.10.-1"} {
		if _, err := Parse(value); err == nil {
			t.Errorf("invalid monthly release %q accepted", value)
		}
	}
}
