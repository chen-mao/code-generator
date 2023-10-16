package main

import (
	"controller-demo/pkg/client/clientset/versioned"
	"controller-demo/pkg/client/informers/externalversions"
	"flag"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/klog/v2"
)

func initClient() (*kubernetes.Clientset, *rest.Config, error) {
	var err error
	var kubeconfig *string
	var config *rest.Config

	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	if config, err = rest.InClusterConfig(); err != nil {
		if config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig); err != nil {
			panic(err.Error())
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, err
	}
	return clientset, config, nil
}

func setupSignalHandler() (stopCh <-chan struct{}) {
	stop := make(chan struct{})

	c := make(chan os.Signal, 2)
	signal.Notify(c, []os.Signal{os.Interrupt, syscall.SIGTERM}...)
	go func() {
		<-c
		close(stop)
		<-c
		os.Exit(1)
	}()
	return stop
}

func main() {
	flag.Parse()

	_, config, err := initClient()
	if err != nil {
		klog.Fatalf("Error init kubernetes client: %s", err.Error())
	}

	// 实例化一个crontab的clientset
	crontabClientSet, err := versioned.NewForConfig(config)
	if err != nil {
		klog.Fatalf("Error init kubernetes crontab client: %s", err)
	}

	stopCh := setupSignalHandler()

	// 实例化Crontab的informerFactory
	sharedInformerFactory := externalversions.NewSharedInformerFactory(crontabClientSet, time.Second*30)

	// 实例化crontab 控制器
	controller := NewController(sharedInformerFactory.Stable().V1beta1().CronTabs())

	go sharedInformerFactory.Start(stopCh)

	if err := controller.Run(1, stopCh); err != nil {
		klog.Fatalf("Error running crontab controller: %s", err.Error())
	}
}
