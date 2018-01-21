package haproxy

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/patrickmn/go-cache"
)

var csvCache = cache.New(time.Second, time.Second)

var HAProxyEmptyBody error = fmt.Errorf("Haproxy GET error: empty body")
var HAProxyEmptyStatus error = fmt.Errorf("Haproxy CSV parsing error: no lines found")
var HAProxyPartialStatus error = fmt.Errorf("Haproxy CSV parsing error: only got partial file")
var HAProxyMissingPool error = fmt.Errorf("Haproxy CSV parsing: pool not found")

var MaxHTTPGetConcurrency = 2
var httpGetConcurrentcyChan = make(chan bool, MaxHTTPGetConcurrency)

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

// ParseHosts reads HAProxy CSV lines and returns lists of hosts participating in the given pool (backend)
// Returned are all non-disabled hosts in given backend. Thus, a NOLB is skipped; any UP or DOWN hosts are returned.
// Such list indicates the hosts which can be expected to be active, which is then the list freno will probe.
func ParseHosts(csvLines []string, poolName string) (hosts []string, err error) {
	if len(csvLines) < 1 {
		return hosts, HAProxyEmptyStatus
	}
	if len(csvLines) == 1 {
		return hosts, HAProxyPartialStatus
	}
	var tokensMap map[string]int
	poolFound := false
	for i, line := range csvLines {
		if i == 0 {
			tokensMap = parseHeader(csvLines[0])
			continue
		}
		tokens := strings.Split(line, ",")
		if tokens[tokensMap["pxname"]] == poolName {
			poolFound = true
			if host := tokens[tokensMap["svname"]]; host != "BACKEND" && host != "FRONTEND" {
				status := tokens[tokensMap["status"]]
				status = strings.Split(status, " ")[0]
				if status == "UP" || status == "DOWN" {
					hosts = append(hosts, host)
				}
			}
		}
	}
	if !poolFound {
		return hosts, HAProxyMissingPool
	}
	return hosts, nil
}

// ParseCsvHosts reads HAProxy CSV text and returns lists of hosts participating in the given pool (backend).
// See comment for ParseHosts
func ParseCsvHosts(csv string, poolName string) (hosts []string, err error) {
	csvLines := parseLines(csv)
	return ParseHosts(csvLines, poolName)
}

// Read will read HAProxy URI and return with the CSV text
func Read(host string, port int) (csv string, err error) {
	httpGetConcurrentcyChan <- true
	defer func() { <-httpGetConcurrentcyChan }()

	haproxyUrl := fmt.Sprintf("http://%s:%d/;csv;norefresh", host, port)

	if cachedCSV, found := csvCache.Get(haproxyUrl); found {
		return cachedCSV.(string), nil
	}

	resp, err := http.Get(haproxyUrl)
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
