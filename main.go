package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecr"

	"github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

// See	https://blog.heptio.com/straighten-out-your-kubernetes-client-go-dependencies-heptioprotip-8baeed46fe7d
//	https://github.com/upmc-enterprises/registry-creds
//	https://github.com/jbeda/tgik-controller
const (
	DefaultAuthenticationTokenRenewalInterval = 10 * time.Second
	DefaultInformersResyncInterval            = 10 * time.Minute
	DefaultNamespaceBlacklist                 = "ci-cd,default,kube-public,kube-system,monitoring"
)

type config struct {
	AuthenticationTokenRenewalInterval time.Duration
	InformersResyncInterval            time.Duration
	KubeConfigFilePath                 string
	NamespaceBlacklist                 string
}

type controller struct {
	Config                config
	Client                *kubernetes.Clientset
	NamespaceListerSynced cache.InformerSynced
	GetECRAuthData        func(context.Context) (*ecr.AuthorizationData, error)
}

func main() {
	config, err := getConfig()
	dieIfErr(err)

	client, err := newK8sClient(config.KubeConfigFilePath)
	dieIfErr(err)

	informersFactory := informers.NewSharedInformerFactory(client, config.InformersResyncInterval)
	controller, err := newController(client, config, informersFactory, getECRAuthData)
	dieIfErr(err)

	ctx, cancel := context.WithCancel(context.Background())

	term := make(chan os.Signal)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)

	informersFactory.Start(ctx.Done())
	go controller.Run(ctx.Done())
	go func(s <-chan struct{}) {
		<-s
		log.Println(">>>>>>>> DONE")
	}(ctx.Done())

	log.Println("Waiting")
	<-term
	cancel()

	log.Println("Allowing 3 seconds to shutdown")
	time.Sleep(3 * time.Second)
	log.Println("Done")
	/*
		authData, err := getECRAuthData(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Err: %v", err)
			os.Exit(1)
		}
		log.Printf("Got ECR AuthData: %v\n", authData)

		nss, err := getActiveK8sNamespaces(ctx, config.KubeConfigFilePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Err: %v", err)
			os.Exit(1)
		}
		log.Printf("Got k8s namespaces: %v\n", nss)
	*/

}

func getConfig() (config, error) {
	config := config{
		AuthenticationTokenRenewalInterval: DefaultAuthenticationTokenRenewalInterval,
		InformersResyncInterval:            DefaultInformersResyncInterval,
		KubeConfigFilePath:                 os.Getenv("KUBECONFIG"),
		NamespaceBlacklist:                 DefaultNamespaceBlacklist,
	}

	flag.StringVar(&config.KubeConfigFilePath, "config-file-path", config.KubeConfigFilePath, "Kube config file pathi, optional, only used for testing outside the cluster, can also set the KUBECONFIG env var")
	flag.DurationVar(&config.AuthenticationTokenRenewalInterval, "auth-token-renewal-interval", config.AuthenticationTokenRenewalInterval, "Authentication token renewal interval - ECR tokens expire after 12 hours so should be less")
	flag.StringVar(&config.NamespaceBlacklist, "namespace-blacklist", config.NamespaceBlacklist, "Namespace blacklist (comma seperated list)")
	flag.DurationVar(&config.InformersResyncInterval, "informers-resync-interval", config.InformersResyncInterval, "Shared informers resync interval")
	flag.Parse()

	return config, nil
}

func dieIfErr(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, err.Error())
		os.Exit(2)
	}
}

func newK8sClient(configFilePath string) (*kubernetes.Clientset, error) {
	var config *rest.Config
	var err error
	if configFilePath != "" {
		config, err = clientcmd.BuildConfigFromFlags("", configFilePath)
	} else {
	}
	if err != nil {
		return nil, errors.Wrap(err, "create k8s client failed")
	}
	client := kubernetes.NewForConfigOrDie(config)

	return client, nil
}

func newController(client *kubernetes.Clientset, config config, informersFactory informers.SharedInformerFactory, getECRAuthData func(context.Context) (*ecr.AuthorizationData, error)) (*controller, error) {
	nsInformer := informersFactory.Core().V1().Namespaces()

	ctrl := &controller{
		Config:                config,
		Client:                client,
		NamespaceListerSynced: nsInformer.Informer().HasSynced,
		GetECRAuthData:        getECRAuthData,
	}

	nsInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				ctrl.onNSAdd(obj.(*corev1.Namespace))
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				ctrl.onNSUpdate(oldObj.(*corev1.Namespace), newObj.(*corev1.Namespace))

			},
		},
	)

	return ctrl, nil
}

func (c *controller) Run(stop <-chan struct{}) {
	log.Print("Controller: Waiting for cache sync")
	if !cache.WaitForCacheSync(stop, c.NamespaceListerSynced) {
		log.Print("Controller: Timed out waiting for cache sync")
		return
	}
	log.Print("Controller: Caches are synced")

	tick := time.Tick(c.Config.AuthenticationTokenRenewalInterval)
	for {
		select {
		case <-tick:
			log.Print("Controller: Tick")
			authData, err := c.GetECRAuthData(context.Background())
			if err != nil {
				log.Printf("Controller: Error ECR auth data : %s\n", err)
				runtime.HandleError(err)
				continue
			}
			log.Printf("Controller: Tick i%s\n", *(*authData).ProxyEndpoint)

		case <-stop:
			log.Print("Controller: Received stop signal")
			return
		}
	}
}

func (c *controller) onNSAdd(ns *corev1.Namespace) {
	log.Printf("Controller: onNSAdd: %s\n", ns.GetName())
}

func (c *controller) onNSUpdate(oldNs, newNs *corev1.Namespace) {
	log.Printf("Controller: onNSUpdate: %s   %s\n", oldNs.GetName(), newNs.GetName())
}

func getECRAuthData(ctx context.Context) (*ecr.AuthorizationData, error) {
	svc := ecr.New(session.New())

	inp := &ecr.GetAuthorizationTokenInput{}
	out, err := svc.GetAuthorizationTokenWithContext(ctx, inp)
	if err != nil {
		return nil, errors.Wrap(err, "get authorization token failed")
	}

	return out.AuthorizationData[0], nil
}

func getActiveK8sNamespaces(ctx context.Context, configFilePath string) ([]string, error) {
	client, err := newK8sClient(configFilePath)
	if err != nil {
		return nil, errors.Wrap(err, "create k8s client failed")
	}

	list, err := client.CoreV1().Namespaces().List(metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "list k8s namespaces failed")
	}

	var nss []string
	for _, ns := range list.Items {
		if ns.Status.Phase == corev1.NamespaceActive {
			nss = append(nss, ns.Name)
		}
	}

	return nss, nil
}
