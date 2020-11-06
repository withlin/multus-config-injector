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
	"github.com/withlin/multus-config-injector/webhook"
	"log"
	"net/http"
	"os"

	"k8s.io/client-go/dynamic"
	ctrl "sigs.k8s.io/controller-runtime"
	// +kubebuilder:scaffold:imports
)

var (
	sslDir   = "./ssl"
)


func podMutatingServe(pod *webhook.MultusWebhook) error {
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

	err =podMutatingServe(&webhook.MultusWebhook{DynamicClient: dynamicClient})
	if err != nil{
		log.Println(err, "unable to start webhook server")
		os.Exit(1)
	}
}
