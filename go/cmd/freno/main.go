package main

import (
	"flag"
	"fmt"
	gohttp "net/http"
	"strings"

	"github.com/github/freno/go/config"
	"github.com/github/freno/go/group"
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

	// The next group of variables override configuration file settings. This allows for easy local testing.
	// In the general deployment case they will not be used.
	httpPort := flag.Int("http-port", 0, "HTTP listen port; overrides config's ListenPort")
	raftDataDir := flag.String("raft-datadir", "", "Data directory for raft backend db; overrides config's RaftDataDir")
	raftBind := flag.String("raft-bind", "", "Raft bind address (example: '127.0.0.1:10008'). Overrides config's RaftBind")
	raftNodes := flag.String("raft-nodes", "", "Comma separated (e.g. 'host:port[,host:port]') list of raft nodes. Overrides config's RaftNodes")

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

	// Potentialy override config
	if *raftDataDir != "" {
		config.Settings().RaftDataDir = *raftDataDir
	}
	if *raftBind != "" {
		config.Settings().RaftBind = *raftBind
	}
	if *raftNodes != "" {
		config.Settings().RaftNodes = strings.Split(*raftNodes, ",")
	}
	if *httpPort > 0 {
		config.Settings().ListenPort = *httpPort
	}

	switch {
	case *http:
		err := httpServe()
		log.Errore(err)
	case *help:
		printHelp()
	default:
		printUsage()
	}
}

func loadConfiguration(configFile string) {
	var err error
	if configFile != "" {
		err = config.Instance().Read(configFile)
	} else {
		err = config.Instance().Read("/etc/freno.conf.json", "conf/freno.conf.json")
	}

	if err != nil {
		log.Fatalf("Error reading configuration, please check the logs. Error was: %s", err.Error())
	}
}

func httpServe() error {
	log.Infof("Starting concensus service")
	if err := group.Setup(); err != nil {
		return err
	}
	go group.Monitor()

	api := http.NewAPIImpl()
	router := http.ConfigureRoutes(api)
	port := config.Settings().ListenPort
	log.Infof("Starting server in port %d", port)
	return gohttp.ListenAndServe(fmt.Sprintf(":%d", port), router)
}

func printHelp() {
	panic("not yet implemented")
}

func printUsage() {
	fmt.Println(`Usage: freno [OPTIONS]

	For more help options use: freno -help.
	`)
}
