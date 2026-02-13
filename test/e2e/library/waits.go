package library

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	pollingInterval       = 2 * time.Second
	controllerSyncTimeout = 300 * time.Second
)

// WaitForSecretSyncCondition waits until the secretsync object at ssNamespace/ssName contains the condition from `cond`.
//
// The matching of the condition is based on (Type,Reason,Status,MesssagePrefix) from the `cond` argument, where MessagePrefix is the cond.Message.
// Prefix matching of the condition Message makes it possible to match conditons when e.g. namespace that varies is a part of the sought condition.Message.
func WaitForSecretSyncCondition(testCfg *TestConfig, ssNamespace, ssName string, cond *metav1.Condition) {
	const waitContinuesFmt = "continuing to wait for SecretSync condition because: %v"
	t := testCfg.T

	var lastWaitReason string
	err := wait.PollUntilContextTimeout(testCfg.Context, pollingInterval, controllerSyncTimeout, true, func(waitCtx context.Context) (bool, error) {
		ss, err := testCfg.Clients.SSClient().SecretSyncV1alpha1().SecretSyncs(ssNamespace).Get(waitCtx, ssName, metav1.GetOptions{})
		if err != nil {
			if lastWaitReason != err.Error() {
				lastWaitReason = err.Error()
				t.Logf(waitContinuesFmt, lastWaitReason)
			}
			return false, nil
		}

		foundCond := meta.FindStatusCondition(ss.Status.Conditions, cond.Type)
		if foundCond == nil {
			reason := fmt.Sprintf("condition of type %q not yet found in %v", cond.Type, ss.Status.Conditions)
			if lastWaitReason != reason {
				lastWaitReason = reason
				t.Logf(waitContinuesFmt, lastWaitReason)
			}
			return false, nil
		}

		if foundCond.Reason != cond.Reason ||
			foundCond.Status != cond.Status ||
			!strings.HasPrefix(foundCond.Message, cond.Message) {

			reason := fmt.Sprintf("condition of type %q was found but expected values don't match %v != %v", cond.Type, cond, foundCond)
			if lastWaitReason != reason {
				lastWaitReason = reason
				t.Logf(waitContinuesFmt, lastWaitReason)
			}
			return false, nil
		}

		return true, nil
	})

	if err != nil {
		t.Fatalf("failed to wait for SecretSync %s/%s condition: %v", ssNamespace, ssName, err)
	}
}

func WaitForSecret(testCfg *TestConfig, secret *corev1.Secret) {
	const waitContinuesFmt = "continuing to wait for Secret condition because: %v"

	var lastWaitReason string
	err := wait.PollUntilContextTimeout(testCfg.Context, pollingInterval, controllerSyncTimeout, true, func(waitCtx context.Context) (bool, error) {
		s, err := testCfg.Clients.kubeClients.CoreV1().Secrets(secret.Namespace).Get(waitCtx, secret.Name, metav1.GetOptions{})
		if err != nil {
			if lastWaitReason != err.Error() {
				lastWaitReason = err.Error()
				testCfg.T.Logf(waitContinuesFmt, lastWaitReason)
			}
			return false, nil
		}

		if labelDiff := cmp.Diff(secret.Labels, s.Labels); len(labelDiff) != 0 {
			return false, fmt.Errorf("labels differ: %s", labelDiff)
		}
		if annotationDiff := cmp.Diff(secret.Annotations, s.Annotations); len(annotationDiff) != 0 {
			return false, fmt.Errorf("annotations differ: %s", annotationDiff)
		}
		if ownersDiff := cmp.Diff(secret.OwnerReferences, s.OwnerReferences); len(ownersDiff) != 0 {
			return false, fmt.Errorf("ownerReferences differ: %s", ownersDiff)
		}
		if dataDiff := cmp.Diff(secret.Data, s.Data); len(dataDiff) != 0 {
			return false, fmt.Errorf("secret.data differ: %s", dataDiff)
		}
		return true, nil
	})

	if err != nil {
		testCfg.T.Fatalf("failed to wait for Secret %s/%s: %v", secret.Namespace, secret.Name, err)
	}
}
