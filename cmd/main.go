// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"time"

	"github.com/kcp-dev/multicluster-provider/apiexport"
	"golang.org/x/sync/errgroup"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"

	controlplanev1alpha1 "github.com/gardener/cluster-api-provider-gardener/api"
	controllercluster "github.com/gardener/cluster-api-provider-gardener/internal/controller/cluster"
	controlplanecontroller "github.com/gardener/cluster-api-provider-gardener/internal/controller/controlplane"
	infrastructurecontroller "github.com/gardener/cluster-api-provider-gardener/internal/controller/infrastructure"
	"github.com/gardener/cluster-api-provider-gardener/internal/util"
	webhookcontrolplanev1alpha1 "github.com/gardener/cluster-api-provider-gardener/internal/webhook/controlplane/v1alpha1"
	webhookinfrastructurev1alpha1 "github.com/gardener/cluster-api-provider-gardener/internal/webhook/infrastructure/v1alpha1"
)

const (
	apiExportName = "controlplane.cluster.x-k8s.io"
)

var (
	setupLog = ctrl.Log.WithName("setup")
)

// nolint:gocyclo
func main() {
	var (
		metricsAddr                                      string
		metricsCertPath, metricsCertName, metricsCertKey string
		webhookCertPath, webhookCertName, webhookCertKey string
		enableLeaderElection                             bool
		probeAddr                                        string
		secureMetrics                                    bool
		enableHTTP2                                      bool
		gardenerKubeConfigPath                           string
		tlsOpts                                          []func(*tls.Config)
		syncPeriod                                       time.Duration
	)
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.StringVar(&webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate.")
	flag.StringVar(&webhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file.")
	flag.StringVar(&webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file.")
	flag.StringVar(&metricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate.")
	flag.StringVar(&metricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file.")
	flag.StringVar(&metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	flag.StringVar(&gardenerKubeConfigPath, "gardener-kubeconfig", "", "Path to the Gardener kube-config")
	flag.DurationVar(&syncPeriod, "sync-period", time.Minute*10,
		"The minimum interval at which watched resources are reconciled (e.g. 15m)")
	ctrl.RegisterFlags(flag.CommandLine)
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgrContext := ctrl.SetupSignalHandler()

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	// Create watchers for metrics and webhooks certificates
	var metricsCertWatcher, webhookCertWatcher *certwatcher.CertWatcher

	// Initial webhook TLS options
	webhookTLSOpts := tlsOpts

	if len(webhookCertPath) > 0 {
		setupLog.Info("Initializing webhook certificate watcher using provided certificates",
			"webhook-cert-path", webhookCertPath, "webhook-cert-name", webhookCertName, "webhook-cert-key", webhookCertKey)

		var err error
		webhookCertWatcher, err = certwatcher.New(
			filepath.Join(webhookCertPath, webhookCertName),
			filepath.Join(webhookCertPath, webhookCertKey),
		)
		if err != nil {
			setupLog.Error(err, "Failed to initialize webhook certificate watcher")
			os.Exit(1)
		}

		webhookTLSOpts = append(webhookTLSOpts, func(config *tls.Config) {
			config.GetCertificate = webhookCertWatcher.GetCertificate
		})
	}

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: webhookTLSOpts,
	})

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/metrics/server
	// - https://book.kubebuilder.io/reference/metrics.html
	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	// If the certificate is not specified, controller-runtime will automatically
	// generate self-signed certificates for the metrics server. While convenient for development and testing,
	// this setup is not recommended for production.
	//
	// TODO(user): If you enable certManager, uncomment the following lines:
	// - [METRICS-WITH-CERTS] at config/default/kustomization.yaml to generate and use certificates
	// managed by cert-manager for the metrics server.
	// - [PROMETHEUS-WITH-CERTS] at config/prometheus/kustomization.yaml for TLS certification.
	if len(metricsCertPath) > 0 {
		setupLog.Info("Initializing metrics certificate watcher using provided certificates",
			"metrics-cert-path", metricsCertPath, "metrics-cert-name", metricsCertName, "metrics-cert-key", metricsCertKey)

		var err error
		metricsCertWatcher, err = certwatcher.New(
			filepath.Join(metricsCertPath, metricsCertName),
			filepath.Join(metricsCertPath, metricsCertKey),
		)
		if err != nil {
			setupLog.Error(err, "to initialize metrics certificate watcher", "error", err)
			os.Exit(1)
		}

		metricsServerOptions.TLSOpts = append(metricsServerOptions.TLSOpts, func(config *tls.Config) {
			config.GetCertificate = metricsCertWatcher.GetCertificate
		})
	}

	var (
		err   error
		isKcp bool
	)

	ctrlOptions := ctrl.Options{
		Scheme:                 controlplanev1alpha1.Scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "7ceb2ea3.cluster.x-k8s.io",
		BaseContext: func() context.Context {
			return mgrContext
		},
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
		Cache: cache.Options{
			SyncPeriod: &syncPeriod,
		},
	}
	restConfig := ctrl.GetConfigOrDie()

	if isKcp, err = util.HasKcpAPIGroups(restConfig); err != nil {
		setupLog.Error(err, "to determine if kcp API Group is present")
	}
	if isKcp {
		setupLog.Info("Found KCP APIs, looking up virtual workspace URL")
		restConfig, err = util.RestConfigForLogicalClusterHostingAPIExport(mgrContext, restConfig, apiExportName)
		if err != nil {
			setupLog.Error(err, "looking up virtual workspace URL")
			os.Exit(1)
		}
	}

	var provider util.ProviderWithRun
	if isKcp {
		provider, err = apiexport.New(restConfig, apiexport.Options{
			Scheme: controlplanev1alpha1.Scheme,
		})
		if err != nil {
			setupLog.Error(err, "unable to create kcp Provider")
			os.Exit(1)
		}
	}

	mgr, err := mcmanager.New(restConfig, provider, ctrlOptions)
	if err != nil {
		setupLog.Error(err, "unable to create manager")
		os.Exit(1)
	}

	if !isKcp {
		cl, err := mgr.GetCluster(mgrContext, "")
		if err != nil {
			setupLog.Error(err, "unable to get cluster")
			os.Exit(1)
		}
		provider = util.NewSingleClusterProviderWithRun(cl)
	}

	// Create client from kubeconfig
	gardenRestConfig, err := clientcmd.BuildConfigFromFlags("", gardenerKubeConfigPath)
	if err != nil {
		setupLog.Error(err, "unable to build Gardener rest config")
		os.Exit(1)
	}

	gardenMgr, err := mcmanager.New(gardenRestConfig, nil, manager.Options{
		Logger:                  setupLog,
		Scheme:                  controlplanev1alpha1.Scheme,
		GracefulShutdownTimeout: ptr.To(5 * time.Second),
		Cache: cache.Options{
			SyncPeriod: &syncPeriod,
		},
		BaseContext: func() context.Context {
			return mgrContext
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to build Gardener rest config")
		os.Exit(1)
	}

	if err = mgr.Add(gardenMgr); err != nil {
		setupLog.Error(err, "unable to add Gardener manager to manager", "controller", "GardenerShootControlPlane")
		os.Exit(1)
	}

	localGardenManager := gardenMgr.GetLocalManager()
	localManager := mgr.GetLocalManager()

	// Create reconcilers
	if isKcp {
		setupLog.Info("Setting up Cluster reconciler, because KCP API Group is present")
		if err = (&controllercluster.ClusterController{
			Manager: mgr,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Cluster")
			os.Exit(1)
		}

		setupLog.Info("Setting up MachinePool reconciler, because KCP API Group is present")
		if err = (&controllercluster.MachinePoolController{
			Manager: mgr,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "MachinePool")
			os.Exit(1)
		}
	}

	if err = (&controlplanecontroller.GardenerShootControlPlaneReconciler{
		Manager:        mgr,
		GardenerClient: localGardenManager.GetClient(),
		IsKCP:          isKcp,
	}).SetupWithManager(mgr, localGardenManager); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GardenerShootControlPlane")
		os.Exit(1)
	}
	if err = (&controlplanecontroller.GardenerShootControlPlaneReconciler{
		Manager:         mgr,
		GardenerClient:  localGardenManager.GetClient(),
		IsKCP:           isKcp,
		PrioritizeShoot: true,
	}).SetupWithManager(mgr, localGardenManager); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GardenerShootControlPlane (prioritized Shoot)")
		os.Exit(1)
	}

	if err = (&infrastructurecontroller.GardenerShootClusterReconciler{
		Manager:        mgr,
		GardenerClient: localGardenManager.GetClient(),
		IsKCP:          isKcp,
	}).SetupWithManager(mgr, localGardenManager); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GardenerShootCluster")
		os.Exit(1)
	}
	if err = (&infrastructurecontroller.GardenerShootClusterReconciler{
		Manager:         mgr,
		GardenerClient:  localGardenManager.GetClient(),
		IsKCP:           isKcp,
		PrioritizeShoot: true,
	}).SetupWithManager(mgr, localGardenManager); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GardenerShootCluster (prioritized Shoot)")
		os.Exit(1)
	}
	// nolint:goconst
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err = webhookcontrolplanev1alpha1.
			SetupGardenerShootControlPlaneWebhookWithManager(localManager, localGardenManager.GetClient()); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "GardenerShootControlPlane")
			os.Exit(1)
		}
	} else {
		setupLog.Info("Skipping webhook setup")
	}
	// nolint:goconst
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err = webhookinfrastructurev1alpha1.
			SetupGardenerShootClusterWebhookWithManager(localManager, localGardenManager.GetClient()); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "GardenerShootCluster")
			os.Exit(1)
		}
	}

	if err = (&infrastructurecontroller.GardenerWorkerPoolReconciler{
		Manager:        mgr,
		GardenerClient: localGardenManager.GetClient(),
		Scheme:         localManager.GetScheme(),
		IsKCP:          isKcp,
	}).SetupWithManager(mgr, localGardenManager); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GardenerWorkerPool")
		os.Exit(1)
	}
	if err = (&infrastructurecontroller.GardenerWorkerPoolReconciler{
		Manager:         mgr,
		GardenerClient:  localGardenManager.GetClient(),
		Scheme:          localManager.GetScheme(),
		IsKCP:           isKcp,
		PrioritizeShoot: true,
	}).SetupWithManager(mgr, localGardenManager); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GardenerWorkerPool (prioritized Shoot)")
		os.Exit(1)
	}
	// nolint:goconst
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err = webhookinfrastructurev1alpha1.
			SetupGardenerWorkerPoolWebhookWithManager(localManager, localGardenManager.GetClient()); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "GardenerWorkerPool")
			os.Exit(1)
		}
	}

	// +kubebuilder:scaffold:builder

	if metricsCertWatcher != nil {
		setupLog.Info("Adding metrics certificate watcher to manager")
		if err := localManager.Add(metricsCertWatcher); err != nil {
			setupLog.Error(err, "unable to add metrics certificate watcher to manager")
			os.Exit(1)
		}
	}

	if webhookCertWatcher != nil {
		setupLog.Info("Adding webhook certificate watcher to manager")
		if err := localManager.Add(webhookCertWatcher); err != nil {
			setupLog.Error(err, "unable to add webhook certificate watcher to manager")
			os.Exit(1)
		}
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	g, ctx := errgroup.WithContext(mgrContext)
	setupLog.Info("starting provider")
	g.Go(func() error {
		return ignoreCanceled(provider.Run(ctx, mgr))
	})
	setupLog.Info("starting multicluster manager")
	g.Go(func() error {
		return ignoreCanceled(mgr.Start(ctx))
	})
	if err := g.Wait(); err != nil {
		setupLog.Error(err, "unable to start")
		os.Exit(1)
	}
}

func ignoreCanceled(err error) error {
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
