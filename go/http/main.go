package main

import (
	"flag"
	"fmt"
	"net/http"

	"github.com/outbrain/golib/log"
)

func main() {
	port := flag.Int("port", 8080, "the port number, defaults to 8080")
	flag.Parse()

	api := new(APIImpl)
	router := ConfigureRoutes(api)
	log.Infof(fmt.Sprintf("Starting server in port %d", *port))
	http.ListenAndServe(fmt.Sprintf(":%d", *port), router)
}
