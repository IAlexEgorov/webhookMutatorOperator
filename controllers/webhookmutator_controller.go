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
	"context"
	"fmt"
	apiv1alpha1 "github.com/IAlexEgorov/webhook-v1/api/v1alpha1"
	"github.com/go-logr/logr"
	v14 "k8s.io/api/admissionregistration/v1"
	v1apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	v13 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"os"
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
	Deployment                   *v1apps.Deployment
	Service                      *v1.Service
	ServiceAccount               *v1.ServiceAccount
	MutatingWebhookConfiguration *v14.MutatingWebhookConfiguration
	ClusterRole                  *v13.ClusterRole
	ClusterRoleBinding           *v13.ClusterRoleBinding
	Secret                       *v1.Secret
	ConfigMap                    *v1.ConfigMap
}

//+kubebuilder:rbac:groups=api.ialexegorov.neoflex.ru,resources=webhookmutators,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=api.ialexegorov.neoflex.ru,resources=webhookmutators/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=api.ialexegorov.neoflex.ru,resources=webhookmutators/finalizers,verbs=update

func (r *WebhookMutatorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	operatorLogger := logger
	operatorLogger.Info("Reconcile called")

	factory := serializer.NewCodecFactory(r.Scheme)
	decoder := factory.UniversalDeserializer()

	webhookMutator := &apiv1alpha1.WebhookMutator{}
	webhookResources := WebhookResources{}

	//if err := r.cleanupResourcesIfOperatorDeleted(); err != nil {
	//	return ctrl.Result{}, err
	//}

	err := r.Get(ctx, req.NamespacedName, webhookMutator)
	if err != nil {
		clientset, err := getKubernetesClient()
		if err != nil {
			return ctrl.Result{}, nil
		}
		webhookResources.deleteAllResources(clientset, &ctx, &operatorLogger)
		return ctrl.Result{}, nil
	}

	clientset, err := getKubernetesClient()
	if err != nil {
		return ctrl.Result{}, nil
	}

	// ----------------------
	// Declaration Resources
	// ----------------------
	webhookResources.declareConfigMap(decoder, operatorLogger)
	webhookResources.declareSecret(decoder, operatorLogger)

	webhookResources.declareServiceAccount(decoder, operatorLogger)
	webhookResources.declareClusterRole(decoder, operatorLogger)
	webhookResources.declareClusterRoleBinding(decoder, operatorLogger)

	webhookResources.declareDeployment(decoder, operatorLogger)
	webhookResources.declareService(decoder, operatorLogger)

	webhookResources.declareMutatingWebhookConfiguration(decoder, operatorLogger)

	// -----------------------
	// Installation Resources
	// -----------------------
	webhookResources.createOrUpdateConfigMap(clientset, &ctx, &operatorLogger)
	webhookResources.createOrUpdateSecret(clientset, &ctx, &operatorLogger)

	webhookResources.createOrUpdateServiceAccount(clientset, &ctx, &operatorLogger)
	webhookResources.createOrUpdateClusterRole(clientset, &ctx, &operatorLogger)
	webhookResources.createOrUpdateClusterRoleBinding(clientset, &ctx, &operatorLogger)

	webhookResources.createOrUpdateDeployment(clientset, &ctx, &operatorLogger)
	webhookResources.createOrUpdateService(clientset, &ctx, &operatorLogger)

	webhookResources.createOrUpdateMutatingWebhookConfiguration(clientset, &ctx, &operatorLogger)

	return ctrl.Result{RequeueAfter: time.Duration(30 * time.Second)}, nil
}

