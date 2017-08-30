package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

func TestNewControllerWithFakes(t *testing.T) {
	config := getDefaultConfig()

	initialActiveNamespaceNames := []string{"ns-0", "ns-1"}
	k8sClient := NewFakeK8SClient(initialActiveNamespaceNames)
	nsInformer := NewFakeSharedInformer()
	prometheusRegistry := prometheus.NewRegistry()
	ecrClient := &FakeECRClient{}

	ctrl, err := newController(config, k8sClient, nsInformer, prometheusRegistry, ecrClient)

	assert.Nil(t, err, "New controller")
	assert.Equal(t, k8sClient, ctrl.K8S, "Controller.K8S")
	assert.Equal(t, ecrClient, ctrl.ECR, "Controller.ECR")
}

func TestRunControllerWithFakesNoInformerResync(t *testing.T) {
	nonBlacklistNamespaceNames := "ns-0, ns-1, ns-2, ns-3"
	initialActiveNamespaceNames := strings.Split(defaultNamespaceBlacklist+nonBlacklistNamespaceNames, ", ")

	config := getDefaultConfig()
	k8sClient := NewFakeK8SClient(initialActiveNamespaceNames)
	nsInformer := NewFakeSharedInformer()
	prometheusRegistry := prometheus.NewRegistry()
	ecrClient := NewFakeECRClient()

	ctx, cancel := context.WithCancel(context.Background())
	ctrl, _ := newController(config, k8sClient, nsInformer, prometheusRegistry, ecrClient)
	go ctrl.Run(ctx.Done())

	// Simulate informers initial add events
	nss, _ := k8sClient.GetActiveNamespaceNames()
	for _, ns := range nss {
		nsInformer.SimulateAddNamespace(ns)
	}

	time.Sleep(500 * time.Millisecond)
	cancel()

	expectedCount := len(strings.Split(nonBlacklistNamespaceNames, ", "))
	actualCount := k8sClient.CreatedSecretCount()
	assert.Equal(t, expectedCount, actualCount, "Expected secret creation count of %d, but got %d", expectedCount, actualCount)
	secrets := k8sClient.Secrets()
	assert.Equal(t, actualCount, len(secrets), "Expected secret count of %d, but got %d", actualCount, len(secrets))
}

func TestRunControllerWithFakesNoInformerResyncButNewNamespaceAdded(t *testing.T) {
	nonBlacklistNamespaceNames := "ns-0, ns-1, ns-2, ns-3"
	initialActiveNamespaceNames := strings.Split(defaultNamespaceBlacklist+nonBlacklistNamespaceNames, ", ")

	config := getDefaultConfig()
	k8sClient := NewFakeK8SClient(initialActiveNamespaceNames)
	nsInformer := NewFakeSharedInformer()
	prometheusRegistry := prometheus.NewRegistry()
	ecrClient := NewFakeECRClient()

	ctx, cancel := context.WithCancel(context.Background())
	ctrl, _ := newController(config, k8sClient, nsInformer, prometheusRegistry, ecrClient)
	go ctrl.Run(ctx.Done())

	// Simulate informers initial add events
	nss, _ := k8sClient.GetActiveNamespaceNames()
	for _, ns := range nss {
		nsInformer.SimulateAddNamespace(ns)
	}

	newNamespace := "ns-4"
	nsInformer.SimulateAddNamespace(newNamespace)

	time.Sleep(1500 * time.Millisecond)
	cancel()

	expectedCount := len(strings.Split(nonBlacklistNamespaceNames, ", ")) + 1
	actualCount := k8sClient.CreatedSecretCount()
	assert.Equal(t, expectedCount, actualCount, "Expected secret creation count of %d, but got %d", expectedCount, actualCount)
	secrets := k8sClient.Secrets()
	assert.Equal(t, actualCount, len(secrets), "Expected secret count of %d, but got %d", actualCount, len(secrets))

	newSecret, err := k8sClient.GetSecret(newNamespace, ecrClient.DomainName)
	assert.Nil(t, err, "New secret not found")
	assert.NotNil(t, newSecret, "New secret is nil")
}

func TestRunControllerWithFakesAndAllowTwoAuthenticationRenewals(t *testing.T) {
	nonBlacklistNamespaceNames := "ns-0, ns-1, ns-2, ns-3"
	initialActiveNamespaceNames := strings.Split(defaultNamespaceBlacklist+nonBlacklistNamespaceNames, ", ")

	config := getDefaultConfig()
	config.AuthenticationTokenRenewalInterval = 500 * time.Millisecond
	k8sClient := NewFakeK8SClient(initialActiveNamespaceNames)
	nsInformer := NewFakeSharedInformer()
	prometheusRegistry := prometheus.NewRegistry()
	ecrClient := NewFakeECRClient()

	ctx, cancel := context.WithCancel(context.Background())
	ctrl, _ := newController(config, k8sClient, nsInformer, prometheusRegistry, ecrClient)
	go ctrl.Run(ctx.Done())

	// Simulate informers initial add events
	nss, _ := k8sClient.GetActiveNamespaceNames()
	for _, ns := range nss {
		nsInformer.SimulateAddNamespace(ns)
	}

	time.Sleep(900 * time.Millisecond)
	cancel()

	// Will get intial secret creation and have allowed time for one addtional renewal
	expectedCount := len(strings.Split(nonBlacklistNamespaceNames, ", "))
	actualCount := k8sClient.CreatedSecretCount()
	assert.Equal(t, expectedCount, actualCount, "Expected secret creation count of %d, but got %d", expectedCount, actualCount)
	actualCount = k8sClient.UpdatedSecretCount()
	assert.Equal(t, expectedCount, actualCount, "Expected secret update count of %d, but got %d", expectedCount, actualCount)
	secrets := k8sClient.Secrets()
	assert.Equal(t, actualCount, len(secrets), "Expected secret count of %d, but got %d", actualCount, len(secrets))
}
