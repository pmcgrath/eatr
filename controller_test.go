package main

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ecr1 = "123456789012.dkr.ecr.eu-west-1.amazonaws.com"
	ecr2 = "444456781111.dkr.ecr.us-east-1.amazonaws.com"
	ecr3 = "444456781111.dkr.ecr.ap-southeast-2.amazonaws.com"
	ns1  = "ns-1"
	ns2  = "ns-2"
	ns3  = "ns-3"
	ns4  = "ns-4"
)

func TestNewControllerWithFakes(t *testing.T) {
	config := getDefaultConfig()
	k8sClient := NewFakeK8SClient([]FakeK8SClientSeedNamespace{
		FakeK8SClientSeedNamespace{
			Name:     config.HostNamespace,
			IsActive: true,
			Labels:   map[string]string{},
			Secrets:  []string{},
		},
	})
	nsInformer := NewFakeSharedInformer()
	prometheusRegistry := prometheus.NewRegistry()
	ecrClient := &FakeECRClient{}

	ctrl, err := newController(config, k8sClient, nsInformer, prometheusRegistry, ecrClient)

	assert.Nil(t, err, "New controller")
	assert.Equal(t, k8sClient, ctrl.K8S, "Controller.K8S")
	assert.Equal(t, ecrClient, ctrl.ECR, "Controller.ECR")
}