func getKubernetesClient() (*kubernetes.Clientset, error) {
	var config *rest.Config
	var err error

	if config, err = rest.InClusterConfig(); err != nil {
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			kubeconfig = os.Getenv("HOME") + "/.kube/config"
		}
		if config, err = clientcmd.BuildConfigFromFlags("", kubeconfig); err != nil {
			return nil, fmt.Errorf("failed to build Kubernetes configuration: %v", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes clientset: %v", err)
	}

	return clientset, nil
}
func (w *WebhookResources) createOrUpdateDeployment(clientset *kubernetes.Clientset, ctx *context.Context, operatorLogger *logr.Logger) {

	//labelSelector := labels.Set(w.Deployment.Labels).AsSelector()
	deployments, err := clientset.AppsV1().Deployments(w.Deployment.Namespace).List(*ctx, metav1.ListOptions{
		LabelSelector: "aegorov=webhook",
	})

	if len(deployments.Items) == 0 {
		_, err = clientset.AppsV1().Deployments(w.Deployment.Namespace).Create(context.Background(), w.Deployment, metav1.CreateOptions{})
		if err != nil {
			operatorLogger.Error(err, fmt.Sprintf("Failed to create deployment: %v", w.Deployment.GetName()))
			return
		}
		operatorLogger.Info(fmt.Sprintf("Deployment has created: %s\n", w.Deployment.GetName()))
		return
	}

	for _, deploy := range deployments.Items {
		operatorLogger.Info(fmt.Sprintf("Found deployment %s\n", deploy.GetName()))
		_, err = clientset.AppsV1().Deployments(w.Deployment.Namespace).Get(context.Background(), deploy.GetName(), metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				// If the deployment doesn't exist, create a new one
				_, err = clientset.AppsV1().Deployments(w.Deployment.Namespace).Create(context.Background(), &deploy, metav1.CreateOptions{})
				if err != nil {
					operatorLogger.Error(err, fmt.Sprintf("Failed to create deployment: %v", deploy.GetName()))
				}
			} else {
				operatorLogger.Error(err, fmt.Sprintf("Failed to get deployment: %v", deploy.GetName()))
			}
		} else {
			// If the deployment exists, update it
			_, err = clientset.AppsV1().Deployments(w.Deployment.Namespace).Update(context.Background(), &deploy, metav1.UpdateOptions{})
			if err != nil {
				operatorLogger.Error(err, fmt.Sprintf("Failed to update deployment: %v", deploy.GetName()))
			}
		}
	}
}
func (w *WebhookResources) createOrUpdateService(clientset *kubernetes.Clientset, ctx *context.Context, operatorLogger *logr.Logger) {

	//labelSelector := labels.Set(w.Service.Labels).AsSelector()
	services, err := clientset.CoreV1().Services(w.Service.Namespace).List(*ctx, metav1.ListOptions{
		LabelSelector: "aegorov=webhook",
	})

	if len(services.Items) == 0 {
		_, err = clientset.CoreV1().Services(w.Service.Namespace).Create(context.Background(), w.Service, metav1.CreateOptions{})
		if err != nil {
			operatorLogger.Error(err, fmt.Sprintf("Failed to create service: %v", w.Service.GetName()))
		}
		operatorLogger.Info(fmt.Sprintf("Service has created: %s\n", w.Service.GetName()))
		return
	}

	for _, svc := range services.Items {
		operatorLogger.Info(fmt.Sprintf("Found service %s\n", svc.GetName()))
		_, err = clientset.CoreV1().Services(w.Service.Namespace).Get(context.Background(), svc.GetName(), metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				// If the service doesn't exist, create a new one
				_, err = clientset.CoreV1().Services(w.Service.Namespace).Create(context.Background(), &svc, metav1.CreateOptions{})
				if err != nil {
					operatorLogger.Error(err, fmt.Sprintf("Failed to create service: %v", svc.GetName()))
				}
			} else {
				operatorLogger.Error(err, fmt.Sprintf("Failed to get service: %v", svc.GetName()))
			}
		} else {
			// If the service exists, update it
			_, err = clientset.CoreV1().Services(w.Service.Namespace).Update(context.Background(), &svc, metav1.UpdateOptions{})
			if err != nil {
				operatorLogger.Error(err, fmt.Sprintf("Failed to update service: %v", svc.GetName()))
			}
		}
	}
}
func (w *WebhookResources) createOrUpdateConfigMap(clientset *kubernetes.Clientset, ctx *context.Context, operatorLogger *logr.Logger) {

	configmaps, err := clientset.CoreV1().ConfigMaps(w.ConfigMap.Namespace).List(*ctx, metav1.ListOptions{
		LabelSelector: "aegorov=webhook",
	})

	if len(configmaps.Items) == 0 {
		_, err = clientset.CoreV1().ConfigMaps(w.ConfigMap.Namespace).Create(context.Background(), w.ConfigMap, metav1.CreateOptions{})
		if err != nil {
			operatorLogger.Error(err, fmt.Sprintf("Failed to create configmap: %v", w.ConfigMap.GetName()))
			return
		}
		operatorLogger.Info(fmt.Sprintf("ConfigMap has created: %s\n", w.ConfigMap.GetName()))
		return
	}

	for _, cm := range configmaps.Items {
		operatorLogger.Info(fmt.Sprintf("Found configmap %s\n", cm.GetName()))
		_, err = clientset.CoreV1().ConfigMaps(w.ConfigMap.Namespace).Get(context.Background(), cm.GetName(), metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				// If the configmap doesn't exist, create a new one
				_, err = clientset.CoreV1().ConfigMaps(w.ConfigMap.Namespace).Create(context.Background(), &cm, metav1.CreateOptions{})
				if err != nil {
					operatorLogger.Error(err, fmt.Sprintf("Failed to create configmap: %v", cm.GetName()))
				}
			} else {
				operatorLogger.Error(err, fmt.Sprintf("Failed to get configmap: %v", cm.GetName()))
			}
		} else {
			// If the configmap exists, update it
			_, err = clientset.CoreV1().ConfigMaps(w.ConfigMap.Namespace).Update(context.Background(), &cm, metav1.UpdateOptions{})
			if err != nil {
				operatorLogger.Error(err, fmt.Sprintf("Failed to update configmap: %v", cm.GetName()))
			}
		}
	}
}
func (w *WebhookResources) createOrUpdateSecret(clientset *kubernetes.Clientset, ctx *context.Context, operatorLogger *logr.Logger) {
	secrets, err := clientset.CoreV1().Secrets(w.Secret.Namespace).List(*ctx, metav1.ListOptions{
		LabelSelector: "aegorov=webhook",
	})

	if len(secrets.Items) == 0 {
		_, err = clientset.CoreV1().Secrets(w.Secret.Namespace).Create(context.Background(), w.Secret, metav1.CreateOptions{})
		if err != nil {
			operatorLogger.Error(err, fmt.Sprintf("Failed to create secret: %v", w.Secret.GetName()))
		}
		operatorLogger.Info(fmt.Sprintf("Secret has created: %s\n", w.Secret.GetName()))
		return
	}

	for _, secret := range secrets.Items {
		operatorLogger.Info(fmt.Sprintf("Found secret %s\n", secret.GetName()))
		_, err = clientset.CoreV1().Secrets(w.Secret.Namespace).Get(context.Background(), secret.GetName(), metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				// If the configmap doesn't exist, create a new one
				_, err = clientset.CoreV1().Secrets(w.Secret.Namespace).Create(context.Background(), &secret, metav1.CreateOptions{})
				if err != nil {
					operatorLogger.Error(err, fmt.Sprintf("Failed to create secret: %v", secret.GetName()))
				}
			} else {
				operatorLogger.Error(err, fmt.Sprintf("Failed to get secret: %v", secret.GetName()))
			}
		} else {
			// If the configmap exists, update it
			_, err = clientset.CoreV1().Secrets(w.Secret.Namespace).Update(context.Background(), &secret, metav1.UpdateOptions{})
			if err != nil {
				operatorLogger.Error(err, fmt.Sprintf("Failed to update secret: %v", secret.GetName()))
			}
		}
	}
}
func (w *WebhookResources) createOrUpdateServiceAccount(clientset *kubernetes.Clientset, ctx *context.Context, operatorLogger *logr.Logger) {

	_, err := clientset.CoreV1().ServiceAccounts(w.ServiceAccount.Namespace).Get(context.Background(), w.ServiceAccount.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// If the ServiceAccount doesn't exist, create a new one
			_, err = clientset.CoreV1().ServiceAccounts(w.ServiceAccount.Namespace).Create(context.Background(), w.ServiceAccount, metav1.CreateOptions{})
			if err != nil {
				operatorLogger.Error(err, fmt.Sprintf("Failed to create ServiceAccount: %v", w.ServiceAccount.GetName()))
			}
			operatorLogger.Info(fmt.Sprintf("ServiceAccount has been created: %s\n", w.ServiceAccount.GetName()))
			return
		} else {
			operatorLogger.Error(err, fmt.Sprintf("Failed to get ServiceAccount: %v", w.ServiceAccount.GetName()))
			return
		}
	}

	// If the ServiceAccount exists, update it
	_, err = clientset.CoreV1().ServiceAccounts(w.ServiceAccount.Namespace).Update(context.Background(), w.ServiceAccount, metav1.UpdateOptions{})
	if err != nil {
		operatorLogger.Error(err, fmt.Sprintf("Failed to update ServiceAccount: %v", w.ServiceAccount.GetName()))
		return
	}
	operatorLogger.Info(fmt.Sprintf("ServiceAccount has been updated: %s\n", w.ServiceAccount.GetName()))
}
func (w *WebhookResources) createOrUpdateClusterRole(clientset *kubernetes.Clientset, ctx *context.Context, operatorLogger *logr.Logger) {

	//labelSelector := labels.Set(w.ClusterRole.Labels).AsSelector()
	clusterRoles, err := clientset.RbacV1().ClusterRoles().List(*ctx, metav1.ListOptions{
		LabelSelector: "aegorov=webhook",
	})

	if len(clusterRoles.Items) == 0 {
		_, err = clientset.RbacV1().ClusterRoles().Create(context.Background(), w.ClusterRole, metav1.CreateOptions{})
		if err != nil {
			operatorLogger.Error(err, fmt.Sprintf("Failed to create ClusterRole: %v", w.ClusterRole.GetName()))
		}
		operatorLogger.Info(fmt.Sprintf("ClusterRole has created: %s\n", w.ClusterRole.GetName()))
		return
	}

	for _, clusterRole := range clusterRoles.Items {
		operatorLogger.Info(fmt.Sprintf("Found ClusterRole %s\n", clusterRole.GetName()))
		_, err = clientset.RbacV1().ClusterRoles().Get(context.Background(), clusterRole.GetName(), metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				// If the ClusterRole doesn't exist, create a new one
				_, err = clientset.RbacV1().ClusterRoles().Create(context.Background(), &clusterRole, metav1.CreateOptions{})
				if err != nil {
					operatorLogger.Error(err, fmt.Sprintf("Failed to create ClusterRole: %v", clusterRole.GetName()))
				}
			} else {
				operatorLogger.Error(err, fmt.Sprintf("Failed to get ClusterRole: %v", clusterRole.GetName()))
			}
		} else {
			// If the ClusterRole exists, update it
			_, err = clientset.RbacV1().ClusterRoles().Update(context.Background(), &clusterRole, metav1.UpdateOptions{})
			if err != nil {
				operatorLogger.Error(err, fmt.Sprintf("Failed to update ClusterRole: %v", clusterRole.GetName()))
			}
		}
	}
}
func (w *WebhookResources) createOrUpdateClusterRoleBinding(clientset *kubernetes.Clientset, ctx *context.Context, operatorLogger *logr.Logger) {

	//labelSelector := labels.Set(w.ClusterRoleBinding.Labels).AsSelector()
	clusterRoleBindings, err := clientset.RbacV1().ClusterRoleBindings().List(*ctx, metav1.ListOptions{
		LabelSelector: "aegorov=webhook",
	})

	if len(clusterRoleBindings.Items) == 0 {
		_, err = clientset.RbacV1().ClusterRoleBindings().Create(context.Background(), w.ClusterRoleBinding, metav1.CreateOptions{})
		if err != nil {
			operatorLogger.Error(err, fmt.Sprintf("Failed to create cluster role binding: %v", w.ClusterRoleBinding.GetName()))
		}
		operatorLogger.Info(fmt.Sprintf("Cluster role binding has created: %s\n", w.ClusterRoleBinding.GetName()))
		return
	}

	for _, crb := range clusterRoleBindings.Items {
		operatorLogger.Info(fmt.Sprintf("Found cluster role binding %s\n", crb.GetName()))
		_, err = clientset.RbacV1().ClusterRoleBindings().Get(context.Background(), crb.GetName(), metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				// If the cluster role binding doesn't exist, create a new one
				_, err = clientset.RbacV1().ClusterRoleBindings().Create(context.Background(), &crb, metav1.CreateOptions{})
				if err != nil {
					operatorLogger.Error(err, fmt.Sprintf("Failed to create cluster role binding: %v", crb.GetName()))
				}
			} else {
				operatorLogger.Error(err, fmt.Sprintf("Failed to get cluster role binding: %v", crb.GetName()))
			}
		} else {
			// If the cluster role binding exists, update it
			_, err = clientset.RbacV1().ClusterRoleBindings().Update(context.Background(), &crb, metav1.UpdateOptions{})
			if err != nil {
				operatorLogger.Error(err, fmt.Sprintf("Failed to update cluster role binding: %v", crb.GetName()))
			}
		}
	}
}
func (w *WebhookResources) createOrUpdateMutatingWebhookConfiguration(clientset *kubernetes.Clientset, ctx *context.Context, operatorLogger *logr.Logger) {

	configurations, err := clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().List(*ctx, metav1.ListOptions{
		LabelSelector: "aegorov=webhook",
	})
	if err != nil {
		operatorLogger.Error(err, "Failed to get MutatingWebhookConfigurations")
		return
	}

	if len(configurations.Items) == 0 {

		// If the MutatingWebhookConfiguration doesn't exist, create a new one
		_, err = clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().Create(context.Background(), w.MutatingWebhookConfiguration, metav1.CreateOptions{})
		if err != nil {
			operatorLogger.Error(err, fmt.Sprintf("Failed to create MutatingWebhookConfiguration: %v", w.MutatingWebhookConfiguration.Name))
			return
		}
		operatorLogger.Info(fmt.Sprintf("MutatingWebhookConfiguration %s created successfully", w.MutatingWebhookConfiguration.Name))

	}
}

