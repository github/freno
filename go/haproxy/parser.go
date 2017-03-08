package haproxy

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

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

func parseLines(csv string) []string {
	return strings.Split(csv, "\n")
}

func ParseHosts(csvLines []string, poolName string) (hosts []string, err error) {
	if len(csvLines) < 1 {
		return hosts, fmt.Errorf("No lines found haproxy CSV; expecting at least a header")
	}
	var tokensMap map[string]int
	for i, line := range csvLines {
		if i == 0 {
			tokensMap = parseHeader(csvLines[0])
			continue
		}
		tokens := strings.Split(line, ",")
		if tokens[tokensMap["pxname"]] == poolName {
			if host := tokens[tokensMap["svname"]]; host != "BACKEND" && host != "FRONTEND" {
				if status := tokens[tokensMap["status"]]; status == "UP" || status == "DOWN" {
					hosts = append(hosts, host)
				}
			}
		}
	}
	return hosts, nil
}

func ParseCsvHosts(csv string, poolName string) (hosts []string, err error) {
	csvLines := parseLines(csv)
	return ParseHosts(csvLines, poolName)
}

func Read(host string, port int) (csv string, err error) {
	haproxyUrl := fmt.Sprintf("http://%s:%d/;csv;norefresh", host, port)
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
	return string(body), nil
}
