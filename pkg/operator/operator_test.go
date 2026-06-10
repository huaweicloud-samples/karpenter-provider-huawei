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
	"strings"
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

func TestBillingRegionUsesSDKEnvOverride(t *testing.T) {
	t.Setenv("HUAWEICLOUD_SDK_REGION_BSS_CN_NORTH_4", "https://bss.custom.example.com")

	region := billingRegion("cn-north-4")
	if region == nil {
		t.Fatalf("expected billing region to be constructed")
	}
	if len(region.Endpoints) != 1 || region.Endpoints[0] != "https://bss.custom.example.com" {
		t.Fatalf("expected endpoints [%q], got %v", "https://bss.custom.example.com", region.Endpoints)
	}
}

func TestBillingRegionUsesConfiguredBSSEndpoint(t *testing.T) {
	t.Setenv(BillingEndpointEnv, "https://bss-intl.myhuaweicloud.com")
	t.Setenv("HUAWEICLOUD_SDK_REGION_BSS_CN_NORTH_4", "https://bss.custom.example.com")

	region := billingRegion("cn-north-4")
	if region == nil {
		t.Fatalf("expected billing region to be constructed")
	}
	if region.Id != "cn-north-4" {
		t.Fatalf("expected region id %q, got %q", "cn-north-4", region.Id)
	}
	if len(region.Endpoints) != 1 || region.Endpoints[0] != "https://bss-intl.myhuaweicloud.com" {
		t.Fatalf("expected endpoints [%q], got %v", "https://bss-intl.myhuaweicloud.com", region.Endpoints)
	}
}

func TestBillingRegionIgnoresBlankConfiguredBSSEndpoint(t *testing.T) {
	t.Setenv(BillingEndpointEnv, " \t ")
	t.Setenv("HUAWEICLOUD_SDK_REGION_BSS_CN_NORTH_4", "https://bss.custom.example.com")

	region := billingRegion("cn-north-4")
	if region == nil {
		t.Fatalf("expected billing region to be constructed")
	}
	if len(region.Endpoints) != 1 || region.Endpoints[0] != "https://bss.custom.example.com" {
		t.Fatalf("expected endpoints [%q], got %v", "https://bss.custom.example.com", region.Endpoints)
	}
}

func TestSDKHTTPConfigUsesConfiguredIgnoreSSLVerification(t *testing.T) {
	t.Setenv(IgnoreSSLEnv, "true")

	httpConfig, err := sdkHTTPConfig()
	if err != nil {
		t.Fatalf("expected http config to be created, got %v", err)
	}
	if httpConfig == nil || !httpConfig.IgnoreSSLVerification {
		t.Fatalf("expected ignore ssl verification to be true, got %#v", httpConfig)
	}
}

func TestSDKHTTPConfigRejectsInvalidIgnoreSSLVerification(t *testing.T) {
	t.Setenv(IgnoreSSLEnv, "definitely-not-a-bool")

	_, err := sdkHTTPConfig()
	if err == nil {
		t.Fatalf("expected invalid bool value to return an error")
	}
	if !strings.Contains(err.Error(), IgnoreSSLEnv) {
		t.Fatalf("expected error to mention %q, got %v", IgnoreSSLEnv, err)
	}
}
