/*
Copyright 2024.

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

package controller

import (
	"context"
	karmadapolicyv1alpha1 "github.com/karmada-io/karmada/pkg/apis/policy/v1alpha1" // Import cho FederatedResourceQuota
	appsv1 "k8s.io/api/apps/v1"                                                    // Import cho Deployment API
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log" // Import log từ controller-runtime
)

// DeploymentReconciler reconciles a Deployment object
type DeploymentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=resourcequotascaling.trhthang.com,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=resourcequotascaling.trhthang.com,resources=deployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=resourcequotascaling.trhthang.com,resources=deployments/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Deployment object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *DeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Lấy thông tin về Deployment
	var deployment appsv1.Deployment
	if err := r.Get(ctx, req.NamespacedName, &deployment); err != nil {
		log.Error(err, "unable to fetch Deployment")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// In ra thông tin về tên và namespace của Deployment
	log.Info("Reconciling Deployment", "Deployment Name", deployment.Name, "Namespace", deployment.Namespace)

	// Gọi hàm để in ra thông tin ResourceQuota của Deployment
	if err := printFederatedResourceQuota(ctx, deployment, r.Client); err != nil {
		log.Error(err, "unable to print FederatedResourceQuotas")
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// return ctrl.NewControllerManagedBy(mgr).
	// 	// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
	// 	// For().
	// 	Complete(r)

	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1.Deployment{}). // Chỉ định controller theo dõi Deployment
		Complete(r)
}

// Hàm lấy danh sách ResourceQuota liên quan đến Deployment và in ra thông tin chi tiết
func printFederatedResourceQuota(ctx context.Context, deployment appsv1.Deployment, c client.Client) error {
	log := log.FromContext(ctx)

	// Lấy thông tin về FederatedResourceQuota dựa trên namespace của Deployment
	var federatedResourceQuotaList karmadapolicyv1alpha1.FederatedResourceQuotaList
	if err := c.List(ctx, &federatedResourceQuotaList, client.InNamespace(deployment.Namespace)); err != nil {
		log.Error(err, "unable to list FederatedResourceQuotas")
		return err
	}

	// Duyệt qua tất cả các FederatedResourceQuota trong namespace của Deployment
	for _, frq := range federatedResourceQuotaList.Items {
		log.Info("Federated ResourceQuota", "name", frq.Name, "namespace", frq.Namespace)

		// In thông tin tổng thể
		log.Info("Overall Quota", "CPU", frq.Spec.Overall.Cpu, "Used CPU", frq.Status.OverallUsed.Cpu)

		// In thông tin chi tiết về staticAssignments và aggregatedStatus
		for _, assignment := range frq.Spec.StaticAssignments {
			log.Info("Cluster Assignment", "Cluster Name", assignment.ClusterName, "Hard CPU", assignment.Hard["cpu"])
		}

		for _, status := range frq.Status.AggregatedStatus {
			log.Info("Cluster Usage", "Cluster Name", status.ClusterName, "Hard CPU", status.Hard["cpu"], "Used CPU", status.Used["cpu"])
		}
	}
	return nil
}
