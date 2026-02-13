package throttle

import (
"net/http"
"testing"

"github.com/github/freno/pkg/base"
"github.com/github/freno/pkg/mysql"

test "github.com/outbrain/golib/tests"
)

func TestHttpStatusExclusion(t *testing.T) {
clusterName := "c0"
k1 := mysql.GetClusterInstanceKey(clusterName, &key1)
k2 := mysql.GetClusterInstanceKey(clusterName, &key2)
k3 := mysql.GetClusterInstanceKey(clusterName, &key3)
k4 := mysql.GetClusterInstanceKey(clusterName, &key4)

instanceResults := mysql.InstanceMetricResultMap{
k1: base.NewSimpleMetricResult(1.2),
k2: base.NewSimpleMetricResult(1.7),
k3: base.NewSimpleMetricResult(0.3),
k4: base.NewSimpleMetricResult(2.5),
}

var probeList mysql.Probes = map[mysql.InstanceKey](*mysql.Probe){}
for ck := range instanceResults {
probeList[ck.Key] = &mysql.Probe{Key: ck.Key}
}

// Test: 500 status should exclude host with worst lag
httpStatuses := mysql.ClusterInstanceHttpCheckResultMap{
mysql.MySQLHttpCheckHashKey(clusterName, &key1): http.StatusOK,
mysql.MySQLHttpCheckHashKey(clusterName, &key2): http.StatusOK,
mysql.MySQLHttpCheckHashKey(clusterName, &key3): http.StatusOK,
mysql.MySQLHttpCheckHashKey(clusterName, &key4): http.StatusInternalServerError,
}

result := aggregateMySQLProbes(&probeList, clusterName, instanceResults, httpStatuses, 0, false, 0)
lagValue, err := result.Get()
test.S(t).ExpectNil(err)
test.S(t).ExpectEquals(lagValue, 1.7)

// Test: 503 status should also exclude
httpStatuses[mysql.MySQLHttpCheckHashKey(clusterName, &key2)] = http.StatusServiceUnavailable
result = aggregateMySQLProbes(&probeList, clusterName, instanceResults, httpStatuses, 0, false, 0)
lagValue, err = result.Get()
test.S(t).ExpectNil(err)
test.S(t).ExpectEquals(lagValue, 1.2)
}
