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

package utils

import (
	"fmt"
	"time"

	gocache "github.com/patrickmn/go-cache"

	sdk "github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/huawei"
)

type OfferingAvailabilityCache struct {
	cache *gocache.Cache
	ttl   time.Duration
}

func NewOfferingAvailabilityCache(ttl, cleanupInterval time.Duration) *OfferingAvailabilityCache {
	return &OfferingAvailabilityCache{
		cache: gocache.New(ttl, cleanupInterval),
		ttl:   ttl,
	}
}

func (c *OfferingAvailabilityCache) MarkUnavailable(capacityType string, instanceType sdk.InstanceType, zone string) {
	if c == nil || c.cache == nil {
		return
	}
	c.cache.Set(offeringAvailabilityKey(capacityType, instanceType, zone), struct{}{}, c.ttl)
}

func (c *OfferingAvailabilityCache) IsUnavailable(capacityType string, instanceType sdk.InstanceType, zone string) bool {
	if c == nil || c.cache == nil {
		return false
	}
	_, ok := c.cache.Get(offeringAvailabilityKey(capacityType, instanceType, zone))
	return ok
}

func (c *OfferingAvailabilityCache) TTL() time.Duration {
	if c == nil {
		return 0
	}
	return c.ttl
}

func offeringAvailabilityKey(capacityType string, instanceType sdk.InstanceType, zone string) string {
	return fmt.Sprintf("%s/%s/%s", capacityType, instanceType, zone)
}
