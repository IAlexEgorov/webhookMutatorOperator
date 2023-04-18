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
	webhookResources.createConfigMap(decoder, operatorLogger)
	//webhookResources.createSecret(decoder, operatorLogger)

	webhookResources.createServiceAccount(decoder, operatorLogger)
	webhookResources.createClusterRole(decoder, operatorLogger)
	webhookResources.createClusterRoleBinding(decoder, operatorLogger)

	webhookResources.createDeployment(decoder, operatorLogger)
	webhookResources.createService(decoder, operatorLogger)

	webhookResources.createMutatingWebhookConfiguration(decoder, operatorLogger)

	return ctrl.Result{RequeueAfter: time.Duration(30 * time.Second)}, nil
}

func compactValue(v string) string {
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(v)); err != nil {
		panic("Hard coded json strings broken!")
	}
	return compact.String()
}

func (w *WebhookResources) createConfigMap(decoder runtime.Decoder, operatorLogger logr.Logger) {

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
func (w *WebhookResources) createDeployment(decoder runtime.Decoder, operatorLogger logr.Logger) {
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
func (w *WebhookResources) createService(decoder runtime.Decoder, operatorLogger logr.Logger) {
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
func (w *WebhookResources) createServiceAccount(decoder runtime.Decoder, operatorLogger logr.Logger) {
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
func (w *WebhookResources) createClusterRole(decoder runtime.Decoder, operatorLogger logr.Logger) {
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
func (w *WebhookResources) createClusterRoleBinding(decoder runtime.Decoder, operatorLogger logr.Logger) {
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
func (w *WebhookResources) createSecret(decoder runtime.Decoder, operatorLogger logr.Logger) {
	input := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: aegorov-admission-tls
type: Opaque
data:
  tls.crt: |
    -----BEGIN CERTIFICATE-----
    MIIENTCCAx2gAwIBAgIUUgD6UPvI3Wbjrf4Ikm4MINyCVCgwDQYJKoZIhvcNAQEL
    BQAwTzELMAkGA1UEBhMCUlUxEDAOBgNVBAgTB0V4YW1wbGUxDzANBgNVBAcTBk1v
    c2NvdzEQMA4GA1UEChMHRXhhbXBsZTELMAkGA1UECxMCQ0EwHhcNMjMwNDE3MTAy
    NzAwWhcNNDMwNDEyMTAyNzAwWjBPMQswCQYDVQQGEwJSVTEQMA4GA1UECBMHRXhh
    bXBsZTEPMA0GA1UEBxMGTW9zY293MRAwDgYDVQQKEwdFeGFtcGxlMQswCQYDVQQL
    EwJDQTCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBALQxrg4GXLb/njuy
    u8+o7n5t54SedWnbbD60CSrkika0i3VYJnK6SMq2Z1Dk1P/T7yVRXrm2LH6z8ocO
    s+A5Gf7LlUIqtrRxmO/9GiK7Yn7/vrkM8fMUs3CyX2RqIIAEcn+a/7C1MJeSWULr
    JsbwEyMc2Xcbj1RikTzHbcRXAgSB0e4fzVEiiIGJZnXw4xcDdvS0gJ6dDi5oEYVR
    hRzouJVQCa18E/fA+6VgVmGQxkmLm/bCcIGhd2/hFMwlOxNKJ5qCZvoB8YwW0uJ0
    lUL5V5mqa9ymxPGdDzj31OQbFyWcUntqZFUH8hO+vHYKADguJiHNrrKcKIO7adNz
    XQ20og8CAwEAAaOCAQcwggEDMA4GA1UdDwEB/wQEAwIFoDAdBgNVHSUEFjAUBggr
    BgEFBQcDAQYIKwYBBQUHAwIwDAYDVR0TAQH/BAIwADAdBgNVHQ4EFgQUa4zr7Y3Q
    Nr4LhjA2rE0LjFTMTNAwgaQGA1UdEQSBnDCBmYIrYWVnb3Jvdi1hZG1pc3Npb24u
    ZGVmYXVsdC5zdmMuY2x1c3Rlci5sb2NhbIIRYWVnb3Jvdi1hZG1pc3Npb26CJ2Fl
    Z29yb3YtYWRtaXNzaW9uLmRlZmF1bHQuY2x1c3Rlci5sb2NhbIIdYWVnb3Jvdi1h
    ZG1pc3Npb24uZGVmYXVsdC5zdmOCCWxvY2FsaG9zdIcEfwAAATANBgkqhkiG9w0B
    AQsFAAOCAQEATzVLyDdUOxgZrgl2d/QP8E+C6KBUen76sw7hsrYgh3xoOZWL7Mea
    IxJvMJ0GgCcjINZzXHRUn4Ai4VPvmOXpsMoeMtkQdy3edIuGu/AtReg75S1vnZsX
    OKUuv0FsgoSYmhx2kLAHzMinLESyeaDFuyPnvBUAY0pKCKMe87RACK7NwWkiJMv3
    5xKdjGfmbD2OMQDhjahfz9ZulPEvruVyu2jtc/cQHAkQ4BKwGygWe0IPH5a3UMnY
    GyzTNy25+V3zDkWeiqJQvP6zh3cWBl5tCOf+boX6VU7GLXvYYJ9YX5wZ4Y+JSM9t
    DxPsByLVfC+YCHm1xuvjB8hpOFlPyqw3Fw==
    -----END CERTIFICATE-----
  tls.key: |
    -----BEGIN RSA PRIVATE KEY-----
    MIIEpAIBAAKCAQEAtDGuDgZctv+eO7K7z6jufm3nhJ51adtsPrQJKuSKRrSLdVgm
    crpIyrZnUOTU/9PvJVFeubYsfrPyhw6z4DkZ/suVQiq2tHGY7/0aIrtifv++uQzx
    8xSzcLJfZGoggARyf5r/sLUwl5JZQusmxvATIxzZdxuPVGKRPMdtxFcCBIHR7h/N
    USKIgYlmdfDjFwN29LSAnp0OLmgRhVGFHOi4lVAJrXwT98D7pWBWYZDGSYub9sJw
    gaF3b+EUzCU7E0onmoJm+gHxjBbS4nSVQvlXmapr3KbE8Z0POPfU5BsXJZxSe2pk
    VQfyE768dgoAOC4mIc2uspwog7tp03NdDbSiDwIDAQABAoIBAC3b4ucw2VG9dmDN
    GR09ag0FHYHT7h/VtxOyMA8ZgNODyWZA2/Ag1ru4dkzRICBHqLo/njL3WRWZ6GRU
    6pRrE+GToFXplvwPWRPiv08Nj4Cwx7JCyCTMJOrOipZ8p+7MsvMk2GP5iPdaSJtO
    S10f5k7uXa4BdpXMTBhCzP3GAv5wOlC88oMB2EQkFIlBliIAA/vGHIiPFxRKgWd8
    /kCeG0wXnsiYGNl6q4b4eziUGe1z/BqMBJEPNjMLw2G3YgL17uWeLsIASUCoSpZs
    Qx0V5DJVrpudJoUAZntGYHIMEokpJbh7xkamOlUiJKFq4Fm+/5fOFfRj7qgGPVIV
    L1guo/kCgYEA1ReStbYR/ogI7njW5MY+5XKfdZLMDeYUVv053A+Gv99BsLNzrrN4
    3yHs+ePaE90AN9HZWyjAklH6jPeuKVcgsYobtxxhpgH9WCqxc2KlrsCMSG1Cj93W
    bnui7BfpG01vA16DISchRGHAOcWKdGTl2chnHmPDFW3MO7RGMbgENuMCgYEA2HpJ
    UvmXZz2o6tZgAglFhyaqCwVAwpcqABd9GoAEJYaw/8ByT0tB4TY7QlIabLeho19X
    RQDnSpkayUD7zvp+Xz9d4u2G0+m6g3iPD76DfzgY1CO2A3fVvSenZbLJCUFtb/7P
    +3LW5n683jRtaFxDgRcVhszFGvHhxodizmSfo+UCgYBm/L+hJt/HfoOijbB+XPnk
    9uLudgY86WgHKted7bsYXJRpDDHqyz6tyL25gE/Trcn/MGK2VhnMHebT3pjDziI0
    7CS8+PCQxQFmbSvaMmEU9mZWUsgKtBKAzyxMorm8wk3W+QSzLzA61muLAFgMLP+R
    o/OCkoCkiK5eZKBQQzl2NQKBgQCHiMivsqUgdnJz1ZR2sedeHs8H51oMeyhItmwV
    U2FFPXdEKPFow+2TW6judQkmWDJTXzX8dfxZsFIc/xqpFBxa9gmKMrzfoM6t0aWA
    bf9Wf4DMES+8LCMeMt2TxsQnj1c9b4Q4IkZ6OY92b8ywMlPxVsqbg0lE/XwmGE28
    ezOYIQKBgQCWBr2iqvsMx4PFZMzl6exy1hQgYAdRc5BStaFEqAggW37q3l5rXn5m
    O0/9JWSQUPhi/mcBWko4hEWkwWAsqaR3bzzyPAtboVwm/jc1jbM3VA+xMbiVHXTN
    sxph/gHtsyHwccvrx94d1YN1/oLO14KcvK3fxRpVamlzN5e32oZo2g==
    -----END RSA PRIVATE KEY-----`)

	obj, _, err := decoder.Decode([]byte(input), nil, nil)
	if err != nil {
		operatorLogger.Error(err, err.Error())
	}

	w.Secret = obj.(*v12.Secret)
	operatorLogger.Info("Secret has added")
}
func (w *WebhookResources) createMutatingWebhookConfiguration(decoder runtime.Decoder, operatorLogger logr.Logger) {
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
