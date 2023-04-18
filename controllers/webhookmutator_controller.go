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
	"k8s.io/api/admissionregistration/v1beta1"
	v1 "k8s.io/api/apps/v1"
	v12 "k8s.io/api/core/v1"
	v13 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
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
	MutatingWebhookConfiguration *v1beta1.MutatingWebhookConfiguration
	ClusterRole                  *v13.ClusterRole
	ClusterRoleBinding           *v13.ClusterRoleBinding
}

//+kubebuilder:rbac:groups=api.ialexegorov.neoflex.ru,resources=webhookmutators,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=api.ialexegorov.neoflex.ru,resources=webhookmutators/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=api.ialexegorov.neoflex.ru,resources=webhookmutators/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the WebhookMutator object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *WebhookMutatorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	operatorLogger := logger
	operatorLogger.Info("Reconcile called")

	webhookMutator := &apiv1alpha1.WebhookMutator{}
	err := r.Get(ctx, req.NamespacedName, webhookMutator)
	if err != nil {
		operatorLogger.Info(fmt.Sprintf("YYYY cyka!!!"))
		return ctrl.Result{}, nil
	}

	err, webhookResources := createResources()
	if err != nil {
		operatorLogger.Info(fmt.Sprintf("Couldn't create resources %v", err))
		return ctrl.Result{}, nil
	}

	//webhooks:
	//	sideEffects: "None"
	//	caBundle: {{ .Values.cert.caBundle | b64enc }}

	operatorLogger.Info("Creating Deployment")
	err = r.Create(ctx, webhookResources.Deployment)
	if err != nil {
		return ctrl.Result{}, nil
	}

	operatorLogger.Info("Creating MutatingWebhookConfiguration")
	err = r.Create(ctx, webhookResources.MutatingWebhookConfiguration)
	if err != nil {
		return ctrl.Result{}, nil
	}

	operatorLogger.Info("Creating Service")
	err = r.Create(ctx, webhookResources.Service)
	if err != nil {
		return ctrl.Result{}, nil
	}

	operatorLogger.Info("Creating ServiceAccount")
	err = r.Create(ctx, webhookResources.ServiceAccount)
	if err != nil {
		return ctrl.Result{}, nil
	}

	operatorLogger.Info(fmt.Sprintf("Creating %v : %v",
		webhookResources.Service.Name, webhookResources.Service.Namespace))
	err = r.Create(ctx, webhookResources.Service)
	if err != nil {
		return ctrl.Result{}, nil
	}

	return ctrl.Result{RequeueAfter: time.Duration(30 * time.Second)}, nil
}

func createResources() (error, WebhookResources) {
	webhookResources := WebhookResources{}

	var timeoutSeconds int32 = 30
	servicePath := "/mutate/deployments"

	admissionWebhookDeployment := &v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "admission-webhook-deployment",
			Namespace: "default",
			Labels:    map[string]string{"test": "test"},
		},
		Spec: v1.DeploymentSpec{
			Replicas: nil,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"test": "test"},
			},
			Template: v12.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "admission-webhook-pod",
					Namespace: "default",
					Labels:    map[string]string{"test": "test"},
				},
				Spec: v12.PodSpec{
					Containers: []v12.Container{
						{
							Name:  "test",
							Image: "89109249948/webhook:config-version-label-bug-resticted-v1",
						},
					},
				},
			},
		},
	}
	admissionWebhookService := &v12.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "admission-webhook-service",
			Namespace: "default",
		},
		Spec: v12.ServiceSpec{
			Selector: map[string]string{"test": "test"},
			Ports: []v12.ServicePort{
				{
					Name: "application",
					Port: 443,
					TargetPort: intstr.IntOrString{
						StrVal: "tls",
					},
				},
				{
					Name: "metrics",
					Port: 80,
					TargetPort: intstr.IntOrString{
						StrVal: "metrics",
					},
				},
			},
		},
	}

	admissionWebhookServiceAccount := &v12.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "aegorov-admission-webhook",
			Namespace: "default",
		},
	}
	clusterRole := &v13.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "aegorov-admission-webhook",
			Namespace: "default",
		},
		Rules: []v13.PolicyRule{
			{
				Verbs:     []string{"get", "watch", "list"},
				APIGroups: []string{""},
				Resources: []string{"pods"},
			},
		},
	}
	clusterRoleBinding := &v13.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "",
			Namespace: "",
		},
		Subjects: []v13.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "aegorov-admission-webhook",
				Namespace: "default",
			},
		},
		RoleRef: v13.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "aegorov-admission-webhook",
		},
	}

	mutationWebhookConfiguration := &v1beta1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "mutation-webhook-configuration",
			Namespace:       "default",
			Labels:          nil,
			Annotations:     nil,
			OwnerReferences: nil,
			Finalizers:      nil,
			ManagedFields:   nil,
		},
		Webhooks: []v1beta1.MutatingWebhook{
			{
				Name: "admission-webhook-service.default.svc",
				ClientConfig: v1beta1.WebhookClientConfig{
					URL: nil,
					Service: &v1beta1.ServiceReference{
						Namespace: "default",
						Name:      "admission-webhook-service",
						Path:      &servicePath,
					},
					CABundle: nil,
				},
				Rules: []v1beta1.RuleWithOperations{{
					Operations: []v1beta1.OperationType{"CREATE"},
					Rule: v1beta1.Rule{
						APIGroups:   []string{"apps"},
						APIVersions: []string{"v1"},
						Resources:   []string{"statefulsets"},
					},
				},
				},
				ObjectSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"example-webhook-enabled": "true"},
				},
				TimeoutSeconds:          &timeoutSeconds,
				AdmissionReviewVersions: []string{"v1beta1"},
			},
		},
	}

	webhookResources = WebhookResources{
		Deployment:                   admissionWebhookDeployment,
		Service:                      admissionWebhookService,
		ServiceAccount:               admissionWebhookServiceAccount,
		MutatingWebhookConfiguration: mutationWebhookConfiguration,
		ClusterRole:                  clusterRole,
		ClusterRoleBinding:           clusterRoleBinding,
	}

	return nil, webhookResources
}

// SetupWithManager sets up the controller with the Manager.
func (r *WebhookMutatorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&apiv1alpha1.WebhookMutator{}).
		Complete(r)
}