func (w *WebhookResources) declareConfigMap(decoder runtime.Decoder, operatorLogger logr.Logger) {

	input := fmt.Sprintf(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: webhook-config
  namespace: default
  labels:
    aegorov: webhook
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

	w.ConfigMap = obj.(*v1.ConfigMap)
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
    aegorov: webhook
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
          name: webhook-config
`)

	obj, _, err := decoder.Decode([]byte(input), nil, nil)
	if err != nil {
		operatorLogger.Error(err, err.Error())
	}

	w.Deployment = obj.(*v1apps.Deployment)
	operatorLogger.Info("Deployment has added")
}
func (w *WebhookResources) declareService(decoder runtime.Decoder, operatorLogger logr.Logger) {
	input := fmt.Sprintf(`
apiVersion: v1
kind: Service
metadata:
  name: aegorov-admission
  namespace: default
  labels:
    aegorov: webhook
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

	w.Service = obj.(*v1.Service)
	operatorLogger.Info("Service has added")
}
func (w *WebhookResources) declareServiceAccount(decoder runtime.Decoder, operatorLogger logr.Logger) {
	input := fmt.Sprintf(`
apiVersion: v1
kind: ServiceAccount
metadata:
  name: aegorov-admission-webhook
  namespace: default
  labels:
    aegorov: webhook`)

	obj, _, err := decoder.Decode([]byte(input), nil, nil)
	if err != nil {
		operatorLogger.Error(err, err.Error())
	}

	w.ServiceAccount = obj.(*v1.ServiceAccount)
	operatorLogger.Info("ServiceAccount has added")
}
func (w *WebhookResources) declareClusterRole(decoder runtime.Decoder, operatorLogger logr.Logger) {
	input := fmt.Sprintf(`
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: aegorov-admission-webhook
  namespace: default
  labels:
    aegorov: webhook
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
  namespace: default
  labels:
    aegorov: webhook
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
  namespace: default
  labels:
    aegorov: webhook
type: Opaque
data:
  tls.crt: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUVOVENDQXgyZ0F3SUJBZ0lVYk1xcm9QZVQzdnVrbFNRQUdBSTBaK1NnT2hjd0RRWUpLb1pJaHZjTkFRRUwKQlFBd1R6RUxNQWtHQTFVRUJoTUNVbFV4RURBT0JnTlZCQWdUQjBWNFlXMXdiR1V4RHpBTkJnTlZCQWNUQmsxdgpjMk52ZHpFUU1BNEdBMVVFQ2hNSFJYaGhiWEJzWlRFTE1Ba0dBMVVFQ3hNQ1EwRXdIaGNOTWpNd05UQTRNVEV5Ck56QXdXaGNOTkRNd05UQXpNVEV5TnpBd1dqQlBNUXN3Q1FZRFZRUUdFd0pTVlRFUU1BNEdBMVVFQ0JNSFJYaGgKYlhCc1pURVBNQTBHQTFVRUJ4TUdUVzl6WTI5M01SQXdEZ1lEVlFRS0V3ZEZlR0Z0Y0d4bE1Rc3dDUVlEVlFRTApFd0pEUVRDQ0FTSXdEUVlKS29aSWh2Y05BUUVCQlFBRGdnRVBBRENDQVFvQ2dnRUJBT2dJaURkekEvdXNIMXpuCnA5T3JCVE5RdkhpRmN2NllmYjM3WHVHblprVHZiQzdEQ1JWQnkzUzc5THd2U01Pa2FQTjczUHZhM0wxck5CM04Kc24wQnBoOE5rRXIxb0Rjek4rRzBUbkZjZEZ6dHVMS3NEaGRkWkxlQmpuVnhMS2FCVkpYZjJhNmZoeUdnbThGWAp4TmV1SXdjcU96bHQzT2Y0SEg4RFozRVJIbDd1NFByeXJWSHREclhuam0xeFRmUGRYK0kwSjRLVC9Ja2ZzVG5xCjM1MTFwSFlHYzJIdDVOV2FVTlRjYkxxMWpsRG9lYWlEK0p1eXY5djRYUkdaOVNsT3h1eUhIbGF2RHNIeVZ0U2sKaEZGRHVCVk1xNDE1elJyUTBTSXFUUDZsdFpScXJuQmRaRGdKYWlqdkRQRVhMUkpzdmtSTmw0NFdHVkNyMm4vMwpGUkUvYkVFQ0F3RUFBYU9DQVFjd2dnRURNQTRHQTFVZER3RUIvd1FFQXdJRm9EQWRCZ05WSFNVRUZqQVVCZ2dyCkJnRUZCUWNEQVFZSUt3WUJCUVVIQXdJd0RBWURWUjBUQVFIL0JBSXdBREFkQmdOVkhRNEVGZ1FVeFZHc2ZOY0sKNmk0RW5QVDJONWJNY3pQM0Nvb3dnYVFHQTFVZEVRU0JuRENCbVlJcllXVm5iM0p2ZGkxaFpHMXBjM05wYjI0dQpaR1ZtWVhWc2RDNXpkbU11WTJ4MWMzUmxjaTVzYjJOaGJJSVJZV1ZuYjNKdmRpMWhaRzFwYzNOcGIyNkNKMkZsCloyOXliM1l0WVdSdGFYTnphVzl1TG1SbFptRjFiSFF1WTJ4MWMzUmxjaTVzYjJOaGJJSWRZV1ZuYjNKdmRpMWgKWkcxcGMzTnBiMjR1WkdWbVlYVnNkQzV6ZG1PQ0NXeHZZMkZzYUc5emRJY0Vmd0FBQVRBTkJna3Foa2lHOXcwQgpBUXNGQUFPQ0FRRUFJWFVQSDEzNTJBRFN4cUlGc2owSktUWGtRY3FTV1lETzEzZS9scER1ampPdXgvRWFCVmpsCkVNV2licnpDUGx3ZnFONUtEK0xDbzVLZGx1bW1TVFRnTjZXalBrWkNCeUxaVXVlazlmQnFzcG1SUDZ3QUo0SnYKTDVpalJoK2ZFdTlRZE55RUQrUWJOaUYrdjdpcWpzaFIwbHh0OVZXZmhETElMZUFtVGNyZmN0eURxKzVrYmh4egp0Y1JweFU0SThSaGdweWJFelY4bVpteUE2VkYyaUIzVVFIUnc3anV6cW5XRXFHa0R3OENleW1QZi9Yby9MSjZZCjVSLzJ2Q1FJb2VYRlJCUyt2cDExQit2QzhxMlpZb0UvL0drWXdVdnovTjZ4bHh5dlE1OG9aZUFXTWJiTDVzT0EKTHVDR2YyYS9vWWFnMlQrd1FNeStaenJMemoxRHdUVEdHQT09Ci0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K
  tls.key: LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpNSUlFb2dJQkFBS0NBUUVBNkFpSU4zTUQrNndmWE9lbjA2c0ZNMUM4ZUlWeS9waDl2ZnRlNGFkbVJPOXNMc01KCkZVSExkTHYwdkM5SXc2Um84M3ZjKzlyY3ZXczBIYzJ5ZlFHbUh3MlFTdldnTnpNMzRiUk9jVngwWE8yNHNxd08KRjExa3Q0R09kWEVzcG9GVWxkL1pycCtISWFDYndWZkUxNjRqQnlvN09XM2M1L2djZndObmNSRWVYdTdnK3ZLdApVZTBPdGVlT2JYRk44OTFmNGpRbmdwUDhpUit4T2VyZm5YV2tkZ1p6WWUzazFacFExTnhzdXJXT1VPaDVxSVA0Cm03Sy8yL2hkRVpuMUtVN0c3SWNlVnE4T3dmSlcxS1NFVVVPNEZVeXJqWG5OR3REUklpcE0vcVcxbEdxdWNGMWsKT0FscUtPOE04UmN0RW15K1JFMlhqaFlaVUt2YWYvY1ZFVDlzUVFJREFRQUJBb0lCQUFpd09SbUtidjIvaGpVZQpYNFJuaFB4VTY1bS90WHlmRFNaT0FWR0Z5U2lQcG9kaHVqZFhqVnpEcFBoZTlPU09oWGVJamMvSWREZUxpaG9MCmw4RmlqR3ZoUUNQdWFwOW1oWk1vQXovdmJGUUdlc0lGKzBrWXNDckc2U1N3cGpGZDZtTHFUT1pqQnRaVmd6K00KSDh6THNuZ1VOcitCdzZIVUFvMG0vWHFZWDREQ2N0djA3emFSQk9uSkdWZHNRN1crUGFzcXhXMThoYnZjeTM3MwppcW54S0c0bVJzb1dTKys5eVZMUGs2Q1ROSEN6SkJwc2QxOHBrZmVwN09UcSszTEk1Q09pdFZDZjVveDk1ZXV2CjU1WW0yd3h1TDZOMnpNMUdEdndiOVBLSVYvNVR0cVQzR3BvdWRVTGc4MGZ5UlRwMHZ2MEtodGIzTi8zajdNQ0MKanhYakdvRUNnWUVBN1Y4RWxEd2ZBUkJoTkFqRlFUb2tROFBUVTBkcGlBalZ1ZnRtRHJRTmxWL2lqNG96LzNTWApYbEh2a1YrN2VDZVFNRkVjMGgreVZGeWNrWjhId01RSDlnc3M3ZUV4L1dPQlVyVkdqeEh0ME9xcFkrMXZjNnVjCjRvdHpHK01hZ3dOQVBSaDJ0c0QyYW1sK0pBUVZUMnVXV3k2ZjdTaVNSMDU2SXRYTW9vakxHMGtDZ1lFQStqNUYKMEIwVlFPTkgyandIVUwyeFBqQkdlbjFQUWNML2pISW5MUG15NitZVVdrL0o5WEN3TnhkaDI0eFdhR0FOVFNSdgpoR2tnT1V1Z2xsMkM3NUd1YWNIMzBqbEdFc25ZQ216S1VVQlZKdUE3eWFUTEdxQmFGZ0VGOG5jQVpkS0FGNDdNCnBKRjE4N2RJTldjYm5WQ3NGMTcyRTJVWGI5VWpDN05xOVlNQ2tUa0NnWUI1ZnBEUmJwUlA3eHBSajh1bXZ5T2cKcTdLV2hZNjJXZzlLeWlwS2pFNEhqclJmMDlVWmc0dVdjMG16bHRSVmc2cUJrSUszNmhGVXJMSld0cGM1U3h6bwpDb0JNb1Y3ODJ0bHVnK3BCZ0dQQTh0c1FrbzdoSFkySFJ1ajc5Um0weFEwME9EbExBU2tlL2kvYUwxelk4YkJiCnExbWdBWXdkZzBWd1h3NEdndzJ5UVFLQmdGdjBHZzg1UUtBUlpFdkxGeDBTTjFrVXdERXVicnRKZmtJTGlGMjgKZTRTM2pPOEt0cm1iNlFTMWNONE9HWXBORVZZeGQxRCttRHExa1pMdlZiZldubko2TmlobnAxb3NGVmp2VlFDNgpWUS91QWNvODVlMG8wekdXdXFxNEU4dFdxSDcvbUM4NHpGRDhIbXFSTXRLQjNGclNLRFpFUlhKd3JXb1ZTYzVoCmo4WHhBb0dBWUx2Y09aYWdDSWwwbTQzcmI4MndsaFYvcFo1NlorU0VNMFFGTCthOTFheHhkZC91WmVaY1ErTVgKWDlvSytMbWNuekhrbkdGMmgzTS9DaE9kc3psMmkzNjg1S3JmR3pYbmpqZjVjd3B0K3FTaXkxZHNlZUY0SFpvYgppY1NaTmxheWtoKzBLK0ovOXRlL0FLTmxlbmFDaUw0SkFMY0lRdS8rMUFuNVU0QzRsSjg9Ci0tLS0tRU5EIFJTQSBQUklWQVRFIEtFWS0tLS0tCg== 
`)

	obj, _, err := decoder.Decode([]byte(input), nil, nil)
	if err != nil {
		operatorLogger.Error(err, err.Error())
	}

	w.Secret = obj.(*v1.Secret)
	operatorLogger.Info("Secret has added")
}
func (w *WebhookResources) declareMutatingWebhookConfiguration(decoder runtime.Decoder, operatorLogger logr.Logger) {
	input := fmt.Sprintf(`
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: aegorov-admission
  namespace: default
  labels:
    aegorov: webhook
webhooks:
  - name: aegorov-admission.default.svc.cluster.local
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
      caBundle: "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURiakNDQWxhZ0F3SUJBZ0lVRzhyOWNHVi8xeUdweFdSbGZnU1RoVG9LMDBzd0RRWUpLb1pJaHZjTkFRRUwKQlFBd1R6RUxNQWtHQTFVRUJoTUNVbFV4RURBT0JnTlZCQWdUQjBWNFlXMXdiR1V4RHpBTkJnTlZCQWNUQmsxdgpjMk52ZHpFUU1BNEdBMVVFQ2hNSFJYaGhiWEJzWlRFTE1Ba0dBMVVFQ3hNQ1EwRXdIaGNOTWpNd05UQTRNVEV5Ck56QXdXaGNOTWpnd05UQTJNVEV5TnpBd1dqQlBNUXN3Q1FZRFZRUUdFd0pTVlRFUU1BNEdBMVVFQ0JNSFJYaGgKYlhCc1pURVBNQTBHQTFVRUJ4TUdUVzl6WTI5M01SQXdEZ1lEVlFRS0V3ZEZlR0Z0Y0d4bE1Rc3dDUVlEVlFRTApFd0pEUVRDQ0FTSXdEUVlKS29aSWh2Y05BUUVCQlFBRGdnRVBBRENDQVFvQ2dnRUJBTEY4bkMzb21RK3Jaa3c4Ckw1elZNWnlnNmFvVCt0SUREeXZjWEcwbmlWa1pzUXJ1VUJJL1ZsN0ZZVWxFWmt6TkszZDgwbW5maW1ab2lhZjMKMlc3a3dzOW5ZVFhBVDJINzlSQjhtMDZEVEozTDdVV3hXQUw5K0J2dnJyTGF1MXB5TGlIVnFjK0NSOERWWHNuUQpPSzhwNWk5VlB6SmJzdmJFbWYzUG5JYUVYWHZmclBlUmpYdDNMejBQY1BEbGhneEF1Q09MamFKZlpBU01La290Ci9zWE14bzFtdDU3QzcxYXIxY01jNGhlNmNhZ2d4aEtpTVdZWmZDd1RPQWoyckthbTlDenRNK0JnYU5Ib0xZYlMKT3ZzbG90YUtSbm9tU1NnekczQ0xKeXljekE4NjZycUdxbjFmeXhPaFd0a1FSVjl4VjlYeDVOdlZrMktwSlhObgpvSlFBYmdrQ0F3RUFBYU5DTUVBd0RnWURWUjBQQVFIL0JBUURBZ0VHTUE4R0ExVWRFd0VCL3dRRk1BTUJBZjh3CkhRWURWUjBPQkJZRUZEdjBqNW94dmJzcnYxVlNUSC92UUZiMXdheE5NQTBHQ1NxR1NJYjNEUUVCQ3dVQUE0SUIKQVFCTTF1VUh3MHZOODk4QUp6UVcvOHR4TS93aWRoUGhqalRla08raEZ6d29UaGF5Z204TmxHZVNmNVRIbEozawpVVGw2MWFPU3Mrazg4VFFZdCtCY0hHMlhuSC85TkRRMmFBVWhmT3R5ejlySEpZRXVIbGNZSjI5VEcrbVl5WFpSCmNZVEt5U0QyUTdkMTRxWWRBa2lBa2RaVGZDWkJzMmYzSUUzOGtrTHpoRDkwOU9vem1xMnpYaEk0RExxSks0dU4KeFpIOGFuMmVvMGZVVThDWHJhVjB4dzViaXFQN2RSai9aTU1STnJhSWdIWmFQQ0FCR1dMbEpyZ25kZmNKWUx6dApEWm9YczRwZ2VlMmVab0d3QW1pWUtBc1RCK040ZTdFQllnaE9MYXpvaGI3bmhIMHF3VDY0TlJnMXZMKzFTNVZuCjd5SGN0WjgxYUtiWHJYcmtZZlNoVHh0TAotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg=="
    rules:
      - operations: [ "CREATE" ]
        apiGroups: [ "apps" ]
        apiVersions: [ "v1" ]
        resources: [ "statefulsets" ]
`)

	obj, _, err := decoder.Decode([]byte(input), nil, nil)
	if err != nil {
		operatorLogger.Error(err, err.Error())
	}

	w.MutatingWebhookConfiguration = obj.(*v14.MutatingWebhookConfiguration)
	operatorLogger.Info("MutatingWebhookConfiguration has added")
}

func (w *WebhookResources) deleteAllResources(clientset *kubernetes.Clientset, ctx *context.Context, operatorLogger *logr.Logger) {

	err := clientset.AppsV1().Deployments(w.Deployment.Namespace).Delete(*ctx, w.Deployment.Name, metav1.DeleteOptions{})
	if err != nil {
		operatorLogger.Error(err, fmt.Sprintf("Failed to delete Deployments: %v", w.Deployment.GetName()))
	}
	err = clientset.CoreV1().ConfigMaps(w.ConfigMap.Namespace).Delete(*ctx, w.ConfigMap.Name, metav1.DeleteOptions{})
	if err != nil {
		operatorLogger.Error(err, fmt.Sprintf("Failed to delete ConfigMaps: %v", w.Deployment.GetName()))
	}
	err = clientset.CoreV1().Services(w.Service.Namespace).Delete(*ctx, w.Service.Name, metav1.DeleteOptions{})
	if err != nil {
		operatorLogger.Error(err, fmt.Sprintf("Failed to delete Services: %v", w.Deployment.GetName()))
	}
	err = clientset.CoreV1().Secrets(w.Secret.Namespace).Delete(*ctx, w.Secret.Name, metav1.DeleteOptions{})
	if err != nil {
		operatorLogger.Error(err, fmt.Sprintf("Failed to delete deployment: %v", w.Deployment.GetName()))
	}
	err = clientset.CoreV1().ServiceAccounts(w.ServiceAccount.Namespace).Delete(*ctx, w.ServiceAccount.Name, metav1.DeleteOptions{})
	if err != nil {
		operatorLogger.Error(err, fmt.Sprintf("Failed to delete ServiceAccounts: %v", w.Deployment.GetName()))
	}
	err = clientset.RbacV1().ClusterRoles().Delete(*ctx, w.ClusterRole.Name, metav1.DeleteOptions{})
	if err != nil {
		operatorLogger.Error(err, fmt.Sprintf("Failed to delete ServiceAccounts: %v", w.Deployment.GetName()))
	}
	err = clientset.RbacV1().ClusterRoleBindings().Delete(*ctx, w.ClusterRoleBinding.Name, metav1.DeleteOptions{})
	if err != nil {
		operatorLogger.Error(err, fmt.Sprintf("Failed to delete ServiceAccounts: %v", w.Deployment.GetName()))
	}
	err = clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().Delete(*ctx, w.MutatingWebhookConfiguration.Name, metav1.DeleteOptions{})
	if err != nil {
		operatorLogger.Error(err, fmt.Sprintf("Failed to delete MutatingWebhookConfigurations: %v", w.Deployment.GetName()))
	}
}

func (r *WebhookMutatorReconciler) cleanupResourcesIfOperatorDeleted() error {
	//resources, err := r.getAssociatedResources()
	//if err != nil {
	//	return err
	//}
	//
	//// Check if any of the resources are missing
	//for _, resource := range resources {
	//	found, err := r.resourceExists(resource)
	//	if err != nil {
	//		return err
	//	}
	//
	//	// If the resource is missing, perform cleanup logic
	//	if !found {
	//		if err := r.cleanupResource(resource); err != nil {
	//			return err
	//		}
	//	}
	//}
	return nil
}

func (r *WebhookMutatorReconciler) getAssociatedResources() (WebhookResources, error) {
	// Implement logic to retrieve associated resources
	return WebhookResources{}, nil
}

func (r *WebhookMutatorReconciler) resourceExists(resource WebhookResources) (bool, error) {
	// Implement logic to check if the resource exists
	return true, nil
}
func (r *WebhookMutatorReconciler) cleanupResource(resource WebhookResources) error {
	// Implement cleanup logic for the resource
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WebhookMutatorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&apiv1alpha1.WebhookMutator{}).
		Complete(r)
}
