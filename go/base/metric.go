package base

import (
	"fmt"
	"strings"
)

func GetMetricName(storeType string, storeName string) (metricName string) {
	return strings.Join([]string{storeType, storeName}, "/")
}

func ParseMetricName(metricName string) (storeType string, storeName string, err error) {
	metricTokens := strings.Split(metricName, "/")
	if len(metricTokens) != 2 {
		return storeType, storeName, fmt.Errorf("Error parsing storename: %s", storeName)
	}
	storeType = metricTokens[0]
	storeName = metricTokens[1]
	return storeType, storeName, nil
}
