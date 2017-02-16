package main

import (
	"flag"
	"fmt"
	gohttp "net/http"

	"github.com/github/freno/go/http"
	"github.com/outbrain/golib/log"
)

// To be filled by ldflags:
var AppVersion string

func main() {
	if AppVersion == "" {
		AppVersion = "local-build"
	}
	log.Infof("starting freno %s", AppVersion)

	server := flag.String("server", "", "spawn the HTTP API server")
	port := flag.Int("port", 8080, "the port number, defaults to 8080")
	flag.Parse()

	if *server != "" {
		mainServer(*port)
	}

}

func mainServer(port int) {
	api := http.NewAPIImpl()
	router := http.ConfigureRoutes(api)
	log.Infof(fmt.Sprintf("Starting server in port %d", port))
	gohttp.ListenAndServe(fmt.Sprintf(":%d", port), router)
}
