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
	"encoding/json"
	"fmt"
	"time"

	karmadapolicyv1alpha1 "github.com/karmada-io/karmada/pkg/apis/policy/v1alpha1" // Import cho PropagationPolicy
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

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy.karmada.io,resources=propagationpolicies,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
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

	// Gọi hàm để lấy thông tin PropagationPolicy tương ứng
	propagationPolicy, err := r.getPropagationPolicyForDeployment(ctx, deployment)
	if err != nil {
		log.Error(err, "unable to fetch PropagationPolicy")
		return ctrl.Result{}, err
	}

	// Lấy danh sách các cluster từ PropagationPolicy
	if propagationPolicy != nil {
		clusterNames := propagationPolicy.Spec.Placement.ClusterAffinity.ClusterNames
		fmt.Printf("Cluster Names: %v\n", clusterNames)

		// Gọi hàm để lấy thông tin Resource Quota của các cluster
		clusterQuotaInfoJson, err := getClusterQuotaInfo(ctx, deployment, clusterNames, r.Client)
		if err != nil {
			log.Error(err, "unable to fetch cluster quota info")
			return ctrl.Result{}, err
		}
		fmt.Printf("Cluster Quota Info: %s\n", clusterQuotaInfoJson)

		// Giải mã JSON thành map[string]interface{}
		var clusterQuotaInfo map[string]interface{}
		if err := json.Unmarshal([]byte(clusterQuotaInfoJson), &clusterQuotaInfo); err != nil {
			log.Error(err, "unable to unmarshal cluster quota info")
			return ctrl.Result{}, err
		}

		// Gọi hàm adjustResourceQuota để điều chỉnh resource quota cho các cluster
		err = adjustResourceQuota(ctx, deployment, clusterQuotaInfo, r.Client)
		if err != nil {
			log.Error(err, "unable to adjust resource quota")
			return ctrl.Result{}, err
		}
	} else {
		fmt.Println("No matching PropagationPolicy found")
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
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

// getPropagationPolicyForDeployment tìm PropagationPolicy tương ứng với Deployment
func (r *DeploymentReconciler) getPropagationPolicyForDeployment(ctx context.Context, deployment appsv1.Deployment) (*karmadapolicyv1alpha1.PropagationPolicy, error) {
	// Tạo danh sách PropagationPolicy để tìm kiếm
	var propagationPolicyList karmadapolicyv1alpha1.PropagationPolicyList

	// Lấy danh sách tất cả các PropagationPolicy trong cùng namespace với Deployment
	if err := r.List(ctx, &propagationPolicyList, &client.ListOptions{Namespace: deployment.Namespace}); err != nil {
		return nil, err
	}

	// Tìm PropagationPolicy có resourceSelector khớp với Deployment
	for _, policy := range propagationPolicyList.Items {
		for _, selector := range policy.Spec.ResourceSelectors {
			if selector.Kind == "Deployment" && selector.Name == deployment.Name {
				// Trả về PropagationPolicy tìm được
				return &policy, nil
			}
		}
	}

	// Nếu không tìm thấy, trả về nil
	return nil, nil
}

// getClusterQuotaInfo trả về thông tin Resource Quota của từng cluster trong danh sách clusterNames dưới dạng JSON
func getClusterQuotaInfo(ctx context.Context, deployment appsv1.Deployment, clusterNames []string, c client.Client) (string, error) {
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
			// Chỉ lấy thông tin resource quota cho các cluster nằm trong danh sách clusterNames
			if contains(clusterNames, status.ClusterName) {
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
	}

	// Chuyển đổi cấu trúc dữ liệu thành JSON
	jsonResult, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}

	// Trả về JSON
	return string(jsonResult), nil
}

