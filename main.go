/*
Copyright 2020 withlin.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	webhookv1 "github.com/withlin/multus-config-injector/api/v1"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
	sslDir   = "./ssl"
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func podMutatingServe(pod *webhookv1.WebhookServer) {
	certFile := fmt.Sprintf("%s%s", sslDir, "/tls.crt")
	keyFile := fmt.Sprintf("%s%s", sslDir, "/tls.key")

	pod.Log.Info("start webhooks", "certFile", certFile, "keyFile", keyFile)

	http.HandleFunc("/injector-mutating-pods", pod.ServeInjectorMutatePods)

	if err := http.ListenAndServeTLS(":1443", certFile, keyFile, nil); err != nil {
		panic(err)
	}
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8081", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "e57d07f4.yce.nip.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	dynamicClient, err := dynamic.NewForConfig(ctrl.GetConfigOrDie())
	if err != nil {
		setupLog.Error(err, "unable to init dynamic client")
		os.Exit(1)
	}

	// +kubebuilder:scaffold:builder
	go podMutatingServe(&webhookv1.WebhookServer{DynamicClient: dynamicClient, Log: ctrl.Log.WithName("webhook").WithName("pod webhook")})

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
