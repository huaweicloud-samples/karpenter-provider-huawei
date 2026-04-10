package instancetype

import (
	"testing"

	ecsMdl "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/model"
)

func TestDefaultMaxPodsByMemoryMiB(t *testing.T) {
	testCases := []struct {
		name      string
		memoryMiB int64
		want      int64
	}{
		{name: "4Gi", memoryMiB: 4096, want: 20},
		{name: "8Gi", memoryMiB: 8192, want: 40},
		{name: "16Gi", memoryMiB: 16384, want: 60},
		{name: "32Gi", memoryMiB: 32768, want: 80},
		{name: "64Gi", memoryMiB: 65536, want: 110},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := defaultMaxPodsByMemoryMiB(tc.memoryMiB); got != tc.want {
				t.Fatalf("expected max pods %d, got %d", tc.want, got)
			}
		})
	}
}

func TestDefaultMaxPods_CapsBySupplementaryNICs(t *testing.T) {
	testCases := []struct {
		name   string
		flavor ecsMdl.Flavor
		want   int64
	}{
		{
			name: "c9.large.2-like",
			flavor: ecsMdl.Flavor{
				Ram: 4096,
				OsExtraSpecs: &ecsMdl.FlavorExtraSpec{
					QuotasubNetworkInterfaceMaxNum: stringPtr("16"),
				},
			},
			want: 16,
		},
		{
			name: "c9.xlarge.2-like",
			flavor: ecsMdl.Flavor{
				Ram: 8192,
				OsExtraSpecs: &ecsMdl.FlavorExtraSpec{
					QuotasubNetworkInterfaceMaxNum: stringPtr("32"),
				},
			},
			want: 32,
		},
		{
			name: "ac7.2xlarge.1-like",
			flavor: ecsMdl.Flavor{
				Ram: 16384,
				OsExtraSpecs: &ecsMdl.FlavorExtraSpec{
					QuotasubNetworkInterfaceMaxNum: stringPtr("40"),
				},
			},
			want: 40,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := defaultMaxPods(tc.flavor); got != tc.want {
				t.Fatalf("expected max pods %d, got %d", tc.want, got)
			}
		})
	}
}

func TestDefaultMaxPods_FallsBackToMemoryWhenNICCapMissing(t *testing.T) {
	flavor := ecsMdl.Flavor{Ram: 16384}
	if got := defaultMaxPods(flavor); got != 60 {
		t.Fatalf("expected max pods 60, got %d", got)
	}
}
