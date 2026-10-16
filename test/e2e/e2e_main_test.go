package test

import (
	"context"
	"fmt"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"

	"sigs.k8s.io/secrets-store-sync-controller/test/e2e/apivalidations"
	"sigs.k8s.io/secrets-store-sync-controller/test/e2e/controller"
	e2elib "sigs.k8s.io/secrets-store-sync-controller/test/e2e/library"
)

var tests = map[string]func(t *testing.T, f *e2elib.Framework){
	"SecretSyncsFailures":                      controller.TestSecretSyncsFailures,
	"SecretSyncSuccess":                        controller.TestSecretSyncSuccess,
	"ControllerResync":                         controller.TestControllerResync,
	"Policies/CreateUpdateSecretsTypes-Create": controller.TestCreateUpdateSecretsTypes_Create,
	"Policies/CreateUpdateSecretsTypes-Patch":  controller.TestCreateUpdateSecretsTypes_Patch,
	"Policies/DeleteSecrets":                   controller.TestDeleteSecrets,
	"Policies/UpdateOwnersCheckOldObject":      controller.TestUpdateOwnersCheckOldObject,
	"Policies/UpdateSecretsBasedOnLabel":       controller.TestUpdateSecretsBasedOnLabel,
	"Policies/ValidateTokenConfig":             controller.TestValidateTokenConfig,
	"APIValidation":                            apivalidations.TestAPIValidation,
}

func checkCRDEstablished(t *testing.T, f *e2elib.Framework, name string) {
	t.Helper()

	crds := f.Clients().Dynamic().Resource(schema.GroupVersionResource{
		Group:    "apiextensions.k8s.io",
		Version:  "v1",
		Resource: "customresourcedefinitions",
	})
	err := wait.PollUntilContextTimeout(t.Context(), 2*time.Second, time.Minute, true, func(ctx context.Context) (bool, error) {
		crd, err := crds.Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("get CRD %q: %w", name, err)
		}

		conditions, found, err := unstructured.NestedSlice(crd.Object, "status", "conditions")
		if err != nil {
			return false, fmt.Errorf("read CRD %q conditions: %w", name, err)
		}
		if !found {
			return false, nil
		}
		for _, raw := range conditions {
			condition, ok := raw.(map[string]interface{})
			if !ok {
				return false, fmt.Errorf("CRD %q has invalid condition %T", name, raw)
			}
			conditionType, ok := condition["type"].(string)
			if !ok {
				return false, fmt.Errorf("CRD %q condition has invalid type", name)
			}
			if conditionType != "Established" {
				continue
			}
			conditionStatus, ok := condition["status"].(string)
			if !ok {
				return false, fmt.Errorf("CRD %q Established condition has invalid status", name)
			}
			return conditionStatus == string(metav1.ConditionTrue), nil
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("wait for CRD %q Established=True: %v", name, err)
	}
	if _, err := crds.Get(t.Context(), name, metav1.GetOptions{}); err != nil {
		t.Fatalf("get CRD %q after establishment: %v", name, err)
	}
}

func Test(t *testing.T) {
	f := e2elib.NewFramework(t)

	t.Run("SecretProviderClass CRD is established", func(t *testing.T) {
		checkCRDEstablished(t, f, "secretproviderclasses.secrets-store.csi.x-k8s.io")
	})
	t.Run("SecretSync CRD is established", func(t *testing.T) {
		checkCRDEstablished(t, f, "secretsyncs.secret-sync.x-k8s.io")
	})
	t.Run("controller ClusterRole and ClusterRoleBinding exist", func(t *testing.T) {
		rbac := f.Clients().KubeClients().RbacV1()
		if _, err := rbac.ClusterRoles().Get(t.Context(), "secrets-store-sync-controller-role", metav1.GetOptions{}); err != nil {
			t.Fatalf("get ClusterRole secrets-store-sync-controller-role: %v", err)
		}
		if _, err := rbac.ClusterRoleBindings().Get(t.Context(), "secrets-store-sync-controller-rolebinding", metav1.GetOptions{}); err != nil {
			t.Fatalf("get ClusterRoleBinding secrets-store-sync-controller-rolebinding: %v", err)
		}
	})

	for testName, testRunner := range tests {
		t.Run(testName, func(t *testing.T) {
			testRunner(t, f)
		})
	}
}
