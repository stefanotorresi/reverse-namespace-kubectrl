package main

import (
	"flag"
	"time"

	"reverse-namespace-kubectrl/pkg/signals"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
)

var (
	masterURL  string
	kubeconfig string
)

func main() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	flag.Parse()

	klog.InitFlags(nil)

	stop := signals.SetupSignalHandler()

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		klog.Fatalf("Error building kubeconfig: %s", err.Error())
	}

	kube, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Error building kubernetes client: %s", err.Error())
	}

	informerFactory := informers.NewSharedInformerFactory(kube, time.Second*30)
	controller := NewController(kube, informerFactory)

	if err = controller.Run(2, stop); err != nil {
		klog.Fatalf("Error running controller: %s", err.Error())
	}
}
