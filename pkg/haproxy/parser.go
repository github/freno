package haproxy

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/github/freno/pkg/base"

	"github.com/patrickmn/go-cache"
)

var csvCache = cache.New(time.Second, time.Second)

var HAProxyEmptyBody error = fmt.Errorf("Haproxy GET error: empty body")
var HAProxyEmptyStatus error = fmt.Errorf("Haproxy CSV parsing error: no lines found")
var HAProxyPartialStatus error = fmt.Errorf("Haproxy CSV parsing error: only got partial file")
var HAProxyMissingPool error = fmt.Errorf("Haproxy CSV parsing: pool not found")
var HAProxyAllUpHostsTransitioning error = fmt.Errorf("Haproxy: all host marked as UP are in transition. HAProxy is likely reloading")
var HAProxyAllHostsTransitioning error = fmt.Errorf("Haproxy: all hosts are in transition. HAProxy is likely reloading")

var MaxHTTPGetConcurrency = 2
var httpGetConcurrencyChan = make(chan bool, MaxHTTPGetConcurrency)

var httpClient *http.Client

func init() {
	httpClient = base.SetupHttpClient(0)
}

// parseHeader parses the HAPRoxy CSV header, which lists column names.
// Returned is a header-to-index map
func parseHeader(header string) (tokensMap map[string]int) {
	tokensMap = map[string]int{}
	header = strings.TrimLeft(header, "#")
	header = strings.TrimSpace(header)
	tokens := strings.Split(header, ",")

	for i, token := range tokens {
		tokensMap[token] = i
	}
	return tokensMap
}

// Simple utility function to split CSV lines
func parseLines(csv string) []string {
	return strings.Split(csv, "\n")
}

func ParseStatus(fullStatus string) (status BackendHostStatus, isTransitioning bool) {
	// status can show up as:
	// `UP`
	// `UP 1/2` (transitioning)
	// `NOLB`
	// `DOWN`
	// `DOWN (agent)`
	// `no check`
	// etc. See https://github.com/haproxy/haproxy/blob/a5de024d42c4113fc6e189ea1d0ba6335219e151/src/dumpstats.c#L4117-L4129
	if ToBackendHostStatus(fullStatus) == StatusNoCheck {
		return StatusNoCheck, isTransitioning
	}

	statusTokens := strings.Split(fullStatus, " ")
	isTransitioning = (len(statusTokens) > 1 && strings.Contains(statusTokens[1], "/"))
	return ToBackendHostStatus(statusTokens[0]), isTransitioning
}

// ParseHosts reads HAProxy CSV lines and returns lists of hosts participating in the given pool (backend)
// Returned are all non-disabled hosts in given backend. Thus, a NOLB is skipped; any UP or DOWN hosts are returned.
// Such list indicates the hosts which can be expected to be active, which is then the list freno will probe.
func ParseHosts(csvLines []string, poolName string) (backendHosts [](*BackendHost), err error) {
	if len(csvLines) < 1 {
		return backendHosts, HAProxyEmptyStatus
	}
	if len(csvLines) == 1 {
		return backendHosts, HAProxyPartialStatus
	}
	var tokensMap map[string]int
	poolFound := false
	countHosts := 0
	countUpHosts := 0
	countTransitioningHosts := 0
	countTransitioningUpHosts := 0
	for i, line := range csvLines {
		if i == 0 {
			tokensMap = parseHeader(csvLines[0])
			continue
		}
		tokens := strings.Split(line, ",")
		if tokens[tokensMap["pxname"]] == poolName {
			poolFound = true
			if host := tokens[tokensMap["svname"]]; host != "BACKEND" && host != "FRONTEND" {
				countHosts++

				status, isTransitioning := ParseStatus(tokens[tokensMap["status"]])

				backendHosts = append(backendHosts, NewBackendHost(host, status, isTransitioning))
				if isTransitioning {
					countTransitioningHosts++
				}
				if status == StatusUp {
					countUpHosts++
					if isTransitioning {
						countTransitioningUpHosts++
					}
				}
			}
		}
	}
	if !poolFound {
		return backendHosts, HAProxyMissingPool
	}
	if countTransitioningHosts == countHosts && countHosts > 0 {
		return backendHosts, HAProxyAllHostsTransitioning
	}
	if countTransitioningUpHosts == countUpHosts && countUpHosts > 0 {
		return backendHosts, HAProxyAllUpHostsTransitioning
	}
	return backendHosts, nil
}

// ParseCsvHosts reads HAProxy CSV text and returns lists of hosts participating in the given pool (backend).
// See comment for ParseHosts
func ParseCsvHosts(csv string, poolName string) (backendHosts [](*BackendHost), err error) {
	csvLines := parseLines(csv)
	return ParseHosts(csvLines, poolName)
}

func toCSVUrl(u url.URL) *url.URL {
	u.Path = fmt.Sprintf("%s;csv;norefresh", u.Path)
	return &u
}

// Read will read HAProxy URI and return with the CSV text
func Read(u *url.URL) (csv string, err error) {
	httpGetConcurrencyChan <- true
	defer func() { <-httpGetConcurrencyChan }()

	haproxyUrl := toCSVUrl(*u).String()

	if cachedCSV, found := csvCache.Get(haproxyUrl); found {
		return cachedCSV.(string), nil
	}

	resp, err := httpClient.Get(haproxyUrl)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return "", err
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	csv = string(body)
	if csv == "" {
		return "", HAProxyEmptyBody
	}
	csvCache.Set(haproxyUrl, csv, cache.DefaultExpiration)
	return csv, nil
}
