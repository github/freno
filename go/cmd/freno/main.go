package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	gohttp "net/http"
	"strings"

	"github.com/github/freno/go/config"
	"github.com/github/freno/go/group"
	"github.com/github/freno/go/http"

	"github.com/github/freno/go/throttle"
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

	forceLeadership := flag.Bool("force-leadership", false, "Make this node consider itself a leader no matter what consensus logic says")

	quiet := flag.Bool("quiet", false, "quiet")
	verbose := flag.Bool("verbose", false, "verbose")
	debug := flag.Bool("debug", false, "debug mode (very verbose)")
	stack := flag.Bool("stack", false, "add stack trace upon error")

	help := flag.Bool("help", false, "show the help")
	flag.Parse()
	group.ForceLeadership = *forceLeadership

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
	throttler := throttle.NewThrottler()
	log.Infof("Starting consensus service")
	log.Infof("- forced leadership: %+v", group.ForceLeadership)
	consensusServiceProvider, err := group.NewConsensusServiceProvider(throttler)
	if err != nil {
		return err
	}
	throttler.SetLeaderFunc(consensusServiceProvider.GetConsensusService().IsLeader)
	throttler.SetSharedDomainServicesFunc(consensusServiceProvider.GetConsensusService().GetSharedDomainServices)

	go consensusServiceProvider.Monitor()
	go throttler.Operate()

	throttlerCheck := throttle.NewThrottlerCheck(throttler)
	throttlerCheck.SelfChecks()
	api := http.NewAPIImpl(throttlerCheck, consensusServiceProvider.GetConsensusService())
	router := http.ConfigureRoutes(api)
	port := config.Settings().ListenPort

	if config.Settings().UseSSL {
		log.Infof("Starting HTTPS server on port %d", port)

		keyFile := config.Settings().SSLPrivateKeyFile
		cert := config.Settings().SSLCertFile
		log.Infof("Using SSLCertFile: %s", cert)
		log.Infof("Using SSLPrivateKeyFile: %s", keyFile)

		tlsConfig := NewTLSConfig(config.Settings())
		srv := &gohttp.Server{
			Addr:         fmt.Sprintf(":%d", port),
			Handler:      router,
			TLSConfig:    tlsConfig,
			TLSNextProto: make(map[string]func(*gohttp.Server, *tls.Conn, gohttp.Handler), 0),
		}
		if err = srv.ListenAndServeTLS(cert, keyFile); err != nil {
			log.Fatale(err)
		}
	} else {
		log.Infof("Starting HTTP server on port %d", port)
		err := gohttp.ListenAndServe(fmt.Sprintf(":%d", port), router)
		if err != nil {
			log.Fatale(err)
		}
	}
	return nil
}

// NewTLSConfig returns an initialized TLS configuration
func NewTLSConfig(conf *config.ConfigurationSettings) *tls.Config {
	return &tls.Config{
		MinVersion:               tls.VersionTLS12,
		CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
			tls.TLS_RSA_WITH_AES_128_CBC_SHA,
			tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		},
		InsecureSkipVerify: conf.SSLSkipVerify,
	}
}

func printHelp() {
	panic("not yet implemented")
}

func printUsage() {
	fmt.Println(`Usage: freno [OPTIONS]
	To run the freno service, execute:
		freno --http

	For more help options use: freno -help.

	freno is a free and open source software.
	  Please see https://github.com/github/freno/blob/master/README.md#license for license.
	  Sources and binaries are found on https://github.com/github/freno/releases.
	  Sources are also available by cloning https://github.com/github/freno.

  Issues can be sumbitted on https://github.com/github/freno/issues
	Please see https://github.com/github/freno/blob/master/README.md#contributing for contributions

	Authored by GitHub engineering
	`)
}
