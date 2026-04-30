package instancetype

import (
	"testing"

	ecsMdl "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/apis/v1alpha1"
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

func TestNewInstanceType_UsesCCEMemoryReservationModel(t *testing.T) {
	flavor := ecsMdl.Flavor{
		Name:  "c6.large.2",
		Ram:   8192,
		Vcpus: "2",
	}

	containerd := NewInstanceType(flavor, "cn-north-4", nil, nil, "containerd", nil, nil, nil, nil, nil, nil)
	assertQuantityEqual(t, containerd.Overhead.SystemReserved[corev1.ResourceMemory], "600Mi")
	assertQuantityEqual(t, containerd.Overhead.KubeReserved[corev1.ResourceMemory], "700Mi")
	assertQuantityEqual(t, containerd.Overhead.EvictionThreshold[corev1.ResourceMemory], "100Mi")
	assertQuantityEqual(t, containerd.Allocatable()[corev1.ResourceMemory], "6792Mi")

	docker := NewInstanceType(flavor, "cn-north-4", nil, nil, "docker", nil, nil, nil, nil, nil, nil)
	assertQuantityEqual(t, docker.Overhead.SystemReserved[corev1.ResourceMemory], "600Mi")
	assertQuantityEqual(t, docker.Overhead.KubeReserved[corev1.ResourceMemory], "1300Mi")
	assertQuantityEqual(t, docker.Allocatable()[corev1.ResourceMemory], "6192Mi")
}

func TestNewInstanceType_KubeReservedUsesMemoryTierDefaultPodCount(t *testing.T) {
	flavor := ecsMdl.Flavor{
		Name:  "c9.large.2",
		Ram:   4096,
		Vcpus: "2",
		OsExtraSpecs: &ecsMdl.FlavorExtraSpec{
			QuotasubNetworkInterfaceMaxNum: stringPtr("16"),
		},
	}

	containerd := NewInstanceType(flavor, "cn-north-4", nil, nil, "containerd", nil, nil, nil, nil, nil, nil)
	assertQuantityEqual(t, containerd.Capacity[corev1.ResourcePods], "16")
	assertQuantityEqual(t, containerd.Overhead.KubeReserved[corev1.ResourceMemory], "600Mi")
	assertQuantityEqual(t, containerd.Allocatable()[corev1.ResourceMemory], "2896Mi")

	docker := NewInstanceType(flavor, "cn-north-4", nil, nil, "docker", nil, nil, nil, nil, nil, nil)
	assertQuantityEqual(t, docker.Capacity[corev1.ResourcePods], "16")
	assertQuantityEqual(t, docker.Overhead.KubeReserved[corev1.ResourceMemory], "900Mi")
	assertQuantityEqual(t, docker.Allocatable()[corev1.ResourceMemory], "2596Mi")
}

func TestDefaultResolverCacheKeyIncludesRuntimeType(t *testing.T) {
	resolver := NewDefaultResolver("cn-north-4")

	defaulted := fakeNodeClass{}
	explicitContainerd := fakeNodeClass{
		runtime: &v1alpha1.RuntimeConfiguration{Type: "containerd"},
	}
	docker := fakeNodeClass{
		runtime: &v1alpha1.RuntimeConfiguration{Type: "docker"},
	}

	if got, want := resolver.CacheKey(defaulted), resolver.CacheKey(explicitContainerd); got != want {
		t.Fatalf("expected default runtime cache key %q to match explicit containerd %q", got, want)
	}
	if resolver.CacheKey(explicitContainerd) == resolver.CacheKey(docker) {
		t.Fatalf("expected runtime type to affect cache key")
	}
}

func TestNormalizedRuntimeType(t *testing.T) {
	testCases := []struct {
		name  string
		input *v1alpha1.RuntimeConfiguration
		want  string
	}{
		{name: "nil defaults to containerd", input: nil, want: "containerd"},
		{name: "empty defaults to containerd", input: &v1alpha1.RuntimeConfiguration{}, want: "containerd"},
		{name: "docker", input: &v1alpha1.RuntimeConfiguration{Type: "docker"}, want: "docker"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizedRuntimeType(tc.input); got != tc.want {
				t.Fatalf("expected runtime type %q, got %q", tc.want, got)
			}
		})
	}
}

func assertQuantityEqual(t *testing.T, got resource.Quantity, want string) {
	t.Helper()
	expected := resource.MustParse(want)
	if got.Cmp(expected) != 0 {
		t.Fatalf("expected quantity %s, got %s", expected.String(), got.String())
	}
}
