package install

import "testing"

func TestValidateDailyAt(t *testing.T) {
	valid := []string{"00:00", "02:00", "02:30", "13:45", "23:59"}
	for _, s := range valid {
		if err := ValidateDailyAt(s); err != nil {
			t.Errorf("ValidateDailyAt(%q) = %v, want nil", s, err)
		}
	}

	invalid := []string{"", "24:00", "2am", "02:60", "-1:00", "02", "02:00:00"}
	for _, s := range invalid {
		if err := ValidateDailyAt(s); err == nil {
			t.Errorf("ValidateDailyAt(%q) = nil, want an error", s)
		}
	}
}
