package throttle

import (
	"fmt"
	"time"

	"github.com/patrickmn/go-cache"

	"github.com/github/freno/pkg/base"
	metrics "github.com/rcrowley/go-metrics"
)

type InventoryType string

var (
	MySQLInventoryType InventoryType = "mysql"
)

type StoreType string

var (
	HAProxyStoreType StoreType = "haproxy"
	VitessStoreType  StoreType = "vitess"
)

func getStoreHealthKey(inventoryType InventoryType, storeType StoreType, storeName, shardName string) string {
	if shardName != "" {
		return fmt.Sprintf("%s/%s/%s/%s", inventoryType, storeType, storeName, shardName)
	}
	return fmt.Sprintf("%s/%s/%s", inventoryType, storeType, storeName)
}

func (throttler *Throttler) getStoreHealth(storeHealthKey string) base.StoreHealth {
	if value, found := throttler.storesHealth.Get(storeHealthKey); found {
		return value.(base.StoreHealth)
	}
	return base.StoreHealth{}
}

func (throttler *Throttler) handleStoreAttempt(inventoryType InventoryType, storeType StoreType, storeName, shardName string) {
	metrics.GetOrRegisterCounter("store.total", nil).Inc(1)
	metrics.GetOrRegisterCounter(fmt.Sprintf("store.%s.total", inventoryType), nil).Inc(1)
	metrics.GetOrRegisterCounter(fmt.Sprintf("store.%s.%s.total", inventoryType, storeType), nil).Inc(1)
	metrics.GetOrRegisterCounter(fmt.Sprintf("store.%s.%s.%s.total", inventoryType, storeType, storeName), nil).Inc(1)
	if shardName != "" {
		metrics.GetOrRegisterCounter(fmt.Sprintf("store.%s.%s.%s.%s.total", inventoryType, storeType, storeName, shardName), nil).Inc(1)
	}
}

func (throttler *Throttler) handleStoreFailure(inventoryType InventoryType, storeType StoreType, storeName, shardName string) {
	metrics.GetOrRegisterCounter("store.error", nil).Inc(1)
	metrics.GetOrRegisterCounter(fmt.Sprintf("store.%s.error", inventoryType), nil).Inc(1)
	metrics.GetOrRegisterCounter(fmt.Sprintf("store.%s.%s.error", inventoryType, storeType), nil).Inc(1)
	metrics.GetOrRegisterCounter(fmt.Sprintf("store.%s.%s.%s.error", inventoryType, storeType, storeName), nil).Inc(1)
	if shardName != "" {
		metrics.GetOrRegisterCounter(fmt.Sprintf("store.%s.%s.%s.%s.error", inventoryType, storeType, storeName, shardName), nil).Inc(1)
	}
}

func (throttler *Throttler) handleStoreHealthy(inventoryType InventoryType, storeType StoreType, storeName, shardName string) {
	storeHealthKey := getStoreHealthKey(inventoryType, storeType, storeName, shardName)
	storeHealth := throttler.getStoreHealth(storeHealthKey)
	storeHealth.LastHealthyAt = time.Now()
	throttler.storesHealth.Set(storeHealthKey, storeHealth, cache.DefaultExpiration)
}

func (throttler *Throttler) updateStoreLatency(inventoryType InventoryType, storeType StoreType, storeName, shardName string, startTime time.Time) {
	latencyMs := time.Since(startTime).Milliseconds()
	metrics.GetOrRegisterCounter("store.latency_ms", nil).Inc(latencyMs)
	metrics.GetOrRegisterCounter(fmt.Sprintf("store.%s.latency_ms", inventoryType), nil).Inc(latencyMs)
	metrics.GetOrRegisterCounter(fmt.Sprintf("store.%s.%s.latency_ms", inventoryType, storeType), nil).Inc(latencyMs)
	metrics.GetOrRegisterCounter(fmt.Sprintf("store.%s.%s.%s.latency_ms", inventoryType, storeType, storeName), nil).Inc(latencyMs)
	if shardName != "" {
		metrics.GetOrRegisterCounter(fmt.Sprintf("store.%s.%s.%s.%s.latency_ms", inventoryType, storeType, storeName, shardName), nil).Inc(latencyMs)
	}
}
