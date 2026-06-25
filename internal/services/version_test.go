package services

import "testing"

func TestCheckCoreVersion(t *testing.T) {
	tests := []struct {
		name           string
		running        string
		wantCompatible bool
	}{
		{"exact match", LastTestedCoreVersion, true},
		{"same minor different patch", "1.43.99", true},
		{"same minor patch zero", "1.43.0", true},
		{"leading v tolerated", "v1.43.2", true},
		{"prerelease suffix tolerated", "1.43.2-rc1", true},
		{"newer minor flagged", "1.44.0", false},
		{"older minor flagged", "1.42.9", false},
		{"newer major flagged", "2.0.0", false},
		{"unparseable flagged", "unknown", false},
		{"empty flagged", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status := CheckCoreVersion(tc.running)
			if status.Compatible != tc.wantCompatible {
				t.Errorf("CheckCoreVersion(%q).Compatible = %v, want %v (warning: %q)",
					tc.running, status.Compatible, tc.wantCompatible, status.Warning)
			}
			if !status.Compatible && status.Warning == "" {
				t.Errorf("expected a warning when not compatible for %q", tc.running)
			}
		})
	}
}