func TestRunController(t *testing.T) {
	config := getDefaultConfig()
	for _, tc := range []struct {
		Name                         string                       // Test case name
		HostNamespaceSecrets         []string                     // Host namespace secrets some of whch can be AWS authentication secrets that can be useed to create secrets
		HostNamespaceLabels          map[string]string            // Host namespace labels which can include secret creation requests
		NS1NamespaceLabels           map[string]string            // Namespace 1 labels
		NS2NamespaceLabels           map[string]string            // Namespace 2 labels
		InitialSecretsCreated        int                          // Count
		AddedNamespaces              map[string]map[string]string // Namespaces to be added subsequent to initial start up stage - is a map of namespace to namespace labels
		UpdatedNamespaces            map[string]map[string]string // Namespaces to be added subsequent to initial start up stage - is a map of namespace to namespace labels
		FinalSecretsCreated          int                          // Final secret count
		ExpectedNamespacedSecretKeys string                       // Expected comma separated namespaced secret keys - distinct list of secrets that were created
	}{
		{
			Name:                         "No AWS credential secret exists",
			HostNamespaceSecrets:         []string{},
			HostNamespaceLabels:          map[string]string{ecr1: "true"},
			NS1NamespaceLabels:           map[string]string{ecr1: "true", "SomeOtherLabel": "ted"},
			NS2NamespaceLabels:           map[string]string{ecr1: "true", ecr2: "true", "env": "dev"},
			InitialSecretsCreated:        0, // No AWS credential secret exists
			FinalSecretsCreated:          0, // No additions or updates
			ExpectedNamespacedSecretKeys: "",
		},
		{
			Name:                         "No AWS credential secret in list",
			HostNamespaceSecrets:         []string{ecr1},
			HostNamespaceLabels:          map[string]string{ecr1: "true"},
			NS1NamespaceLabels:           map[string]string{ecr1: "true", "SomeOtherLabel": "ted"},
			NS2NamespaceLabels:           map[string]string{ecr1: "true", ecr2: "true", "env": "dev"},
			InitialSecretsCreated:        0, // No AWS credential secret exists
			FinalSecretsCreated:          0, // No additions or updates
			ExpectedNamespacedSecretKeys: "",
		},
		{
			Name:                         "All AWS credential secrets used by 2 namespaces exist, host namespace has label but it is set to false",
			HostNamespaceSecrets:         []string{config.AWSCredentialsSecretPrefix + "-" + ecr1},
			HostNamespaceLabels:          map[string]string{ecr1: "false"},
			NS1NamespaceLabels:           map[string]string{ecr1: "true", "SomeOtherLabel": "ted"},
			NS2NamespaceLabels:           map[string]string{ecr1: "true", ecr2: "true", "env": "dev"},
			InitialSecretsCreated:        2, // ECR1 for NS1 and NS2
			FinalSecretsCreated:          2, // No additions or updates
			ExpectedNamespacedSecretKeys: "ns-1:123456789012.dkr.ecr.eu-west-1.amazonaws.com,ns-2:123456789012.dkr.ecr.eu-west-1.amazonaws.com",
		},
		{
			Name:                         "A single AWS credential secret used by all the namespaces exists",
			HostNamespaceSecrets:         []string{config.AWSCredentialsSecretPrefix + "-" + ecr1},
			HostNamespaceLabels:          map[string]string{ecr1: "true"},
			NS1NamespaceLabels:           map[string]string{ecr1: "true", "SomeOtherLabel": "ted"},
			NS2NamespaceLabels:           map[string]string{ecr1: "true", ecr2: "true", "env": "dev"},
			InitialSecretsCreated:        3, // ECR1 for NS1, NS2 and host namespace
			FinalSecretsCreated:          3, // No additions or updates
			ExpectedNamespacedSecretKeys: "ci-cd:123456789012.dkr.ecr.eu-west-1.amazonaws.com,ns-1:123456789012.dkr.ecr.eu-west-1.amazonaws.com,ns-2:123456789012.dkr.ecr.eu-west-1.amazonaws.com",
		},
		{
			Name:                         "All AWS credential secrets used by all the namespaces exist",
			HostNamespaceSecrets:         []string{config.AWSCredentialsSecretPrefix + "-" + ecr1, config.AWSCredentialsSecretPrefix + "-" + ecr2},
			HostNamespaceLabels:          map[string]string{ecr1: "true"},
			NS1NamespaceLabels:           map[string]string{ecr1: "true", "SomeOtherLabel": "ted"},
			NS2NamespaceLabels:           map[string]string{ecr1: "true", ecr2: "true", "env": "dev"},
			InitialSecretsCreated:        4, // ECR1 for NS1, NS2, host namespace and ECR2 for NS2
			FinalSecretsCreated:          4, // No additions or updates
			ExpectedNamespacedSecretKeys: "ci-cd:123456789012.dkr.ecr.eu-west-1.amazonaws.com,ns-1:123456789012.dkr.ecr.eu-west-1.amazonaws.com,ns-2:123456789012.dkr.ecr.eu-west-1.amazonaws.com,ns-2:444456781111.dkr.ecr.us-east-1.amazonaws.com",
		},
		{
			Name:                         "Subsequent new namespace but AWS credentials for the new ECR repo do not exist",
			HostNamespaceSecrets:         []string{config.AWSCredentialsSecretPrefix + "-" + ecr1, config.AWSCredentialsSecretPrefix + "-" + ecr2},
			HostNamespaceLabels:          map[string]string{ecr1: "true"},
			NS1NamespaceLabels:           map[string]string{ecr1: "true", "SomeOtherLabel": "ted"},
			NS2NamespaceLabels:           map[string]string{ecr1: "true", ecr2: "true", "env": "dev"},
			InitialSecretsCreated:        4, // ECR1 for NS1, NS2, host namespace and ECR2 for NS2
			AddedNamespaces:              map[string]map[string]string{ns3: {ecr3: "true"}},
			FinalSecretsCreated:          4,
			ExpectedNamespacedSecretKeys: "ci-cd:123456789012.dkr.ecr.eu-west-1.amazonaws.com,ns-1:123456789012.dkr.ecr.eu-west-1.amazonaws.com,ns-2:123456789012.dkr.ecr.eu-west-1.amazonaws.com,ns-2:444456781111.dkr.ecr.us-east-1.amazonaws.com",
		},
		{
			Name:                         "Subsequent new namespace where AWS credentials for the new ECR repo do exist",
			HostNamespaceSecrets:         []string{config.AWSCredentialsSecretPrefix + "-" + ecr1, config.AWSCredentialsSecretPrefix + "-" + ecr2, config.AWSCredentialsSecretPrefix + "-" + ecr3},
			HostNamespaceLabels:          map[string]string{ecr1: "true"},
			NS1NamespaceLabels:           map[string]string{ecr1: "true", "SomeOtherLabel": "ted"},
			NS2NamespaceLabels:           map[string]string{ecr1: "true", ecr2: "true", "env": "dev"},
			InitialSecretsCreated:        4,
			AddedNamespaces:              map[string]map[string]string{ns3: {ecr3: "true"}},
			FinalSecretsCreated:          5,
			ExpectedNamespacedSecretKeys: "ci-cd:123456789012.dkr.ecr.eu-west-1.amazonaws.com,ns-1:123456789012.dkr.ecr.eu-west-1.amazonaws.com,ns-2:123456789012.dkr.ecr.eu-west-1.amazonaws.com,ns-2:444456781111.dkr.ecr.us-east-1.amazonaws.com,ns-3:444456781111.dkr.ecr.ap-southeast-2.amazonaws.com",
		},
		{
			Name:                         "Subsequent namespace alteration where label removed",
			HostNamespaceSecrets:         []string{config.AWSCredentialsSecretPrefix + "-" + ecr1, config.AWSCredentialsSecretPrefix + "-" + ecr2, config.AWSCredentialsSecretPrefix + "-" + ecr3},
			HostNamespaceLabels:          map[string]string{ecr1: "true"},
			NS1NamespaceLabels:           map[string]string{ecr1: "true", "SomeOtherLabel": "ted"},
			NS2NamespaceLabels:           map[string]string{ecr1: "true", ecr2: "true", "env": "dev"},
			InitialSecretsCreated:        4,
			UpdatedNamespaces:            map[string]map[string]string{ns1: {}},
			FinalSecretsCreated:          4,
			ExpectedNamespacedSecretKeys: "ci-cd:123456789012.dkr.ecr.eu-west-1.amazonaws.com,ns-1:123456789012.dkr.ecr.eu-west-1.amazonaws.com,ns-2:123456789012.dkr.ecr.eu-west-1.amazonaws.com,ns-2:444456781111.dkr.ecr.us-east-1.amazonaws.com",
		},
		{
			Name:                         "Subsequent namespace alteration where labels removed and new one added",
			HostNamespaceSecrets:         []string{config.AWSCredentialsSecretPrefix + "-" + ecr1, config.AWSCredentialsSecretPrefix + "-" + ecr2, config.AWSCredentialsSecretPrefix + "-" + ecr3},
			NS1NamespaceLabels:           map[string]string{ecr1: "true", "SomeOtherLabel": "ted"},
			NS2NamespaceLabels:           map[string]string{ecr1: "true", ecr2: "true", "env": "dev"},
			InitialSecretsCreated:        3,
			UpdatedNamespaces:            map[string]map[string]string{ns1: {ecr3: "true"}},
			FinalSecretsCreated:          4,
			ExpectedNamespacedSecretKeys: "ns-1:123456789012.dkr.ecr.eu-west-1.amazonaws.com,ns-1:444456781111.dkr.ecr.ap-southeast-2.amazonaws.com,ns-2:123456789012.dkr.ecr.eu-west-1.amazonaws.com,ns-2:444456781111.dkr.ecr.us-east-1.amazonaws.com",
		},
		{
			Name:                         "Subsequent namespace added and alteration where label removed",
			HostNamespaceSecrets:         []string{config.AWSCredentialsSecretPrefix + "-" + ecr1, config.AWSCredentialsSecretPrefix + "-" + ecr2, config.AWSCredentialsSecretPrefix + "-" + ecr3},
			HostNamespaceLabels:          map[string]string{ecr1: "true"},
			NS1NamespaceLabels:           map[string]string{ecr1: "true", "SomeOtherLabel": "ted"},
			NS2NamespaceLabels:           map[string]string{ecr1: "true", ecr2: "true", "env": "dev"},
			InitialSecretsCreated:        4,
			AddedNamespaces:              map[string]map[string]string{ns4: {ecr3: "true"}},
			UpdatedNamespaces:            map[string]map[string]string{ns1: {ecr3: "false"}},
			FinalSecretsCreated:          5,
			ExpectedNamespacedSecretKeys: "ci-cd:123456789012.dkr.ecr.eu-west-1.amazonaws.com,ns-1:123456789012.dkr.ecr.eu-west-1.amazonaws.com,ns-2:123456789012.dkr.ecr.eu-west-1.amazonaws.com,ns-2:444456781111.dkr.ecr.us-east-1.amazonaws.com,ns-4:444456781111.dkr.ecr.ap-southeast-2.amazonaws.com",
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			seedData := []FakeK8SClientSeedNamespace{
				{
					Name:     config.HostNamespace,
					IsActive: true,
					Labels:   tc.HostNamespaceLabels,
					Secrets:  tc.HostNamespaceSecrets,
				},
				{
					Name:     ns1,
					IsActive: true,
					Labels:   tc.NS1NamespaceLabels,
				},
				{
					Name:     ns2,
					IsActive: true,
					Labels:   tc.NS2NamespaceLabels,
				},
			}
			k8sClient := NewFakeK8SClient(seedData)
			nsInformer := NewFakeSharedInformer()
			prometheusRegistry := prometheus.NewRegistry()
			ecrClient := NewFakeECRClient()

			ctx, cancel := context.WithCancel(context.Background())
			ctrl, err := newController(config, k8sClient, nsInformer, prometheusRegistry, ecrClient)
			assert.Nil(t, err, "New controller error")

			go ctrl.Run(ctx.Done())

			// Simulate informers initial add events - easier to so this way rather than via code in the fake informer
			nsList, _ := k8sClient.GetNamespaces()
			for _, ns := range nsList.Items {
				nsInformer.SimulateAddNamespace(&ns)
			}

			// Allow time for the initialization to complete - goroutines etc.
			time.Sleep(150 * time.Millisecond)
			actualCount := k8sClient.TotalSecretsCreated()
			assert.Equal(t, tc.InitialSecretsCreated, actualCount, "Initial secret creation count")

			// New namespaces
			for nsName, nsLabels := range tc.AddedNamespaces {
				ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName, Namespace: nsName, Labels: nsLabels}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}}
				k8sClient.InsertNewNamespaceRecord(ns)
				nsInformer.SimulateAddNamespace(ns)
			}
			// Altered namespaces
			for nsName, nsLabels := range tc.UpdatedNamespaces {
				// Going to assume we can get ns, so ignoring err
				oldNS, _ := k8sClient.GetNamespace(nsName)
				newNS := oldNS.DeepCopy()
				newNS.Labels = nsLabels
				newNS.ResourceVersion += "."
				k8sClient.UpdateNamespaceRecord(newNS)
				nsInformer.SimulateUpdateNamespace(oldNS, newNS)
			}

			// Allow time for the initialization to complete - goroutines etc.
			time.Sleep(150 * time.Millisecond)
			cancel()
			actualCount = k8sClient.TotalSecretsCreated()
			assert.Equal(t, tc.FinalSecretsCreated, actualCount, "Final secret creation count")

			// Allow time for the controller to stop and re-apply altered namespaces - post cancellation - should be ignored
			time.Sleep(150 * time.Millisecond)
			for nsName, nsLabels := range tc.UpdatedNamespaces {
				// Going to assume we can get ns, so ignoring err
				oldNS, _ := k8sClient.GetNamespace(nsName)
				newNS := oldNS.DeepCopy()
				newNS.Labels = nsLabels
				newNS.ResourceVersion += "."
				k8sClient.UpdateNamespaceRecord(newNS)
				nsInformer.SimulateUpdateNamespace(oldNS, newNS)
			}
			time.Sleep(150 * time.Millisecond)
			actualCount = k8sClient.TotalSecretsCreated()
			assert.Equal(t, tc.FinalSecretsCreated, actualCount, "Final secret creation post controller cancellation count")

			actualNamespacedSecretKeys := k8sClient.DistinctNamespacedSecretKeysCreated()
			assert.Equal(t, tc.ExpectedNamespacedSecretKeys, actualNamespacedSecretKeys, "Namespaced secret keys")
		})
	}
}