// contains kiểm tra xem một chuỗi có nằm trong slice hay không
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// adjustResourceQuota điều chỉnh resource quota cho các cluster để đáp ứng resource requirement của deployment
func adjustResourceQuota(ctx context.Context, deployment appsv1.Deployment, clusterQuotaInfo map[string]interface{}, c client.Client) error {
	log := log.FromContext(ctx)

	// Lấy yêu cầu về CPU và Memory của Deployment
	container := deployment.Spec.Template.Spec.Containers[0]
	requiredCPU := container.Resources.Requests.Cpu()
	requiredMemory := container.Resources.Requests.Memory()

	// Duyệt qua từng cluster trong clusterQuotaInfo và kiểm tra điều chỉnh quota nếu cần
	clusterStatus, ok := clusterQuotaInfo["clusterStatus"].([]interface{})
	if !ok {
		return fmt.Errorf("unexpected data format for clusterStatus")
	}

	for _, clusterStatusItem := range clusterStatus {
		clusterMap, ok := clusterStatusItem.(map[string]interface{})
		if !ok {
			return fmt.Errorf("unexpected data format in cluster status item")
		}

		clusterName := clusterMap["Cluster Name"].(string)
		hardCPU := resource.MustParse(clusterMap["Hard CPU"].(string))
		usedCPU := resource.MustParse(clusterMap["Used CPU"].(string))
		hardMemory := resource.MustParse(clusterMap["Hard Memory"].(string))
		usedMemory := resource.MustParse(clusterMap["Used Memory"].(string))

		// Tính toán tài nguyên còn lại (hard - used)
		availableCPU := hardCPU.DeepCopy()
		availableCPU.Sub(usedCPU)

		availableMemory := hardMemory.DeepCopy()
		availableMemory.Sub(usedMemory)

		// Kiểm tra nếu tài nguyên còn lại không đủ đáp ứng resource requirement
		if availableCPU.Cmp(*requiredCPU) < 0 || availableMemory.Cmp(*requiredMemory) < 0 {
			// Kiểm tra xem resource quota đã được cập nhật chưa, nếu cập nhật đủ rồi thì không cần điều chỉnh
			if hardCPU.Cmp(*requiredCPU) >= 0 && hardMemory.Cmp(*requiredMemory) >= 0 {
				fmt.Printf("Resource quota for cluster %s is already sufficient, no update needed.\n", clusterName)
				continue
			}

			// Tính toán lượng tài nguyên cần bổ sung
			cpuToAdd := requiredCPU.DeepCopy()
			cpuToAdd.Sub(availableCPU) // Chỉ thêm phần thiếu

			memoryToAdd := requiredMemory.DeepCopy()
			memoryToAdd.Sub(availableMemory) // Chỉ thêm phần thiếu

			// Tạo đối tượng FederatedResourceQuota để điều chỉnh
			var frq karmadapolicyv1alpha1.FederatedResourceQuota
			if err := c.Get(ctx, client.ObjectKey{Namespace: deployment.Namespace, Name: deployment.Namespace}, &frq); err != nil {
				log.Error(err, "unable to fetch FederatedResourceQuota")
				return err
			}

			// Tìm staticAssignments tương ứng với clusterName
			for i, assignment := range frq.Spec.StaticAssignments {
				if assignment.ClusterName == clusterName {
					// Điều chỉnh CPU và Memory trong quota cho cluster đó
					newHardCPU := hardCPU.DeepCopy()
					newHardCPU.Add(cpuToAdd) // Thêm phần thiếu hụt

					newHardMemory := hardMemory.DeepCopy()
					newHardMemory.Add(memoryToAdd) // Thêm phần thiếu hụt

					frq.Spec.StaticAssignments[i].Hard["cpu"] = newHardCPU
					frq.Spec.StaticAssignments[i].Hard["memory"] = newHardMemory

					// Cập nhật FederatedResourceQuota
					if err := c.Update(ctx, &frq); err != nil {
						log.Error(err, "unable to update FederatedResourceQuota")
						return err
					}

					fmt.Printf("Adjusted resource quota for cluster %s: new CPU = %s, new Memory = %s\n", clusterName, newHardCPU.String(), newHardMemory.String())

					// Tạm dừng một khoảng thời gian ngắn để hệ thống đồng bộ lại trạng thái
					time.Sleep(5 * time.Second)

					// Lấy lại thông tin mới từ API sau khi cập nhật (không cần truyền ListOptions)
					if err := c.Get(ctx, client.ObjectKey{Namespace: deployment.Namespace, Name: deployment.Namespace}, &frq); err != nil {
						log.Error(err, "unable to fetch updated FederatedResourceQuota directly from API server")
						return err
					}
					break
				}
			}
		} else {
			fmt.Printf("No adjustment needed for cluster %s: available CPU and Memory are sufficient.\n", clusterName)
		}
	}

	return nil
}
