package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"gomodules.xyz/jsonpatch/v2"
	"k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	//"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/dynamic"
)



const (
	annotationsMultusCniKey = "k8s.v1.cni.cncf.io/networks"
)

type WebhookServer struct {
	DynamicClient dynamic.Interface
}

//TODO:just like istio,label to namespace: multus-cni-config=enabled,which will injector cni config
var ingoredList = []string{
	"kube-system",
	"default",
	"kube-public",
}

func toAdmissionResponse(err error) *v1beta1.AdmissionResponse {
	return &v1beta1.AdmissionResponse{
		Result: &metav1.Status{
			Message: err.Error(),
		},
	}
}

func ignoredRequired(namespace string) bool {
	// skip special kubernetes system namespaces
	for _, ignoreNamespace := range ingoredList {
		if namespace == ignoreNamespace {
			return false
		}
	}
	return true
}

var (
	podGvr = metav1.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	}

	deserializer = serializer.NewCodecFactory(
		runtime.NewScheme(),
	).UniversalDeserializer()
)

type admitFunc func(v1beta1.AdmissionReview) *v1beta1.AdmissionResponse

//type validateFunc func(ar *v1beta1.AdmissionReview) *v1beta1.AdmissionResponse

// TODO: Only support Create Event,Not Support Update Event.Next version will Support it
func (p *WebhookServer) serve(w http.ResponseWriter, r *http.Request, admit admitFunc) {
	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
	}
	defer r.Body.Close()

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		log.Println(
			fmt.Errorf(
				"context type is non expect error, value: %v", contentType),
			"",
		)
		return
	}

	var reviewResponse *v1beta1.AdmissionResponse
	ar := v1beta1.AdmissionReview{}

	if _, _, err := deserializer.Decode(body, nil, &ar); err != nil {
		reviewResponse = toAdmissionResponse(err)
	} else {
		reviewResponse = admit(ar)
	}

	response := v1beta1.AdmissionReview{}
	if reviewResponse != nil {
		response.Response = reviewResponse
		response.Response.UID = ar.Request.UID
	}

	// must add api version and kind on kube16.2
	response.APIVersion = "admission.k8s.io/v1"
	response.Kind = "AdmissionReview"

	resp, err := json.Marshal(response)
	if err != nil {
		log.Println(err, "")
	}

	if _, err := w.Write(resp); err != nil {
		log.Println(err, "")
	}
}

func (p *WebhookServer) ServeInjectorMutatePods(w http.ResponseWriter, r *http.Request) {
	p.serve(w, r, p.injectorMutatePods)
}



func (p *WebhookServer) injectorMutatePods(ar v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	log.Println("--------------->mutate multus cni  config  request is comming")
	reviewResponse := &v1beta1.AdmissionResponse{}
	reviewResponse.Allowed = true

	if ar.Request.Resource != podGvr {
		err := fmt.Errorf("expect resource to be %s", podGvr)
		log.Println(err, "")
		return toAdmissionResponse(err)
	}

	raw := ar.Request.Object.Raw
	pod := corev1.Pod{}

	if _, _, err := deserializer.Decode(raw, nil, &pod); err != nil {
		log.Println(err, "")
		return toAdmissionResponse(err)
	}

	//TODO: according multus-cni-config-injection=enabled to namespace will injection
	if !ignoredRequired(ar.Request.Namespace) {
		return reviewResponse
	}


	podCopy := pod.DeepCopy()

	// Ignore if exclusion annotation is present
	if podAnnotations := pod.GetAnnotations(); podAnnotations != nil {
		if _, isMirrorPod := podAnnotations[corev1.MirrorPodAnnotationKey]; isMirrorPod {
			return reviewResponse
		}
	}
	result, err := p.lookUpOwnerReference(podCopy.OwnerReferences, ar.Request.Namespace)
	if err != nil {
		return reviewResponse
	}
	if result == nil {
		return reviewResponse
	}

	cniValue, ok := result[annotationsMultusCniKey].(string)
	if !ok {
		log.Println("warn: cni value to string :")
		return reviewResponse
	}

	podCopyAnnos := podCopy.GetAnnotations()
	if podCopyAnnos == nil {
		podCopyAnnos = make(map[string]string)
	}
	podCopyAnnos[annotationsMultusCniKey] = cniValue
	podCopy.Annotations = podCopyAnnos
	log.Println("podCopy Annotations:",podCopy.Annotations)

	// TODO: investigate why GetGenerateName doesn't work
	podCopyJSON, err := json.Marshal(podCopy)
	if err != nil {
		return toAdmissionResponse(err)
	}

	podJSON, err := json.Marshal(pod)
	if err != nil {
		return toAdmissionResponse(err)
	}

	jsonPatch, err := jsonpatch.CreatePatch(podJSON, podCopyJSON)
	if err != nil {
		return toAdmissionResponse(err)
	}

	jsonPatchBytes, _ := json.Marshal(jsonPatch)

	reviewResponse.Patch = jsonPatchBytes
	pt := v1beta1.PatchTypeJSONPatch
	reviewResponse.PatchType = &pt

	return reviewResponse
}

func (p *WebhookServer) lookUpOwnerReference(ownerReferences []metav1.OwnerReference, namespace string) (map[string]interface{}, error) {
	if len(ownerReferences) > 0 {
		for _, or := range ownerReferences {

			apiVersion := strings.Split(or.APIVersion, "/")
			res := schema.GroupVersionResource{Group: apiVersion[0], Version: apiVersion[1], Resource: strings.ToLower(or.Kind) + "s"}
			obj, err := p.DynamicClient.Resource(res).Namespace(namespace).Get(or.Name, metav1.GetOptions{})

			if err != nil {
				return nil, err
			}
			unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
			if err != nil {
				return nil, err
			}

			annos,ok:= unstructuredObj["metadata"].(map[string]interface{})["annotations"].(map[string]interface{})
			if !ok {
				return nil,nil
			}
			cniNetWork := annos[annotationsMultusCniKey]
			if cniNetWork != nil {
				log.Println("find  k8s.v1.cni.cncf.io/networks Annotations:",cniNetWork)
				result := make(map[string]interface{}, 1)
				result[annotationsMultusCniKey] = cniNetWork
				return result, nil
			}

			orMap, ok := unstructuredObj["metadata"].(map[string]interface{})["ownerReferences"].([]interface{})
			if !ok {
				return nil, nil
			}
			if len(orMap) == 0 {
				return nil, err
			}

			var ors []metav1.OwnerReference
			for _, or := range orMap {
				result, ok := or.(map[string]interface{})
				if !ok {
					return nil, nil
				}
				or := metav1.OwnerReference{}
				or.APIVersion = result["apiVersion"].(string)
				or.Kind = result["kind"].(string)
				or.Name = result["name"].(string)
				ors = append(ors, or)
			}

			if len(ors) > 0 {
				p.lookUpOwnerReference(ors, namespace)
			}

		}
	}
	return nil, nil
}