func TestGetNamespacesToProcess(t *testing.T) {
	config := getDefaultConfig()
	k8sClient := NewFakeK8SClient([]FakeK8SClientSeedNamespace{
		{
			Name:     ns1,
			IsActive: true,
			Labels:   map[string]string{ecr1: "true"},
		},
		{
			Name:     ns2,
			IsActive: true,
			Labels:   map[string]string{ecr1: "false", ecr2: "true", ecr3: "false"},
		},
		{
			Name:     ns3,
			IsActive: true,
			Labels:   map[string]string{ecr1: "true", ecr2: "true", ecr3: "true"},
		},
	})
	nsInformer := NewFakeSharedInformer()
	prometheusRegistry := prometheus.NewRegistry()
	ecrClient := NewFakeECRClient()

	ctrl, err := newController(config, k8sClient, nsInformer, prometheusRegistry, ecrClient)
	assert.Nil(t, err, "New controller error")

	nss, err := ctrl.getNamespacesToProcess(allNamespacesKey)
	assert.Nil(t, err, "Get namespaces to process error")
	assert.NotNil(t, 3, len(nss), "Namesapces to process count")
}

func TestGetDistinctSecretNames(t *testing.T) {
	config := getDefaultConfig()
	k8sClient := NewFakeK8SClient(nil)
	nsInformer := NewFakeSharedInformer()
	prometheusRegistry := prometheus.NewRegistry()
	ecrClient := NewFakeECRClient()

	ctrl, err := newController(config, k8sClient, nsInformer, prometheusRegistry, ecrClient)
	assert.Nil(t, err, "New controller error")

	secretNames := ctrl.getDistinctSecretNames([]corev1.Namespace{
		corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns1, Namespace: ns1, Labels: map[string]string{"abc": "something", ecr1: "true", ecr2: "false", ecr3: "true"}}},
		corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns2, Namespace: ns1, Labels: map[string]string{ecr3: "true", "env": "dev"}}},
	})

	assert.Equal(t, 2, len(secretNames), "Count")
}

