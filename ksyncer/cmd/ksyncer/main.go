package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/ashrafalaodat/smcp/ksyncer/pkg/controllers"
	"github.com/ashrafalaodat/smcp/ksyncer/pkg/registry"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	// Register Knative Service GVK for unstructured client operations.
	servingGVK := schema.GroupVersionKind{Group: "serving.knative.dev", Version: "v1", Kind: "Service"}
	scheme.AddKnownTypeWithName(servingGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(servingGVK.GroupVersion().WithKind("ServiceList"), &unstructured.UnstructuredList{})
}

func main() {
	var (
		metricsAddr          string
		probeAddr            string
		enableLeaderElection bool
		concurrency          int
		registryURL          string
		requeueDelay         time.Duration
	)

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", true, "Enable leader election for controller manager.")
	flag.IntVar(&concurrency, "concurrency", 4, "Number of concurrent reconciles.")
	flag.StringVar(&registryURL, "registry-url", os.Getenv("REGISTRY_URL"), "Base URL of the registry service (env: REGISTRY_URL).")
	flag.DurationVar(&requeueDelay, "requeue-delay", 30*time.Second, "Requeue delay when description is missing.")

	opts := zap.Options{Development: false}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if registryURL == "" {
		log.Fatalf("registry-url is required")
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "ksyncer-controller",
	})
	if err != nil {
		log.Fatalf("unable to start manager: %v", err)
	}

	regClient := registry.NewHTTPClient(registryURL, &http.Client{Timeout: 10 * time.Second})

	reconciler := &controllers.KServiceReconciler{
		Client:       mgr.GetClient(),
		Scheme:       mgr.GetScheme(),
		Recorder:     mgr.GetEventRecorderFor("ksyncer"),
		Registry:     regClient,
		RequeueDelay: requeueDelay,
	}

	if err = reconciler.SetupWithManager(mgr, concurrency); err != nil {
		log.Fatalf("unable to create controller: %v", err)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		log.Fatalf("unable to set up health check: %v", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		log.Fatalf("unable to set up ready check: %v", err)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Fatalf("problem running manager: %v", err)
	}
}
