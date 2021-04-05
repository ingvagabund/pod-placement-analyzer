package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"time"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/ingvagabund/pod-placement-analyzer/pkg"
)

func main() {

	pc := pkg.NewPodCollector()

	dat, _ := ioutil.ReadFile("testdata/pods.json")
	fmt.Println(string(dat))
	fmt.Printf("err: %v\n", pc.Import(dat))

	pc.ComputePodTransitions()

	pc.PodDisplacements().Dump()

	return

	cfg := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{})
	clusterConfig, err := cfg.ClientConfig()
	if err != nil {
		fmt.Printf("could not load client configuration: %v", err)
		return
	}
	client, err := kubernetes.NewForConfig(clusterConfig)
	if err != nil {
		fmt.Printf("err: %v\n", err)
		return
	}
	sharedInformerFactory := informers.NewSharedInformerFactory(client, 10*time.Minute)

	ctx, cancel := context.WithCancel(context.Background())

	pc.Setup(ctx, sharedInformerFactory)
	pc.Run(ctx)

	time.Sleep(time.Minute)
	pc.ComputePodTransitions()

	data, err := pc.JsonDump()
	if err != nil {
		fmt.Printf("JsonDump failed: %v", err)
	} else {
		fmt.Println(data)
	}

	cancel()
}
