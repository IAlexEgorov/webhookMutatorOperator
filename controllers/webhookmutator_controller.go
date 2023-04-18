/*
Copyright 2023 Alex Egorov.

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

package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	apiv1alpha1 "github.com/IAlexEgorov/webhook-v1/api/v1alpha1"
	"github.com/go-logr/logr"
	v14 "k8s.io/api/admissionregistration/v1"
	v1 "k8s.io/api/apps/v1"
	v12 "k8s.io/api/core/v1"
	v13 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"time"
)

var logger = log.Log.WithName("controller_scaler")

// WebhookMutatorReconciler reconciles a WebhookMutator object
type WebhookMutatorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

type WebhookResources struct {
	Deployment                   *v1.Deployment
	Service                      *v12.Service
	ServiceAccount               *v12.ServiceAccount
	MutatingWebhookConfiguration *v14.MutatingWebhookConfiguration
	ClusterRole                  *v13.ClusterRole
	ClusterRoleBinding           *v13.ClusterRoleBinding
	Secret                       *v12.Secret
	ConfigMap                    *v12.ConfigMap
}

//+kubebuilder:rbac:groups=api.ialexegorov.neoflex.ru,resources=webhookmutators,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=api.ialexegorov.neoflex.ru,resources=webhookmutators/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=api.ialexegorov.neoflex.ru,resources=webhookmutators/finalizers,verbs=update

func (r *WebhookMutatorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	operatorLogger := logger
	operatorLogger.Info("Reconcile called")

	webhookMutator := &apiv1alpha1.WebhookMutator{}
	err := r.Get(ctx, req.NamespacedName, webhookMutator)
	if err != nil {
		operatorLogger.Info(fmt.Sprintf("YYYY cyka!!!"))
		return ctrl.Result{}, nil
	}

	factory := serializer.NewCodecFactory(r.Scheme)
	decoder := factory.UniversalDeserializer()

	webhookResources := WebhookResources{}
	webhookResources.declareConfigMap(decoder, operatorLogger)
	webhookResources.declareSecret(decoder, operatorLogger)

	webhookResources.declareServiceAccount(decoder, operatorLogger)
	webhookResources.declareClusterRole(decoder, operatorLogger)
	webhookResources.declareClusterRoleBinding(decoder, operatorLogger)

	webhookResources.declareDeployment(decoder, operatorLogger)
	webhookResources.declareService(decoder, operatorLogger)

	webhookResources.declareMutatingWebhookConfiguration(decoder, operatorLogger)

	return ctrl.Result{RequeueAfter: time.Duration(30 * time.Second)}, nil
}

func compactValue(v string) string {
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(v)); err != nil {
		panic("Hard coded json strings broken!")
	}
	return compact.String()
}

func (w *WebhookResources) declareConfigMap(decoder runtime.Decoder, operatorLogger logr.Logger) {

	input := fmt.Sprintf(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: webhook-config
data:
  config.yaml: |
    general:
      port: 8443
      tlsCertFile: /etc/webhook/certs/tls.crt
      tlsKeyFile: /etc/webhook/certs/tls.key
      logLevel: debug
    triggerLabel:
      notebook-name: "*"
    patchData:
      labels:
        type-app: "notebook"
      annotations:
        sidecar.istio.io/componentLogLevel: "wasm:debug"
        sidecar.istio.io/userVolume: "[{\"name\":\"wasmfilters-dir\",\"emptyDir\": { } } ]"
        sidecar.istio.io/userVolumeMount: "[{\"mountPath\":\"/var/local/lib/wasm-filters\",\"name\":\"wasmfilters-dir\"}]"
        `)
	obj, _, err := decoder.Decode([]byte(input), nil, nil)
	if err != nil {
		operatorLogger.Error(err, err.Error())
	}

	w.ConfigMap = obj.(*v12.ConfigMap)
	operatorLogger.Info("ConfigMap has added")
}
func (w *WebhookResources) declareDeployment(decoder runtime.Decoder, operatorLogger logr.Logger) {
	input := fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: aegorov-admission-webhook
  namespace: default
  labels:
    app: aegorov-admission-webhook
spec:
  replicas: 1
  selector:
    matchLabels:
      app: aegorov-admission-webhook
  template:
    metadata:
      labels:
        app: aegorov-admission-webhook
    spec:
      nodeSelector:
        kubernetes.io/os: linux
      serviceAccountName: aegorov-admission-webhook
      securityContext:
        runAsNonRoot: true
        runAsUser: 1234
      containers:
      - name: server
        image: "89109249948/webhook:config-version-label-bug-resticted-v1"
        imagePullPolicy: IfNotPresent
        args: ["--config-file", "/etc/webhook/config.yaml"]
        ports:
        - containerPort: 8443
          name: tls
        - containerPort: 80
          name: metrics
        volumeMounts:
        - name: webhook-tls-certs
          mountPath: /etc/webhook/certs/
          readOnly: true
        - name: config-volume
          mountPath: /etc/webhook/
      volumes:
      - name: webhook-tls-certs
        secret:
          secretName: aegorov-admission-tls
      - name: config-volume
        configMap:
          name: webhook-config`)

	obj, _, err := decoder.Decode([]byte(input), nil, nil)
	if err != nil {
		operatorLogger.Error(err, err.Error())
	}

	w.Deployment = obj.(*v1.Deployment)
	operatorLogger.Info("Deployment has added")
}
func (w *WebhookResources) declareService(decoder runtime.Decoder, operatorLogger logr.Logger) {
	input := fmt.Sprintf(`
apiVersion: v1
kind: Service
metadata:
  name: aegorov-admission
  namespace: default
spec:
  selector:
    app: aegorov-admission-webhook
  ports:
    - port: 443
      targetPort: tls
      name: application
    - port: 80
      targetPort: metrics
      name: metrics`)

	obj, _, err := decoder.Decode([]byte(input), nil, nil)
	if err != nil {
		operatorLogger.Error(err, err.Error())
	}

	w.Service = obj.(*v12.Service)
	operatorLogger.Info("Service has added")
}
func (w *WebhookResources) declareServiceAccount(decoder runtime.Decoder, operatorLogger logr.Logger) {
	input := fmt.Sprintf(`
apiVersion: v1
kind: ServiceAccount
metadata:
  name: aegorov-admission-webhook
  namespace: default`)

	obj, _, err := decoder.Decode([]byte(input), nil, nil)
	if err != nil {
		operatorLogger.Error(err, err.Error())
	}

	w.ServiceAccount = obj.(*v12.ServiceAccount)
	operatorLogger.Info("ServiceAccount has added")
}
func (w *WebhookResources) declareClusterRole(decoder runtime.Decoder, operatorLogger logr.Logger) {
	input := fmt.Sprintf(`
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: aegorov-admission-webhook
rules:
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "watch", "list"]`)

	obj, _, err := decoder.Decode([]byte(input), nil, nil)
	if err != nil {
		operatorLogger.Error(err, err.Error())
	}

	w.ClusterRole = obj.(*v13.ClusterRole)
	operatorLogger.Info("ClusterRole has added")
}
func (w *WebhookResources) declareClusterRoleBinding(decoder runtime.Decoder, operatorLogger logr.Logger) {
	input := fmt.Sprintf(`
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: aegorov-admission-webhook
subjects:
- kind: ServiceAccount
  name: aegorov-admission-webhook
  namespace: default
roleRef:
  kind: ClusterRole
  name: aegorov-admission-webhook
  apiGroup: rbac.authorization.k8s.io`)

	obj, _, err := decoder.Decode([]byte(input), nil, nil)
	if err != nil {
		operatorLogger.Error(err, err.Error())
	}

	w.ClusterRoleBinding = obj.(*v13.ClusterRoleBinding)
	operatorLogger.Info("ClusterRoleBinding has added")
}
func (w *WebhookResources) declareSecret(decoder runtime.Decoder, operatorLogger logr.Logger) {
	input := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: aegorov-admission-tls
type: Opaque
data:
  tls.crt: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURiakNDQWxhZ0F3SUJBZ0lVWHVMYW1NSms5bEhORjZEOGpudkZxZHMxNDhvd0RRWUpLb1pJaHZjTkFRRUwKQlFBd1R6RUxNQWtHQTFVRUJoTUNVbFV4RURBT0JnTlZCQWdUQjBWNFlXMXdiR1V4RHpBTkJnTlZCQWNUQmsxdgpjMk52ZHpFUU1BNEdBMVVFQ2hNSFJYaGhiWEJzWlRFTE1Ba0dBMVVFQ3hNQ1EwRXdIaGNOTWpNd05ERTNNVEF5Ck56QXdXaGNOTWpnd05ERTFNVEF5TnpBd1dqQlBNUXN3Q1FZRFZRUUdFd0pTVlRFUU1BNEdBMVVFQ0JNSFJYaGgKYlhCc1pURVBNQTBHQTFVRUJ4TUdUVzl6WTI5M01SQXdEZ1lEVlFRS0V3ZEZlR0Z0Y0d4bE1Rc3dDUVlEVlFRTApFd0pEUVRDQ0FTSXdEUVlKS29aSWh2Y05BUUVCQlFBRGdnRVBBRENDQVFvQ2dnRUJBTWlGd2dlUGFrdC91dy9RCjl1N21pWmpndG9nWXl3U0xZcTN6c0I0RFQyK0twUEVpbTFVVFRCVHZzVWNnYTk5cUt5bTFxZXF3WWJSa3RIZHUKZGwzOHZTSEY0K0lOYmFpem1mY1hrSTFVV3I4dmFHaHVCc0lRd2lWdm9UODFialFSTk1MbWNma2dYM09BakJQSgo5U2UwSnpjbGY0dUVEd2R4R0xzdnJieElKWGk1UmxZVjZwekFiUUF0UE5pYWc0aExDaFpqY1FmRW1Cc1oyMjBkCncxMDZzeFVmRWplYjZoRWVvYnhjTHdzcTlGY00ySGJXMm8xYmtDY3ZuSm4ySEYzNXRlSmlDbHFBRWczaFpQOEYKOFdQKzNXREVHUVV2eFdWSFJtaEZqb0RNRFdDV1QzWkZZaVQvZU12c1l6c3duM2tpS2dON29jWnhXNXJvN3ZFQQpEeDI4K3ljQ0F3RUFBYU5DTUVBd0RnWURWUjBQQVFIL0JBUURBZ0VHTUE4R0ExVWRFd0VCL3dRRk1BTUJBZjh3CkhRWURWUjBPQkJZRUZQdDUzT0s3R0FMd3RidlVWNXorNXlvN2ZTU1lNQTBHQ1NxR1NJYjNEUUVCQ3dVQUE0SUIKQVFBWnhwcEs0eXhSVW1sckM5Q3BDR3M0VzR5LzNmTUtCVTRPT2Y3Q1R3VlIzQVc2bEdUbXYvTitCM01Wd3d1OQpESjJsYmhiTUhpSGdnMUdSd1IvN0t5T2FmeFQ0YkNPUG9NUjBZRU1Db0J6R3pRNHIvRUp1aStyRk03RGY1Mnh2ClJSOHprTnJIcXk5KzB1b1JackltRnNqYWNKOFBhUEdsZmV0eWFISEVGTDFNSXZkeGtEZ0xGYVBlcTZBaVJhT3AKd0VZenVhUnRsRDNUbVVSSXN1Yk9tN3Bpc1lITUN6NFdUeERzd2QzVU9OUGxpQlU2ZkVsZzZkNVFlMDVyWGc4YQpTMURnQThFKzRJZHkralZ0dk9YcmJTa3d0RDRYY2w3RWppQXk5TVY5TGlnelNCdmFReXlvb3FGR3FMYU43WDV3CkNtZVlxLzNHNUpVb2wzR3J5bENkekNDUQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0t
  tls.key: LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpNSUlFcEFJQkFBS0NBUUVBdERHdURnWmN0ditlTzdLN3o2anVmbTNuaEo1MWFkdHNQclFKS3VTS1JyU0xkVmdtCmNycEl5clpuVU9UVS85UHZKVkZldWJZc2ZyUHlodzZ6NERrWi9zdVZRaXEydEhHWTcvMGFJcnRpZnYrK3VRengKOHhTemNMSmZaR29nZ0FSeWY1ci9zTFV3bDVKWlF1c214dkFUSXh6WmR4dVBWR0tSUE1kdHhGY0NCSUhSN2gvTgpVU0tJZ1lsbWRmRGpGd04yOUxTQW5wME9MbWdSaFZHRkhPaTRsVkFKclh3VDk4RDdwV0JXWVpER1NZdWI5c0p3CmdhRjNiK0VVekNVN0Uwb25tb0ptK2dIeGpCYlM0blNWUXZsWG1hcHIzS2JFOFowUE9QZlU1QnNYSlp4U2UycGsKVlFmeUU3NjhkZ29BT0M0bUljMnVzcHdvZzd0cDAzTmREYlNpRHdJREFRQUJBb0lCQUMzYjR1Y3cyVkc5ZG1ETgpHUjA5YWcwRkhZSFQ3aC9WdHhPeU1BOFpnTk9EeVdaQTIvQWcxcnU0ZGt6UklDQkhxTG8vbmpMM1dSV1o2R1JVCjZwUnJFK0dUb0ZYcGx2d1BXUlBpdjA4Tmo0Q3d4N0pDeUNUTUpPck9pcFo4cCs3TXN2TWsyR1A1aVBkYVNKdE8KUzEwZjVrN3VYYTRCZHBYTVRCaEN6UDNHQXY1d09sQzg4b01CMkVRa0ZJbEJsaUlBQS92R0hJaVBGeFJLZ1dkOAova0NlRzB3WG5zaVlHTmw2cTRiNGV6aVVHZTF6L0JxTUJKRVBOak1MdzJHM1lnTDE3dVdlTHNJQVNVQ29TcFpzClF4MFY1REpWcnB1ZEpvVUFabnRHWUhJTUVva3BKYmg3eGthbU9sVWlKS0ZxNEZtKy81Zk9GZlJqN3FnR1BWSVYKTDFndW8va0NnWUVBMVJlU3RiWVIvb2dJN25qVzVNWSs1WEtmZFpMTURlWVVWdjA1M0ErR3Y5OUJzTE56cnJONAozeUhzK2VQYUU5MEFOOUhaV3lqQWtsSDZqUGV1S1ZjZ3NZb2J0eHhocGdIOVdDcXhjMktscnNDTVNHMUNqOTNXCmJudWk3QmZwRzAxdkExNkRJU2NoUkdIQU9jV0tkR1RsMmNobkhtUERGVzNNTzdSR01iZ0VOdU1DZ1lFQTJIcEoKVXZtWFp6Mm82dFpnQWdsRmh5YXFDd1ZBd3BjcUFCZDlHb0FFSllhdy84QnlUMHRCNFRZN1FsSWFiTGVobzE5WApSUURuU3BrYXlVRDd6dnArWHo5ZDR1MkcwK202ZzNpUEQ3NkRmemdZMUNPMkEzZlZ2U2VuWmJMSkNVRnRiLzdQCiszTFc1bjY4M2pSdGFGeERnUmNWaHN6Rkd2SGh4b2Rpem1TZm8rVUNnWUJtL0wraEp0L0hmb09pamJCK1hQbmsKOXVMdWRnWTg2V2dIS3RlZDdic1lYSlJwRERIcXl6NnR5TDI1Z0UvVHJjbi9NR0syVmhuTUhlYlQzcGpEemlJMAo3Q1M4K1BDUXhRRm1iU3ZhTW1FVTltWldVc2dLdEJLQXp5eE1vcm04d2szVytRU3pMekE2MW11TEFGZ01MUCtSCm8vT0Nrb0NraUs1ZVpLQlFRemwyTlFLQmdRQ0hpTWl2c3FVZ2RuSnoxWlIyc2VkZUhzOEg1MW9NZXloSXRtd1YKVTJGRlBYZEVLUEZvdysyVFc2anVkUWttV0RKVFh6WDhkZnhac0ZJYy94cXBGQnhhOWdtS01yemZvTTZ0MGFXQQpiZjlXZjRETUVTKzhMQ01lTXQyVHhzUW5qMWM5YjRRNElrWjZPWTkyYjh5d01sUHhWc3FiZzBsRS9Yd21HRTI4CmV6T1lJUUtCZ1FDV0JyMmlxdnNNeDRQRlpNemw2ZXh5MWhRZ1lBZFJjNUJTdGFGRXFBZ2dXMzdxM2w1clhuNW0KTzAvOUpXU1FVUGhpL21jQldrbzRoRVdrd1dBc3FhUjNienp5UEF0Ym9Wd20vamMxamJNM1ZBK3hNYmlWSFhUTgpzeHBoL2dIdHN5SHdjY3ZyeDk0ZDFZTjEvb0xPMTRLY3ZLM2Z4UnBWYW1sek41ZTMyb1pvMmc9PQotLS0tLUVORCBSU0EgUFJJVkFURSBLRVktLS0tLQ==`)

	obj, _, err := decoder.Decode([]byte(input), nil, nil)
	if err != nil {
		operatorLogger.Error(err, err.Error())
	}

	w.Secret = obj.(*v12.Secret)
	operatorLogger.Info("Secret has added")
}
func (w *WebhookResources) declareMutatingWebhookConfiguration(decoder runtime.Decoder, operatorLogger logr.Logger) {
	input := fmt.Sprintf(`
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: aegorov-admission
webhooks:
  - name: aegorov-admission.default.svc
    admissionReviewVersions:
      - "v1beta1"
    sideEffects: "None"
    timeoutSeconds: 30
    objectSelector:
      matchLabels:
        example-webhook-enabled: "true"
    clientConfig:
      service:
        name: aegorov-admission
        namespace: default
        path: "/mutate/deployments"
      caBundle: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURiakNDQWxhZ0F3SUJBZ0lVWHVMYW1NSms5bEhORjZEOGpudkZxZHMxNDhvd0RRWUpLb1pJaHZjTkFRRUwKQlFBd1R6RUxNQWtHQTFVRUJoTUNVbFV4RURBT0JnTlZCQWdUQjBWNFlXMXdiR1V4RHpBTkJnTlZCQWNUQmsxdgpjMk52ZHpFUU1BNEdBMVVFQ2hNSFJYaGhiWEJzWlRFTE1Ba0dBMVVFQ3hNQ1EwRXdIaGNOTWpNd05ERTNNVEF5Ck56QXdXaGNOTWpnd05ERTFNVEF5TnpBd1dqQlBNUXN3Q1FZRFZRUUdFd0pTVlRFUU1BNEdBMVVFQ0JNSFJYaGgKYlhCc1pURVBNQTBHQTFVRUJ4TUdUVzl6WTI5M01SQXdEZ1lEVlFRS0V3ZEZlR0Z0Y0d4bE1Rc3dDUVlEVlFRTApFd0pEUVRDQ0FTSXdEUVlKS29aSWh2Y05BUUVCQlFBRGdnRVBBRENDQVFvQ2dnRUJBTWlGd2dlUGFrdC91dy9RCjl1N21pWmpndG9nWXl3U0xZcTN6c0I0RFQyK0twUEVpbTFVVFRCVHZzVWNnYTk5cUt5bTFxZXF3WWJSa3RIZHUKZGwzOHZTSEY0K0lOYmFpem1mY1hrSTFVV3I4dmFHaHVCc0lRd2lWdm9UODFialFSTk1MbWNma2dYM09BakJQSgo5U2UwSnpjbGY0dUVEd2R4R0xzdnJieElKWGk1UmxZVjZwekFiUUF0UE5pYWc0aExDaFpqY1FmRW1Cc1oyMjBkCncxMDZzeFVmRWplYjZoRWVvYnhjTHdzcTlGY00ySGJXMm8xYmtDY3ZuSm4ySEYzNXRlSmlDbHFBRWczaFpQOEYKOFdQKzNXREVHUVV2eFdWSFJtaEZqb0RNRFdDV1QzWkZZaVQvZU12c1l6c3duM2tpS2dON29jWnhXNXJvN3ZFQQpEeDI4K3ljQ0F3RUFBYU5DTUVBd0RnWURWUjBQQVFIL0JBUURBZ0VHTUE4R0ExVWRFd0VCL3dRRk1BTUJBZjh3CkhRWURWUjBPQkJZRUZQdDUzT0s3R0FMd3RidlVWNXorNXlvN2ZTU1lNQTBHQ1NxR1NJYjNEUUVCQ3dVQUE0SUIKQVFBWnhwcEs0eXhSVW1sckM5Q3BDR3M0VzR5LzNmTUtCVTRPT2Y3Q1R3VlIzQVc2bEdUbXYvTitCM01Wd3d1OQpESjJsYmhiTUhpSGdnMUdSd1IvN0t5T2FmeFQ0YkNPUG9NUjBZRU1Db0J6R3pRNHIvRUp1aStyRk03RGY1Mnh2ClJSOHprTnJIcXk5KzB1b1JackltRnNqYWNKOFBhUEdsZmV0eWFISEVGTDFNSXZkeGtEZ0xGYVBlcTZBaVJhT3AKd0VZenVhUnRsRDNUbVVSSXN1Yk9tN3Bpc1lITUN6NFdUeERzd2QzVU9OUGxpQlU2ZkVsZzZkNVFlMDVyWGc4YQpTMURnQThFKzRJZHkralZ0dk9YcmJTa3d0RDRYY2w3RWppQXk5TVY5TGlnelNCdmFReXlvb3FGR3FMYU43WDV3CkNtZVlxLzNHNUpVb2wzR3J5bENkekNDUQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0t
    rules:
      - operations: [ "CREATE" ]
        apiGroups: [ "apps" ]
        apiVersions: [ "v1" ]
        resources: [ "statefulsets" ]`)

	obj, _, err := decoder.Decode([]byte(input), nil, nil)
	if err != nil {
		operatorLogger.Error(err, err.Error())
	}

	w.MutatingWebhookConfiguration = obj.(*v14.MutatingWebhookConfiguration)
	operatorLogger.Info("MutatingWebhookConfiguration has added")
}

// SetupWithManager sets up the controller with the Manager.
func (r *WebhookMutatorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&apiv1alpha1.WebhookMutator{}).
		Complete(r)
}
