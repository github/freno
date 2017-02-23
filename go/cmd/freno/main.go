package main

import (
	"flag"
	"fmt"
	gohttp "net/http"

	"github.com/github/freno/go/config"
	"github.com/github/freno/go/http"
	"github.com/outbrain/golib/log"
)

// AppVersion has to be filled by ldflags:
var AppVersion string

func main() {
	if AppVersion == "" {
		AppVersion = "local-build"
	}

	configFile := flag.String("config", "", "config file name")
	http := flag.Bool("http", false, "spawn the HTTP API server")
	quiet := flag.Bool("quiet", false, "quiet")
	verbose := flag.Bool("verbose", false, "verbose")
	debug := flag.Bool("debug", false, "debug mode (very verbose)")
	stack := flag.Bool("stack", false, "add stack trace upon error")
	help := flag.Bool("help", false, "show the help")
	flag.Parse()

	if *help {
		printHelp()
		return
	}

	log.SetLevel(log.ERROR)
	if *verbose {
		log.SetLevel(log.INFO)
	}
	if *debug {
		log.SetLevel(log.DEBUG)
	}
	if *stack {
		log.SetPrintStackTrace(*stack)
	}
	if *quiet {
		// Override!!
		log.SetLevel(log.ERROR)
	}
	log.Infof("starting freno %s", AppVersion)

	loadConfiguration(*configFile)

	switch {
	case *http:
		httpServe()
	default:
		printUsage()
	}
}

func loadConfiguration(configFile string) {
	var err error
	if configFile != "" {
		err = config.Instance().Read(configFile)
	} else {
		err = config.Instance().Read("/etc/freno.conf.json")
	}

	if err != nil {
		log.Fatalf("Error reading configuration, please check the logs. Error was: %s", err.Error())
	}
}

func httpServe() {
	api := http.NewAPIImpl()
	router := http.ConfigureRoutes(api)
	port := config.Settings().ListenPort
	log.Infof(fmt.Sprintf("Starting server in port %d", port))
	gohttp.ListenAndServe(fmt.Sprintf(":%d", port), router)
}

func printHelp() {
	panic("not yet implemented")
}

func printUsage() {
	fmt.Println(`Usage: freno [OPTIONS]

	For more help options use: freno -help.
	`)
}
