package app

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/IrishBruse/mkw-dwc/internal/backend"
	"github.com/IrishBruse/mkw-dwc/internal/config"
	"github.com/IrishBruse/mkw-dwc/internal/database"
	dbjson "github.com/IrishBruse/mkw-dwc/internal/database/json"
	"github.com/IrishBruse/mkw-dwc/internal/gamespy"
	"github.com/IrishBruse/mkw-dwc/internal/gamespy/browser"
	"github.com/IrishBruse/mkw-dwc/internal/gamespy/gpsp"
	"github.com/IrishBruse/mkw-dwc/internal/gamespy/natneg"
	"github.com/IrishBruse/mkw-dwc/internal/gamespy/profile"
	"github.com/IrishBruse/mkw-dwc/internal/gamespy/qr"
	"github.com/IrishBruse/mkw-dwc/internal/httpfix"
	"github.com/IrishBruse/mkw-dwc/internal/logging"
	"github.com/IrishBruse/mkw-dwc/internal/nas"
	"github.com/IrishBruse/mkw-dwc/internal/proxy"
)

func Run() {
	cfgPath := flag.String("config", "mkw-dwc.ini", "path to mkw-dwc.ini")
	proxyBind := flag.String("proxy-bind", "", "optional HTTP reverse proxy bind address (e.g. :80)")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	logCfg, err := cfg.LoggingSettings()
	if err != nil {
		fmt.Fprintf(os.Stderr, "logging config: %v\n", err)
		os.Exit(1)
	}
	if err := logging.Init(logCfg); err != nil {
		fmt.Fprintf(os.Stderr, "logging init: %v\n", err)
		os.Exit(1)
	}
	if err := httpfix.SetDumpFile(logCfg.DumpFile); err != nil {
		fmt.Fprintf(os.Stderr, "dump file: %v\n", err)
		os.Exit(1)
	}

	log := logging.For("app")

	storeCfg, err := cfg.Store()
	if err != nil {
		log.Fatalf("store config: %v", err)
	}

	gpcm, err := openStore(storeCfg.Type, storeCfg.Path)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer gpcm.Close()

	if err := gpcm.Initialize(); err != nil {
		log.Fatalf("initialize store: %v", err)
	}

	be := backend.New()

	keys := gamespy.SecretKeys()

	nasHost, nasPort, err := cfg.BindAddr("NasServer")
	if err != nil {
		log.Fatalf("nas bind: %v", err)
	}

	profileHost, profilePort, err := cfg.BindAddr("GameSpyProfileServer")
	if err != nil {
		log.Fatalf("profile bind: %v", err)
	}

	gpspHost, gpspPort, err := cfg.BindAddr("GameSpyPlayerSearchServer")
	if err != nil {
		log.Fatalf("gpsp bind: %v", err)
	}

	qrHost, qrPort, err := cfg.BindAddr("GameSpyQRServer")
	if err != nil {
		log.Fatalf("qr bind: %v", err)
	}

	browserHost, browserPort, err := cfg.BindAddr("GameSpyServerBrowserServer")
	if err != nil {
		log.Fatalf("browser bind: %v", err)
	}

	natnegHost, natnegPort, err := cfg.BindAddr("GameSpyNatNegServer")
	if err != nil {
		log.Fatalf("natneg bind: %v", err)
	}

	log.Infof("store: type=%s path=%s", storeCfg.Type, storeCfg.Path)
	log.Infof("logging: level=%s color=%s timestamps=%t log_file=%q dump_file=%q", logCfg.Level, logCfg.Color, logCfg.Timestamps, logCfg.LogFile, logCfg.DumpFile)
	log.Infof("nas: %s", formatListenAddr(nasHost, nasPort))
	log.Infof("profile: %s", formatListenAddr(profileHost, profilePort))
	log.Infof("gpsp: %s", formatListenAddr(gpspHost, gpspPort))
	log.Infof("qr: %s", formatListenAddr(qrHost, qrPort))
	log.Infof("browser: %s", formatListenAddr(browserHost, browserPort))
	log.Infof("natneg: %s", formatListenAddr(natnegHost, natnegPort))
	if *proxyBind != "" {
		log.Infof("proxy: %s", *proxyBind)
	}

	qrServer := qr.New(formatListenAddr(qrHost, qrPort), be, keys)
	qrServer.Profiles = gpcm
	browserServer := browser.New(formatListenAddr(browserHost, browserPort), be, keys, qrServer)

	nasServer := &nas.Server{
		DB:      gpcm,
		SvcHost: cfg.NasSvcHost(),
		Addr:    formatListenAddr(nasHost, nasPort),
	}
	profileServer := &profile.Server{
		DB:   gpcm,
		Addr: formatListenAddr(profileHost, profilePort),
	}
	gpspServer := &gpsp.Server{
		DB:   gpcm,
		Addr: formatListenAddr(gpspHost, gpspPort),
	}
	natnegServer := natneg.New(formatListenAddr(natnegHost, natnegPort), be)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	type service struct {
		name string
		run  func(context.Context) error
	}

	services := []service{
		{name: "nas", run: nasServer.Serve},
		{name: "profile", run: profileServer.Serve},
		{name: "gpsp", run: gpspServer.Serve},
		{name: "qr", run: qrServer.Serve},
		{name: "browser", run: browserServer.Serve},
		{name: "natneg", run: natnegServer.Serve},
	}

	if *proxyBind != "" {
		nasBackend := fmt.Sprintf("http://127.0.0.1:%d", nasPort)
		bind := *proxyBind
		services = append(services, service{
			name: "proxy",
			run: func(ctx context.Context) error {
				return proxy.Serve(ctx, bind, nasBackend)
			},
		})
	}

	errCh := make(chan error, len(services))
	for _, svc := range services {
		svc := svc
		go func() {
			log.Infof("starting %s", svc.name)
			if err := svc.run(ctx); err != nil {
				log.Errorf("%s stopped: %v", svc.name, err)
				errCh <- err
				return
			}
			log.Infof("%s stopped", svc.name)
			errCh <- nil
		}()
	}

	var firstErr error
	for range services {
		if err := <-errCh; err != nil && firstErr == nil {
			firstErr = err
			stop()
		}
	}
	if firstErr != nil {
		log.Fatalf("server error: %v", firstErr)
	}
}

func openStore(kind, path string) (database.Store, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "json":
		return dbjson.Open(path)
	default:
		return nil, fmt.Errorf("unknown store %q (want json)", kind)
	}
}

func formatListenAddr(host string, port int) string {
	if host == "" || host == "0.0.0.0" {
		return fmt.Sprintf(":%d", port)
	}
	return net.JoinHostPort(host, fmt.Sprintf("%d", port))
}
