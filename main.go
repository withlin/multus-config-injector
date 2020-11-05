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
	"fmt"
	"log"
	"net/http"
	"os"

	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	sslDir   = "./ssl"
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func podMutatingServe(pod *WebhookServer) error {
	certFile := fmt.Sprintf("%s%s", sslDir, "/tls.crt")
	keyFile := fmt.Sprintf("%s%s", sslDir, "/tls.key")

	log.Println("------>start webhooks,and listen :9443 port and", "certFile dir  is ", certFile, "keyFile dir is", keyFile)
	http.HandleFunc("/multus-cni-config-pods", pod.ServeInjectorMutatePods)
	if err := http.ListenAndServeTLS(":9443", certFile, keyFile, nil); err != nil {
		return err
	}
	return  nil
}

func main() {

	dynamicClient, err := dynamic.NewForConfig(ctrl.GetConfigOrDie())
	if err != nil {
		log.Println(err, "unable to init dynamic client")
		os.Exit(1)
	}

	err =podMutatingServe(&WebhookServer{DynamicClient: dynamicClient})
	if err != nil{
		log.Println(err, "unable to start webhook server")
		os.Exit(1)
	}
}
