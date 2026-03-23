/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package operator

import (
	"testing"
)

func TestBillingRegionUsesBusinessRegionIDWithBSSGlobalEndpoint(t *testing.T) {
	region := billingRegion("cn-north-4")
	if region == nil {
		t.Fatalf("expected billing region to be constructed")
	}
	if region.Id != "cn-north-4" {
		t.Fatalf("expected region id %q, got %q", "cn-north-4", region.Id)
	}
	if len(region.Endpoints) != 1 || region.Endpoints[0] != BillingEndpoint {
		t.Fatalf("expected endpoints [%q], got %v", BillingEndpoint, region.Endpoints)
	}
}
