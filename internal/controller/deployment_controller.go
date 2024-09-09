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
	corev1 "k8s.io/api/core/v1"

	"context"
	"encoding/json"
	"fmt"

	karmadapolicyv1alpha1 "github.com/karmada-io/karmada/pkg/apis/policy/v1alpha1" // Import cho FederatedResourceQuota
	appsv1 "k8s.io/api/apps/v1"                                                    // Import cho Deployment API
	"k8s.io/apimachinery/pkg/api/resource"
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

	// Gọi hàm để lấy thông tin Deployment và in ra console
	deploymentInfo, err := getDeploymentInfo(deployment)
	if err != nil {
		log.Error(err, "unable to get deployment info")
		return ctrl.Result{}, err
	}
	fmt.Printf("Deployment Info: %s\n", deploymentInfo)

	// Gọi hàm để lấy thông tin Resource Quota của từng Cluster và in ra console
	clusterQuotaInfo, err := getClusterQuotaInfo(ctx, deployment, r.Client)
	if err != nil {
		log.Error(err, "unable to get cluster quota info")
		return ctrl.Result{}, err
	}
	fmt.Printf("Cluster Quota Info: %s\n", clusterQuotaInfo)

	if err := adjustQuotaBasedOnDeployment(ctx, deployment, r.Client); err != nil {
		log.Error(err, "unable to adjust and update cluster quota based on deployment")
		return ctrl.Result{}, err
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

// getDeploymentInfo trả về thông tin của Deployment dưới dạng JSON
func getDeploymentInfo(deployment appsv1.Deployment) (string, error) {
	// Lấy thông tin yêu cầu CPU và Memory của container đầu tiên trong Deployment
	container := deployment.Spec.Template.Spec.Containers[0]

	// Lấy CPU request
	cpuRequest := "0"
	if container.Resources.Requests.Cpu() != nil {
		cpuRequest = container.Resources.Requests.Cpu().String()
	}

	// Lấy Memory request
	memoryRequest := "0"
	if container.Resources.Requests.Memory() != nil {
		memoryRequest = container.Resources.Requests.Memory().String()
	}

	// Chuẩn bị cấu trúc JSON để chứa thông tin Deployment
	deploymentInfo := map[string]interface{}{
		"Deployment Name": deployment.Name,
		"Namespace":       deployment.Namespace,
		"Container": map[string]string{
			"Container Name": container.Name,
			"CPU Request":    cpuRequest,
			"Memory Request": memoryRequest,
		},
	}

	// Chuyển đổi cấu trúc dữ liệu thành JSON
	jsonResult, err := json.MarshalIndent(deploymentInfo, "", "  ")
	if err != nil {
		return "", err
	}

	// Trả về JSON
	return string(jsonResult), nil
}

// getClusterQuotaInfo trả về thông tin Resource Quota của từng cluster dưới dạng JSON
func getClusterQuotaInfo(ctx context.Context, deployment appsv1.Deployment, c client.Client) (string, error) {
	log := log.FromContext(ctx)

	// Lấy thông tin về FederatedResourceQuota dựa trên namespace của Deployment
	var federatedResourceQuotaList karmadapolicyv1alpha1.FederatedResourceQuotaList
	if err := c.List(ctx, &federatedResourceQuotaList, client.InNamespace(deployment.Namespace)); err != nil {
		log.Error(err, "unable to list FederatedResourceQuotas")
		return "", err
	}

	// Chuẩn bị cấu trúc để chứa thông tin ResourceQuota cho từng Cluster
	result := map[string]interface{}{
		"clusterStatus": []map[string]string{},
	}

	// Duyệt qua tất cả các FederatedResourceQuota trong namespace của Deployment
	for _, frq := range federatedResourceQuotaList.Items {
		// Thêm các thông tin aggregatedStatus
		for _, status := range frq.Status.AggregatedStatus {
			hardCpu := status.Hard["cpu"]
			usedCpu := status.Used["cpu"]
			hardMemory := status.Hard["memory"]
			usedMemory := status.Used["memory"]

			// Thêm thông tin cluster vào kết quả
			result["clusterStatus"] = append(result["clusterStatus"].([]map[string]string), map[string]string{
				"Cluster Name": status.ClusterName,
				"Hard CPU":     hardCpu.String(),
				"Used CPU":     usedCpu.String(),
				"Hard Memory":  hardMemory.String(),
				"Used Memory":  usedMemory.String(),
			})
		}
	}

	// Chuyển đổi cấu trúc dữ liệu thành JSON
	jsonResult, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}

	// Trả về JSON
	return string(jsonResult), nil
}

// adjustQuotaBasedOnDeployment điều chỉnh FederatedResourceQuota dựa trên yêu cầu tài nguyên của Deployment
func adjustQuotaBasedOnDeployment(ctx context.Context, deployment appsv1.Deployment, c client.Client) error {
	log := log.FromContext(ctx)

	// Lấy danh sách FederatedResourceQuota trong namespace của Deployment
	var federatedResourceQuotaList karmadapolicyv1alpha1.FederatedResourceQuotaList
	if err := c.List(ctx, &federatedResourceQuotaList, client.InNamespace(deployment.Namespace)); err != nil {
		log.Error(err, "unable to list FederatedResourceQuotas")
		return err
	}

	// Lấy yêu cầu CPU và Memory từ Deployment
	container := deployment.Spec.Template.Spec.Containers[0]
	cpuRequest := container.Resources.Requests[corev1.ResourceCPU]
	memoryRequest := container.Resources.Requests[corev1.ResourceMemory]

	for _, frq := range federatedResourceQuotaList.Items {
		quotaUpdated := false // Cờ để kiểm tra xem quota có được cập nhật hay không

		// Duyệt qua từng cluster trong staticAssignments của FederatedResourceQuota
		for i, assignment := range frq.Spec.StaticAssignments {
			hardCpu := assignment.Hard[corev1.ResourceCPU]
			hardMemory := assignment.Hard[corev1.ResourceMemory]

			// So sánh yêu cầu CPU và Memory của Deployment với quota hiện tại
			if cpuRequest.Cmp(hardCpu) > 0 || memoryRequest.Cmp(hardMemory) > 0 {
				// Điều chỉnh quota tài nguyên cho cluster này
				frq.Spec.StaticAssignments[i].Hard[corev1.ResourceCPU] = resource.MustParse(cpuRequest.String())
				frq.Spec.StaticAssignments[i].Hard[corev1.ResourceMemory] = resource.MustParse(memoryRequest.String())

				log.Info("Adjusted FederatedResourceQuota for cluster", "cluster", assignment.ClusterName, "new CPU request", cpuRequest.String(), "new Memory request", memoryRequest.String())

				quotaUpdated = true
			}
		}

		// Nếu quota đã được điều chỉnh, cập nhật lại FederatedResourceQuota
		if quotaUpdated {
			if err := c.Update(ctx, &frq); err != nil {
				log.Error(err, "unable to update FederatedResourceQuota", "namespace", frq.Namespace, "name", frq.Name)
				return err
			}
			log.Info("Successfully updated FederatedResourceQuota", "name", frq.Name, "namespace", frq.Namespace)
		}
	}

	return nil
}