func TestCreateECRAuthTokenData(t *testing.T) {
	config := getDefaultConfig()
	for _, tc := range []struct {
		Name                 string   // Test case name
		HostNamespaceSecrets []string // Host namespace secrets some of whch can be AWS authentication secrets that can be useed to create secrets
		NamespaceName        string   // Namespace
		NamespaceSeedSecrets []string // Namespace seed secrets
		SecretNames          []string // Secret names
		ExpectedCount        int      // Expected secret auth data tokens created
	}{
		{
			Name:                 "No AWS credential secret exists",
			HostNamespaceSecrets: []string{},
			NamespaceName:        ns1,
			SecretNames:          []string{"s1"},
			ExpectedCount:        0,
		},
		{
			Name:                 "1 AWS credential secret exists",
			HostNamespaceSecrets: []string{config.AWSCredentialsSecretPrefix + "-s1", config.AWSCredentialsSecretPrefix + "-s2"},
			NamespaceName:        ns1,
			SecretNames:          []string{"s1"},
			ExpectedCount:        1,
		},
		{
			Name:                 "All AWS credential secret exists",
			HostNamespaceSecrets: []string{config.AWSCredentialsSecretPrefix + "-s1", config.AWSCredentialsSecretPrefix + "-s2"},
			NamespaceName:        ns1,
			SecretNames:          []string{"s1", "s2"},
			ExpectedCount:        2,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			k8sClient := NewFakeK8SClient([]FakeK8SClientSeedNamespace{
				{
					Name:     config.HostNamespace,
					IsActive: true,
					Secrets:  tc.HostNamespaceSecrets,
				},
				{
					Name:     tc.NamespaceName,
					IsActive: true,
				},
			})
			nsInformer := NewFakeSharedInformer()
			prometheusRegistry := prometheus.NewRegistry()
			ecrClient := NewFakeECRClient()

			ctrl, err := newController(config, k8sClient, nsInformer, prometheusRegistry, ecrClient)
			assert.Nil(t, err, "New controller error")

			authTokenData, err := ctrl.createECRAuthTokenData(tc.SecretNames)
			assert.Nil(t, err, "Create ECR token data")
			assert.NotNil(t, authTokenData, "ECR token data")
			assert.Equal(t, tc.ExpectedCount, len(authTokenData), "ECR token data count")
		})
	}
}

