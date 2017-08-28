package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"k8s.io/client-go/informers"
)

// See	https://blog.heptio.com/straighten-out-your-kubernetes-client-go-dependencies-heptioprotip-8baeed46fe7d
//	https://github.com/coreos/prometheus-operator/blob/master/pkg/prometheus/operator.go
//	https://github.com/upmc-enterprises/registry-creds
//	https://github.com/jbeda/tgik-controller
//	https://engineering.bitnami.com/articles/a-deep-dive-into-kubernetes-controllers.html
//	https://github.com/skippbox/kubewatch/
var (
	version     = "1.0"
	repoBranch  = "NotSet"
	repoVersion = "NotSet"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	config, err := getConfig(os.Args)
	dieIfErr(err)

	glog.Infof("Starting listener on port %d\n", config.Port)
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", config.Port))
	dieIfErr(err)

	glog.Infoln("Newing up k8s client")
	k8sClient, err := newK8sClient(config.KubeConfigFilePath)
	dieIfErr(err)

	glog.Infoln("Newing up ECR")
	ecr := newECRClient()

	glog.Infoln("Newing up shared informer factory and namesapce informer")
	informersFactory := informers.NewSharedInformerFactory(k8sClient.ClientSet, config.InformersResyncInterval)
	nsInformer := informersFactory.Core().V1().Namespaces()

	glog.Infoln("Getting prometheus registry")
	prometheusRegistry := prometheus.DefaultRegisterer.(*prometheus.Registry)

	glog.Infoln("Newing up controller")
	controller, err := newController(config, k8sClient, nsInformer.Informer(), prometheusRegistry, ecr)
	dieIfErr(err)

	glog.Infoln("Newing up diagnostic HTTP server")
	srv := newDiagnosticHTTPServer()

	glog.Infoln("Starting informers factory")
	informersFactory.Start(ctx.Done())

	glog.Infoln("Starting controller go routine")
	go func() {
		controller.Run(ctx.Done())
		glog.Infoln("Controller run completed")
	}()

	glog.Infoln("Starting diagnostic HTTP server go routine")
	go func() {
		err := srv.Serve(listener)
		if err != http.ErrServerClosed {
			dieIfErr(errors.Wrap(err, "HTTP serve failed"))
		}
		glog.Infoln("HTTP serve completed")
	}()

	glog.Infoln("Starting diagnostic HTTP server gracefull shutdown go routine")
	go func() {
		<-ctx.Done()
		glog.Infoln("Shutting down HTTP server")
		if err := srv.Shutdown(context.Background()); err != nil {
			dieIfErr(errors.Wrap(err, "HTTP server shutdown failed"))
		}
		glog.Infoln("HTTP server shutdown completed")
	}()

	glog.Infoln("Waiting...")
	term := make(chan os.Signal)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)
	<-term
	cancel()

	glog.Infof("Allowing %s to shutdown\n", config.ShutdownGracePeriod)
	time.Sleep(config.ShutdownGracePeriod)
	glog.Infoln("Done")
}

func dieIfErr(err error) {
	if err != nil {
		glog.Error(err.Error())
		os.Exit(2)
	}
}

func newDiagnosticHTTPServer() *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
	mux.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	mux.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	mux.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
	mux.Handle("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))

	return &http.Server{Handler: mux}
}
