package main

import (
	"flag"
	"os"
	"time"

	"github.com/go-logr/logr"
	"github.com/masa213f/secret-injector/pkg/injector"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	scheme          = runtime.NewScheme()
	log             logr.Logger
	metricsAddr     string
	certDir         string
	pollingInterval time.Duration
	githubToken     string
)

func init() {
	ctrl.SetLogger(zap.New(zap.UseDevMode(false)))
	log = ctrl.Log.WithName("secret-injector")
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "listen address for metrics")
	flag.StringVar(&certDir, "cert-dir", "/certs", "certificate directory")
	flag.DurationVar(&pollingInterval, "polling-interval", 30*time.Second, "polling interval to check github")
	flag.StringVar(&githubToken, "github-token", "", "github token")
	flag.Parse()
}

func main() {
	setupLog := log.WithName("setup")
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               8443,
		LeaderElection:     false,
		CertDir:            certDir,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	err = injector.New(pollingInterval, log).SetupWithManager(mgr)
	if err != nil {
		setupLog.Error(err, "unable to set up")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
