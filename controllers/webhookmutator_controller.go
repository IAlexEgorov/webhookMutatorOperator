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
	v14 "k8s.io/api/admissionregistration/v1"
	v1 "k8s.io/api/apps/v1"
	v12 "k8s.io/api/core/v1"
	v13 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

	webhookResources := createResources()

	operatorLogger.Info("   ----- Creating block -----")
	operatorLogger.Info("Creating ServiceAccount")

	err = r.Update(ctx, webhookResources.ServiceAccount)
	if err != nil {
		return ctrl.Result{}, nil
	}
	operatorLogger.Info("Creating ClusterRole")
	err = r.Update(ctx, webhookResources.ClusterRole)
	if err != nil {
		return ctrl.Result{}, nil
	}
	operatorLogger.Info("Creating ClusterRoleBinding")
	err = r.Update(ctx, webhookResources.ClusterRoleBinding)
	if err != nil {
		return ctrl.Result{}, nil
	}

	operatorLogger.Info("Creating Secret")
	err = r.Update(ctx, webhookResources.Secret)
	if err != nil {
		return ctrl.Result{}, nil
	}
	operatorLogger.Info("Creating ConfigMap")
	err = r.Update(ctx, webhookResources.ConfigMap)
	if err != nil {
		return ctrl.Result{}, nil
	}

	operatorLogger.Info("Creating Deployment")
	err = r.Update(ctx, webhookResources.Deployment)
	if err != nil {
		return ctrl.Result{}, nil
	}
	operatorLogger.Info("Creating Service")
	err = r.Update(ctx, webhookResources.Service)
	if err != nil {
		return ctrl.Result{}, nil
	}

	operatorLogger.Info("Creating MutatingWebhookConfiguration")
	err = r.Create(ctx, webhookResources.MutatingWebhookConfiguration)
	if err != nil {
		return ctrl.Result{}, nil
	}
	operatorLogger.Info("   ----- End of Creating block -----")

	return ctrl.Result{RequeueAfter: time.Duration(30 * time.Second)}, nil
}