func TestCreateNamespaceSecret(t *testing.T) {
	for _, tc := range []struct {
		Name                         string   // Test case name
		NamespaceName                string   // Namespace
		NamespaceSeedSecrets         []string // Namespace seed secrets
		SecretName                   string   // Secret name
		ExpectedNamespacedSecretKeys string   // Expected comma separated namespaced secret keys - distinct list of secrets that were created
	}{
		{
			Name:                         "No AWS credential secret exists",
			NamespaceName:                ns1,
			NamespaceSeedSecrets:         []string{},
			SecretName:                   "the-secret",
			ExpectedNamespacedSecretKeys: "ns-1:the-secret",
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			config := getDefaultConfig()
			k8sClient := NewFakeK8SClient([]FakeK8SClientSeedNamespace{
				FakeK8SClientSeedNamespace{
					Name:     tc.NamespaceName,
					IsActive: true,
					Secrets:  []string{},
				},
			})
			nsInformer := NewFakeSharedInformer()
			prometheusRegistry := prometheus.NewRegistry()
			ecrClient := &FakeECRClient{}

			ctrl, err := newController(config, k8sClient, nsInformer, prometheusRegistry, ecrClient)
			assert.Nil(t, err, "New controller error")

			// Create
			err = ctrl.createNamespaceSecret(tc.NamespaceName, tc.SecretName, &ecr.AuthorizationData{ProxyEndpoint: aws.String("ecr-endpoint"), AuthorizationToken: aws.String("password which as an ECR token-1")})
			assert.Nil(t, err, "Creation error")
			actualNamespacedSecretKeys := k8sClient.DistinctNamespacedSecretKeysCreated()
			assert.Equal(t, tc.ExpectedNamespacedSecretKeys, actualNamespacedSecretKeys, "Namespaced secret keys")
			actualCount := k8sClient.NewlyCreatedSecretCount()
			assert.Equal(t, 1, actualCount, "Secret creation count")

			// Update
			err = ctrl.createNamespaceSecret(tc.NamespaceName, tc.SecretName, &ecr.AuthorizationData{ProxyEndpoint: aws.String("ecr-endpoint"), AuthorizationToken: aws.String("password which as an ECR token-2")})
			assert.Nil(t, err, "Update error")
			actualCount = k8sClient.UpdatedSecretCount()
			assert.Equal(t, 1, actualCount, "Secret update count")
		})
	}
}
