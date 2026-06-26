package version

import "testing"

func TestValidateK8sVersionRejectsVersionAboveSupportedMaximum(t *testing.T) {
	if err := validateK8sVersion("1.37"); err == nil {
		t.Fatalf("expected kubernetes version %q to be rejected", "1.37")
	}
}

func TestValidateK8sVersionAcceptsSupportedMaximum(t *testing.T) {
	if err := validateK8sVersion("1.36"); err != nil {
		t.Fatalf("expected kubernetes version %q to be accepted, got %v", "1.36", err)
	}
}