func createResources() (webhookResources WebhookResources) {

	var timeoutSeconds int32 = 30
	servicePath := "/mutate/deployments"
	var sideEffect v14.SideEffectClass = "None"
	var operationType v14.OperationType = "CREATE"

	admissionWebhookConfigMap := &v12.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "webhook-config",
			Namespace: "default",
		},
		Data: map[string]string{
			"config.yaml": "general:\n  port: 8443\n  tlsCertFile: /etc/webhook/certs/tls.crt\n  tlsKeyFile: /etc/webhook/certs/tls.key\n  logLevel: debug\ntriggerLabel:\n  notebook-name: \"*\"\npatchData:\n  labels:\n    type-app: \"notebook\"\n  annotations:\n    sidecar.istio.io/componentLogLevel: \"wasm:debug\"\n    sidecar.istio.io/userVolume: \"[{\\\"name\\\":\\\"wasmfilters-dir\\\",\\\"emptyDir\\\": { } } ]\"\n    sidecar.istio.io/userVolumeMount: \"[{\\\"mountPath\\\":\\\"/var/local/lib/wasm-filters\\\",\\\"name\\\":\\\"wasmfilters-dir\\\"}]\"",
		},
	}
	admissionWebhookSecret := &v12.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "aegorov-admission-tls",
			Namespace: "default",
		},
		//Data: map[string][]byte{
		//	"tls.crt": []byte("LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUVOVENDQXgyZ0F3SUJBZ0lVVWdENlVQdkkzV2JqcmY0SWttNE1JTnlDVkNnd0RRWUpLb1pJaHZjTkFRRUwKQlFBd1R6RUxNQWtHQTFVRUJoTUNVbFV4RURBT0JnTlZCQWdUQjBWNFlXMXdiR1V4RHpBTkJnTlZCQWNUQmsxdgpjMk52ZHpFUU1BNEdBMVVFQ2hNSFJYaGhiWEJzWlRFTE1Ba0dBMVVFQ3hNQ1EwRXdIaGNOTWpNd05ERTNNVEF5Ck56QXdXaGNOTkRNd05ERXlNVEF5TnpBd1dqQlBNUXN3Q1FZRFZRUUdFd0pTVlRFUU1BNEdBMVVFQ0JNSFJYaGgKYlhCc1pURVBNQTBHQTFVRUJ4TUdUVzl6WTI5M01SQXdEZ1lEVlFRS0V3ZEZlR0Z0Y0d4bE1Rc3dDUVlEVlFRTApFd0pEUVRDQ0FTSXdEUVlKS29aSWh2Y05BUUVCQlFBRGdnRVBBRENDQVFvQ2dnRUJBTFF4cmc0R1hMYi9uanV5CnU4K283bjV0NTRTZWRXbmJiRDYwQ1Nya2lrYTBpM1ZZSm5LNlNNcTJaMURrMVAvVDd5VlJYcm0yTEg2ejhvY08KcytBNUdmN0xsVUlxdHJSeG1PLzlHaUs3WW43L3Zya004Zk1VczNDeVgyUnFJSUFFY24rYS83QzFNSmVTV1VMcgpKc2J3RXlNYzJYY2JqMVJpa1R6SGJjUlhBZ1NCMGU0ZnpWRWlpSUdKWm5YdzR4Y0RkdlMwZ0o2ZERpNW9FWVZSCmhSem91SlZRQ2ExOEUvZkErNlZnVm1HUXhrbUxtL2JDY0lHaGQyL2hGTXdsT3hOS0o1cUNadm9COFl3VzB1SjAKbFVMNVY1bXFhOXlteFBHZER6ajMxT1FiRnlXY1VudHFaRlVIOGhPK3ZIWUtBRGd1SmlITnJyS2NLSU83YWROegpYUTIwb2c4Q0F3RUFBYU9DQVFjd2dnRURNQTRHQTFVZER3RUIvd1FFQXdJRm9EQWRCZ05WSFNVRUZqQVVCZ2dyCkJnRUZCUWNEQVFZSUt3WUJCUVVIQXdJd0RBWURWUjBUQVFIL0JBSXdBREFkQmdOVkhRNEVGZ1FVYTR6cjdZM1EKTnI0TGhqQTJyRTBMakZUTVROQXdnYVFHQTFVZEVRU0JuRENCbVlJcllXVm5iM0p2ZGkxaFpHMXBjM05wYjI0dQpaR1ZtWVhWc2RDNXpkbU11WTJ4MWMzUmxjaTVzYjJOaGJJSVJZV1ZuYjNKdmRpMWhaRzFwYzNOcGIyNkNKMkZsCloyOXliM1l0WVdSdGFYTnphVzl1TG1SbFptRjFiSFF1WTJ4MWMzUmxjaTVzYjJOaGJJSWRZV1ZuYjNKdmRpMWgKWkcxcGMzTnBiMjR1WkdWbVlYVnNkQzV6ZG1PQ0NXeHZZMkZzYUc5emRJY0Vmd0FBQVRBTkJna3Foa2lHOXcwQgpBUXNGQUFPQ0FRRUFUelZMeURkVU94Z1pyZ2wyZC9RUDhFK0M2S0JVZW43NnN3N2hzcllnaDN4b09aV0w3TWVhCkl4SnZNSjBHZ0NjaklOWnpYSFJVbjRBaTRWUHZtT1hwc01vZU10a1FkeTNlZEl1R3UvQXRSZWc3NVMxdm5ac1gKT0tVdXYwRnNnb1NZbWh4MmtMQUh6TWluTEVTeWVhREZ1eVBudkJVQVkwcEtDS01lODdSQUNLN053V2tpSk12Mwo1eEtkakdmbWJEMk9NUURoamFoZno5WnVsUEV2cnVWeXUyanRjL2NRSEFrUTRCS3dHeWdXZTBJUEg1YTNVTW5ZCkd5elROeTI1K1YzekRrV2VpcUpRdlA2emgzY1dCbDV0Q09mK2JvWDZWVTdHTFh2WVlKOVlYNXdaNFkrSlNNOXQKRHhQc0J5TFZmQytZQ0htMXh1dmpCOGhwT0ZsUHlxdzNGdz09Ci0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0="),
		//	"tls.key": []byte("LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpNSUlFcEFJQkFBS0NBUUVBdERHdURnWmN0ditlTzdLN3o2anVmbTNuaEo1MWFkdHNQclFKS3VTS1JyU0xkVmdtCmNycEl5clpuVU9UVS85UHZKVkZldWJZc2ZyUHlodzZ6NERrWi9zdVZRaXEydEhHWTcvMGFJcnRpZnYrK3VRengKOHhTemNMSmZaR29nZ0FSeWY1ci9zTFV3bDVKWlF1c214dkFUSXh6WmR4dVBWR0tSUE1kdHhGY0NCSUhSN2gvTgpVU0tJZ1lsbWRmRGpGd04yOUxTQW5wME9MbWdSaFZHRkhPaTRsVkFKclh3VDk4RDdwV0JXWVpER1NZdWI5c0p3CmdhRjNiK0VVekNVN0Uwb25tb0ptK2dIeGpCYlM0blNWUXZsWG1hcHIzS2JFOFowUE9QZlU1QnNYSlp4U2UycGsKVlFmeUU3NjhkZ29BT0M0bUljMnVzcHdvZzd0cDAzTmREYlNpRHdJREFRQUJBb0lCQUMzYjR1Y3cyVkc5ZG1ETgpHUjA5YWcwRkhZSFQ3aC9WdHhPeU1BOFpnTk9EeVdaQTIvQWcxcnU0ZGt6UklDQkhxTG8vbmpMM1dSV1o2R1JVCjZwUnJFK0dUb0ZYcGx2d1BXUlBpdjA4Tmo0Q3d4N0pDeUNUTUpPck9pcFo4cCs3TXN2TWsyR1A1aVBkYVNKdE8KUzEwZjVrN3VYYTRCZHBYTVRCaEN6UDNHQXY1d09sQzg4b01CMkVRa0ZJbEJsaUlBQS92R0hJaVBGeFJLZ1dkOAova0NlRzB3WG5zaVlHTmw2cTRiNGV6aVVHZTF6L0JxTUJKRVBOak1MdzJHM1lnTDE3dVdlTHNJQVNVQ29TcFpzClF4MFY1REpWcnB1ZEpvVUFabnRHWUhJTUVva3BKYmg3eGthbU9sVWlKS0ZxNEZtKy81Zk9GZlJqN3FnR1BWSVYKTDFndW8va0NnWUVBMVJlU3RiWVIvb2dJN25qVzVNWSs1WEtmZFpMTURlWVVWdjA1M0ErR3Y5OUJzTE56cnJONAozeUhzK2VQYUU5MEFOOUhaV3lqQWtsSDZqUGV1S1ZjZ3NZb2J0eHhocGdIOVdDcXhjMktscnNDTVNHMUNqOTNXCmJudWk3QmZwRzAxdkExNkRJU2NoUkdIQU9jV0tkR1RsMmNobkhtUERGVzNNTzdSR01iZ0VOdU1DZ1lFQTJIcEoKVXZtWFp6Mm82dFpnQWdsRmh5YXFDd1ZBd3BjcUFCZDlHb0FFSllhdy84QnlUMHRCNFRZN1FsSWFiTGVobzE5WApSUURuU3BrYXlVRDd6dnArWHo5ZDR1MkcwK202ZzNpUEQ3NkRmemdZMUNPMkEzZlZ2U2VuWmJMSkNVRnRiLzdQCiszTFc1bjY4M2pSdGFGeERnUmNWaHN6Rkd2SGh4b2Rpem1TZm8rVUNnWUJtL0wraEp0L0hmb09pamJCK1hQbmsKOXVMdWRnWTg2V2dIS3RlZDdic1lYSlJwRERIcXl6NnR5TDI1Z0UvVHJjbi9NR0syVmhuTUhlYlQzcGpEemlJMAo3Q1M4K1BDUXhRRm1iU3ZhTW1FVTltWldVc2dLdEJLQXp5eE1vcm04d2szVytRU3pMekE2MW11TEFGZ01MUCtSCm8vT0Nrb0NraUs1ZVpLQlFRemwyTlFLQmdRQ0hpTWl2c3FVZ2RuSnoxWlIyc2VkZUhzOEg1MW9NZXloSXRtd1YKVTJGRlBYZEVLUEZvdysyVFc2anVkUWttV0RKVFh6WDhkZnhac0ZJYy94cXBGQnhhOWdtS01yemZvTTZ0MGFXQQpiZjlXZjRETUVTKzhMQ01lTXQyVHhzUW5qMWM5YjRRNElrWjZPWTkyYjh5d01sUHhWc3FiZzBsRS9Yd21HRTI4CmV6T1lJUUtCZ1FDV0JyMmlxdnNNeDRQRlpNemw2ZXh5MWhRZ1lBZFJjNUJTdGFGRXFBZ2dXMzdxM2w1clhuNW0KTzAvOUpXU1FVUGhpL21jQldrbzRoRVdrd1dBc3FhUjNienp5UEF0Ym9Wd20vamMxamJNM1ZBK3hNYmlWSFhUTgpzeHBoL2dIdHN5SHdjY3ZyeDk0ZDFZTjEvb0xPMTRLY3ZLM2Z4UnBWYW1sek41ZTMyb1pvMmc9PQotLS0tLUVORCBSU0EgUFJJVkFURSBLRVktLS0tLQ=="),
		//},
		StringData: map[string]string{
			"tls.crt": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUVOVENDQXgyZ0F3SUJBZ0lVVWdENlVQdkkzV2JqcmY0SWttNE1JTnlDVkNnd0RRWUpLb1pJaHZjTkFRRUwKQlFBd1R6RUxNQWtHQTFVRUJoTUNVbFV4RURBT0JnTlZCQWdUQjBWNFlXMXdiR1V4RHpBTkJnTlZCQWNUQmsxdgpjMk52ZHpFUU1BNEdBMVVFQ2hNSFJYaGhiWEJzWlRFTE1Ba0dBMVVFQ3hNQ1EwRXdIaGNOTWpNd05ERTNNVEF5Ck56QXdXaGNOTkRNd05ERXlNVEF5TnpBd1dqQlBNUXN3Q1FZRFZRUUdFd0pTVlRFUU1BNEdBMVVFQ0JNSFJYaGgKYlhCc1pURVBNQTBHQTFVRUJ4TUdUVzl6WTI5M01SQXdEZ1lEVlFRS0V3ZEZlR0Z0Y0d4bE1Rc3dDUVlEVlFRTApFd0pEUVRDQ0FTSXdEUVlKS29aSWh2Y05BUUVCQlFBRGdnRVBBRENDQVFvQ2dnRUJBTFF4cmc0R1hMYi9uanV5CnU4K283bjV0NTRTZWRXbmJiRDYwQ1Nya2lrYTBpM1ZZSm5LNlNNcTJaMURrMVAvVDd5VlJYcm0yTEg2ejhvY08KcytBNUdmN0xsVUlxdHJSeG1PLzlHaUs3WW43L3Zya004Zk1VczNDeVgyUnFJSUFFY24rYS83QzFNSmVTV1VMcgpKc2J3RXlNYzJYY2JqMVJpa1R6SGJjUlhBZ1NCMGU0ZnpWRWlpSUdKWm5YdzR4Y0RkdlMwZ0o2ZERpNW9FWVZSCmhSem91SlZRQ2ExOEUvZkErNlZnVm1HUXhrbUxtL2JDY0lHaGQyL2hGTXdsT3hOS0o1cUNadm9COFl3VzB1SjAKbFVMNVY1bXFhOXlteFBHZER6ajMxT1FiRnlXY1VudHFaRlVIOGhPK3ZIWUtBRGd1SmlITnJyS2NLSU83YWROegpYUTIwb2c4Q0F3RUFBYU9DQVFjd2dnRURNQTRHQTFVZER3RUIvd1FFQXdJRm9EQWRCZ05WSFNVRUZqQVVCZ2dyCkJnRUZCUWNEQVFZSUt3WUJCUVVIQXdJd0RBWURWUjBUQVFIL0JBSXdBREFkQmdOVkhRNEVGZ1FVYTR6cjdZM1EKTnI0TGhqQTJyRTBMakZUTVROQXdnYVFHQTFVZEVRU0JuRENCbVlJcllXVm5iM0p2ZGkxaFpHMXBjM05wYjI0dQpaR1ZtWVhWc2RDNXpkbU11WTJ4MWMzUmxjaTVzYjJOaGJJSVJZV1ZuYjNKdmRpMWhaRzFwYzNOcGIyNkNKMkZsCloyOXliM1l0WVdSdGFYTnphVzl1TG1SbFptRjFiSFF1WTJ4MWMzUmxjaTVzYjJOaGJJSWRZV1ZuYjNKdmRpMWgKWkcxcGMzTnBiMjR1WkdWbVlYVnNkQzV6ZG1PQ0NXeHZZMkZzYUc5emRJY0Vmd0FBQVRBTkJna3Foa2lHOXcwQgpBUXNGQUFPQ0FRRUFUelZMeURkVU94Z1pyZ2wyZC9RUDhFK0M2S0JVZW43NnN3N2hzcllnaDN4b09aV0w3TWVhCkl4SnZNSjBHZ0NjaklOWnpYSFJVbjRBaTRWUHZtT1hwc01vZU10a1FkeTNlZEl1R3UvQXRSZWc3NVMxdm5ac1gKT0tVdXYwRnNnb1NZbWh4MmtMQUh6TWluTEVTeWVhREZ1eVBudkJVQVkwcEtDS01lODdSQUNLN053V2tpSk12Mwo1eEtkakdmbWJEMk9NUURoamFoZno5WnVsUEV2cnVWeXUyanRjL2NRSEFrUTRCS3dHeWdXZTBJUEg1YTNVTW5ZCkd5elROeTI1K1YzekRrV2VpcUpRdlA2emgzY1dCbDV0Q09mK2JvWDZWVTdHTFh2WVlKOVlYNXdaNFkrSlNNOXQKRHhQc0J5TFZmQytZQ0htMXh1dmpCOGhwT0ZsUHlxdzNGdz09Ci0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0=",
			"tls.key": "LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpNSUlFcEFJQkFBS0NBUUVBdERHdURnWmN0ditlTzdLN3o2anVmbTNuaEo1MWFkdHNQclFKS3VTS1JyU0xkVmdtCmNycEl5clpuVU9UVS85UHZKVkZldWJZc2ZyUHlodzZ6NERrWi9zdVZRaXEydEhHWTcvMGFJcnRpZnYrK3VRengKOHhTemNMSmZaR29nZ0FSeWY1ci9zTFV3bDVKWlF1c214dkFUSXh6WmR4dVBWR0tSUE1kdHhGY0NCSUhSN2gvTgpVU0tJZ1lsbWRmRGpGd04yOUxTQW5wME9MbWdSaFZHRkhPaTRsVkFKclh3VDk4RDdwV0JXWVpER1NZdWI5c0p3CmdhRjNiK0VVekNVN0Uwb25tb0ptK2dIeGpCYlM0blNWUXZsWG1hcHIzS2JFOFowUE9QZlU1QnNYSlp4U2UycGsKVlFmeUU3NjhkZ29BT0M0bUljMnVzcHdvZzd0cDAzTmREYlNpRHdJREFRQUJBb0lCQUMzYjR1Y3cyVkc5ZG1ETgpHUjA5YWcwRkhZSFQ3aC9WdHhPeU1BOFpnTk9EeVdaQTIvQWcxcnU0ZGt6UklDQkhxTG8vbmpMM1dSV1o2R1JVCjZwUnJFK0dUb0ZYcGx2d1BXUlBpdjA4Tmo0Q3d4N0pDeUNUTUpPck9pcFo4cCs3TXN2TWsyR1A1aVBkYVNKdE8KUzEwZjVrN3VYYTRCZHBYTVRCaEN6UDNHQXY1d09sQzg4b01CMkVRa0ZJbEJsaUlBQS92R0hJaVBGeFJLZ1dkOAova0NlRzB3WG5zaVlHTmw2cTRiNGV6aVVHZTF6L0JxTUJKRVBOak1MdzJHM1lnTDE3dVdlTHNJQVNVQ29TcFpzClF4MFY1REpWcnB1ZEpvVUFabnRHWUhJTUVva3BKYmg3eGthbU9sVWlKS0ZxNEZtKy81Zk9GZlJqN3FnR1BWSVYKTDFndW8va0NnWUVBMVJlU3RiWVIvb2dJN25qVzVNWSs1WEtmZFpMTURlWVVWdjA1M0ErR3Y5OUJzTE56cnJONAozeUhzK2VQYUU5MEFOOUhaV3lqQWtsSDZqUGV1S1ZjZ3NZb2J0eHhocGdIOVdDcXhjMktscnNDTVNHMUNqOTNXCmJudWk3QmZwRzAxdkExNkRJU2NoUkdIQU9jV0tkR1RsMmNobkhtUERGVzNNTzdSR01iZ0VOdU1DZ1lFQTJIcEoKVXZtWFp6Mm82dFpnQWdsRmh5YXFDd1ZBd3BjcUFCZDlHb0FFSllhdy84QnlUMHRCNFRZN1FsSWFiTGVobzE5WApSUURuU3BrYXlVRDd6dnArWHo5ZDR1MkcwK202ZzNpUEQ3NkRmemdZMUNPMkEzZlZ2U2VuWmJMSkNVRnRiLzdQCiszTFc1bjY4M2pSdGFGeERnUmNWaHN6Rkd2SGh4b2Rpem1TZm8rVUNnWUJtL0wraEp0L0hmb09pamJCK1hQbmsKOXVMdWRnWTg2V2dIS3RlZDdic1lYSlJwRERIcXl6NnR5TDI1Z0UvVHJjbi9NR0syVmhuTUhlYlQzcGpEemlJMAo3Q1M4K1BDUXhRRm1iU3ZhTW1FVTltWldVc2dLdEJLQXp5eE1vcm04d2szVytRU3pMekE2MW11TEFGZ01MUCtSCm8vT0Nrb0NraUs1ZVpLQlFRemwyTlFLQmdRQ0hpTWl2c3FVZ2RuSnoxWlIyc2VkZUhzOEg1MW9NZXloSXRtd1YKVTJGRlBYZEVLUEZvdysyVFc2anVkUWttV0RKVFh6WDhkZnhac0ZJYy94cXBGQnhhOWdtS01yemZvTTZ0MGFXQQpiZjlXZjRETUVTKzhMQ01lTXQyVHhzUW5qMWM5YjRRNElrWjZPWTkyYjh5d01sUHhWc3FiZzBsRS9Yd21HRTI4CmV6T1lJUUtCZ1FDV0JyMmlxdnNNeDRQRlpNemw2ZXh5MWhRZ1lBZFJjNUJTdGFGRXFBZ2dXMzdxM2w1clhuNW0KTzAvOUpXU1FVUGhpL21jQldrbzRoRVdrd1dBc3FhUjNienp5UEF0Ym9Wd20vamMxamJNM1ZBK3hNYmlWSFhUTgpzeHBoL2dIdHN5SHdjY3ZyeDk0ZDFZTjEvb0xPMTRLY3ZLM2Z4UnBWYW1sek41ZTMyb1pvMmc9PQotLS0tLUVORCBSU0EgUFJJVkFURSBLRVktLS0tLQ==",
		},
	}

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
							Args:  []string{"--config-file", "/etc/webhook/config.yaml"},
							Ports: []v12.ContainerPort{
								{
									Name:          "tls",
									ContainerPort: 8443,
								},
								{
									Name:          "metrics",
									ContainerPort: 80,
								},
							},
							VolumeMounts: []v12.VolumeMount{
								{
									Name:      "webhook-tls-certs",
									ReadOnly:  true,
									MountPath: "/etc/webhook/certs/",
								},
								{
									Name:      "config-volume",
									ReadOnly:  true,
									MountPath: "/etc/webhook/",
								},
							},
						},
					},
					ServiceAccountName: "aegorov-admission-webhook",
					Volumes: []v12.Volume{
						{
							Name: "webhook-tls-certs",
							VolumeSource: v12.VolumeSource{
								Secret: &v12.SecretVolumeSource{
									SecretName: "aegorov-admission-tls",
								},
							},
						},
						{
							Name: "config-volume",
							VolumeSource: v12.VolumeSource{
								ConfigMap: &v12.ConfigMapVolumeSource{
									LocalObjectReference: v12.LocalObjectReference{
										Name: "webhook-config",
									},
								},
							},
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
				},
				{
					Name: "metrics",
					Port: 80,
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
			Name: "aegorov-admission-webhook",
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

	mutationWebhookConfiguration := &v14.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mutation-webhook-configuration",
			Namespace: "default",
		},
		Webhooks: []v14.MutatingWebhook{
			{
				Name: "admission-webhook-service.default.svc",
				ClientConfig: v14.WebhookClientConfig{
					Service: &v14.ServiceReference{
						Namespace: "default",
						Name:      "admission-webhook-service",
						Path:      &servicePath,
					},
					CABundle: []byte("LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURiakNDQWxhZ0F3SUJBZ0lVWHVMYW1NSms5bEhORjZEOGpudkZxZHMxNDhvd0RRWUpLb1pJaHZjTkFRRUwKQlFBd1R6RUxNQWtHQTFVRUJoTUNVbFV4RURBT0JnTlZCQWdUQjBWNFlXMXdiR1V4RHpBTkJnTlZCQWNUQmsxdgpjMk52ZHpFUU1BNEdBMVVFQ2hNSFJYaGhiWEJzWlRFTE1Ba0dBMVVFQ3hNQ1EwRXdIaGNOTWpNd05ERTNNVEF5Ck56QXdXaGNOTWpnd05ERTFNVEF5TnpBd1dqQlBNUXN3Q1FZRFZRUUdFd0pTVlRFUU1BNEdBMVVFQ0JNSFJYaGgKYlhCc1pURVBNQTBHQTFVRUJ4TUdUVzl6WTI5M01SQXdEZ1lEVlFRS0V3ZEZlR0Z0Y0d4bE1Rc3dDUVlEVlFRTApFd0pEUVRDQ0FTSXdEUVlKS29aSWh2Y05BUUVCQlFBRGdnRVBBRENDQVFvQ2dnRUJBTWlGd2dlUGFrdC91dy9RCjl1N21pWmpndG9nWXl3U0xZcTN6c0I0RFQyK0twUEVpbTFVVFRCVHZzVWNnYTk5cUt5bTFxZXF3WWJSa3RIZHUKZGwzOHZTSEY0K0lOYmFpem1mY1hrSTFVV3I4dmFHaHVCc0lRd2lWdm9UODFialFSTk1MbWNma2dYM09BakJQSgo5U2UwSnpjbGY0dUVEd2R4R0xzdnJieElKWGk1UmxZVjZwekFiUUF0UE5pYWc0aExDaFpqY1FmRW1Cc1oyMjBkCncxMDZzeFVmRWplYjZoRWVvYnhjTHdzcTlGY00ySGJXMm8xYmtDY3ZuSm4ySEYzNXRlSmlDbHFBRWczaFpQOEYKOFdQKzNXREVHUVV2eFdWSFJtaEZqb0RNRFdDV1QzWkZZaVQvZU12c1l6c3duM2tpS2dON29jWnhXNXJvN3ZFQQpEeDI4K3ljQ0F3RUFBYU5DTUVBd0RnWURWUjBQQVFIL0JBUURBZ0VHTUE4R0ExVWRFd0VCL3dRRk1BTUJBZjh3CkhRWURWUjBPQkJZRUZQdDUzT0s3R0FMd3RidlVWNXorNXlvN2ZTU1lNQTBHQ1NxR1NJYjNEUUVCQ3dVQUE0SUIKQVFBWnhwcEs0eXhSVW1sckM5Q3BDR3M0VzR5LzNmTUtCVTRPT2Y3Q1R3VlIzQVc2bEdUbXYvTitCM01Wd3d1OQpESjJsYmhiTUhpSGdnMUdSd1IvN0t5T2FmeFQ0YkNPUG9NUjBZRU1Db0J6R3pRNHIvRUp1aStyRk03RGY1Mnh2ClJSOHprTnJIcXk5KzB1b1JackltRnNqYWNKOFBhUEdsZmV0eWFISEVGTDFNSXZkeGtEZ0xGYVBlcTZBaVJhT3AKd0VZenVhUnRsRDNUbVVSSXN1Yk9tN3Bpc1lITUN6NFdUeERzd2QzVU9OUGxpQlU2ZkVsZzZkNVFlMDVyWGc4YQpTMURnQThFKzRJZHkralZ0dk9YcmJTa3d0RDRYY2w3RWppQXk5TVY5TGlnelNCdmFReXlvb3FGR3FMYU43WDV3CkNtZVlxLzNHNUpVb2wzR3J5bENkekNDUQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0t"),
				},
				Rules: []v14.RuleWithOperations{
					{
						Operations: []v14.OperationType{operationType},
						Rule: v14.Rule{
							APIGroups:   []string{"apps"},
							APIVersions: []string{"v1"},
							Resources:   []string{"statefulsets"},
						},
					},
				},
				ObjectSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"example-webhook-enabled": "true"},
				},
				TimeoutSeconds: &timeoutSeconds,
				SideEffects:    &sideEffect,
				// TODO: Не слетит ли тут указатель на sideEffect при завершении метода?
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
		Secret:                       admissionWebhookSecret,
		ConfigMap:                    admissionWebhookConfigMap,
	}

	return
}

// SetupWithManager sets up the controller with the Manager.
func (r *WebhookMutatorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&apiv1alpha1.WebhookMutator{}).
		Complete(r)
}
