package main

import (
	"flag"
	"os"

	secretinjector "github.com/masa213f/secret-injector/pkg/secret-injector"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	scheme = runtime.NewScheme()
)

func main() {
	var metricsAddr string
	var certDir string
	var githubToken string
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "Listen address for metrics")
	flag.StringVar(&certDir, "cert-dir", "/certs", "certificate directory")
	flag.StringVar(&githubToken, "github-token", "", "github token")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	setupLog := ctrl.Log.WithName("setup")

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

	si := secretinjector.New(ctrl.Log.WithName("secret-injector"))
	si.SetupWithManager(mgr)
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
