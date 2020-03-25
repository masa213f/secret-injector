package main

import (
	"flag"
	"os"

	"github.com/masa213f/secret-injector/pkg/injector"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	metricsAddr string
	certDir     string
	githubToken string
)

func init() {
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "listen address for metrics")
	flag.StringVar(&certDir, "cert-dir", "/certs", "certificate directory")
	flag.StringVar(&githubToken, "github-token", "", "github token")
	flag.Parse()
}

func main() {
	logf.SetLogger(zap.New(zap.UseDevMode(false)))
	log := logf.Log.WithName("secret-injector")

	setupLog := log.WithName("setup")
	mgr, err := manager.New(config.GetConfigOrDie(), manager.Options{
		MetricsBindAddress: metricsAddr,
		Port:               8443,
		CertDir:            certDir,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	hookServer := mgr.GetWebhookServer()
	hookServer.Register("/secrets/mutate", &admission.Webhook{Handler: injector.New(githubToken, log)})

	setupLog.Info("starting manager")
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
