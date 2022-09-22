/*
Copyright 2018-2022 Mailgun Technologies Inc

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

package k8spool

import (
	"context"
	"fmt"
	"log"
	"reflect"

	api_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

type UpdateFunc func(peers []string)

type Logger interface {
	Debugf(format string, v ...any)
	Errorf(format string, v ...any)
}

type StdLogger struct {
	Debug bool
	Error bool
}

func (c StdLogger) Debugf(format string, v ...any) {
	if c.Debug {
		log.Printf(format, v...)
	}

}
func (c StdLogger) Errorf(format string, v ...any) {
	if c.Error {
		log.Printf(format, v...)
	}
}

type K8sPool struct {
	informer    cache.SharedIndexInformer
	client      *kubernetes.Clientset
	log         Logger
	conf        Config
	watchCtx    context.Context
	watchCancel func()
	done        chan struct{}
}

type WatchMechanism string

const (
	WatchEndpoints WatchMechanism = "endpoints"
	WatchPods      WatchMechanism = "pods"
)

type Config struct {
	Logger     Logger
	Mechanism  WatchMechanism
	OnUpdate   UpdateFunc
	Namespace  string
	Selector   string
	PeerScheme string
	PeerPort   int
}

func New(conf Config) (*K8sPool, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("Failed to get k8s rest config: %w", err)
	}
	// creates the client
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("Failed to create k8s client: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	if conf.Logger == nil {
		conf.Logger = &StdLogger{Error: true}
	}
	if conf.PeerScheme == "" {
		conf.PeerScheme = "http"
	}
	if conf.PeerPort == 0 {
		conf.PeerPort = 8080
	}

	pool := &K8sPool{
		done:        make(chan struct{}),
		log:         conf.Logger,
		client:      client,
		conf:        conf,
		watchCtx:    ctx,
		watchCancel: cancel,
	}

	return pool, pool.start()
}

func (e *K8sPool) start() error {
	switch e.conf.Mechanism {
	case "", WatchEndpoints:
		return e.startEndpointWatch()
	case WatchPods:
		return e.startPodWatch()
	default:
		return fmt.Errorf("unknown value for watch mechanism: %s", e.conf.Mechanism)
	}
}

func (e *K8sPool) startGenericWatch(objType runtime.Object, listWatch *cache.ListWatch, updateFunc func()) error {
	e.informer = cache.NewSharedIndexInformer(
		listWatch,
		objType,
		0, //Skip resync
		cache.Indexers{},
	)

	e.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			e.log.Debugf("Queue (Add) '%s' - %v", key, err)
			if err != nil {
				e.log.Errorf("while calling MetaNamespaceKeyFunc(): %s", err)
				return
			}
			updateFunc()
		},
		UpdateFunc: func(obj, new interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			e.log.Debugf("Queue (Update) '%s' - %v", key, err)
			if err != nil {
				e.log.Errorf("while calling MetaNamespaceKeyFunc(): %s", err)
				return
			}
			updateFunc()
		},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			e.log.Debugf("Queue (Delete) '%s' - %v", key, err)
			if err != nil {
				e.log.Errorf("while calling MetaNamespaceKeyFunc(): %s", err)
				return
			}
			updateFunc()
		},
	})

	go e.informer.Run(e.done)

	if !cache.WaitForCacheSync(e.done, e.informer.HasSynced) {
		close(e.done)
		return fmt.Errorf("timed out waiting for caches to sync")
	}

	return nil
}

func (e *K8sPool) startPodWatch() error {
	listWatch := &cache.ListWatch{
		ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
			options.LabelSelector = e.conf.Selector
			return e.client.CoreV1().Pods(e.conf.Namespace).List(context.Background(), options)
		},
		WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
			options.LabelSelector = e.conf.Selector
			return e.client.CoreV1().Pods(e.conf.Namespace).Watch(e.watchCtx, options)
		},
	}
	return e.startGenericWatch(&api_v1.Pod{}, listWatch, e.updatePeersFromPods)
}

func (e *K8sPool) startEndpointWatch() error {
	listWatch := &cache.ListWatch{
		ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
			options.LabelSelector = e.conf.Selector
			return e.client.CoreV1().Endpoints(e.conf.Namespace).List(context.Background(), options)
		},
		WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
			options.LabelSelector = e.conf.Selector
			return e.client.CoreV1().Endpoints(e.conf.Namespace).Watch(e.watchCtx, options)
		},
	}
	return e.startGenericWatch(&api_v1.Endpoints{}, listWatch, e.updatePeersFromEndpoints)
}

func (e *K8sPool) updatePeersFromPods() {
	e.log.Debugf("Fetching peer list from pods API")
	var peers []string
main:
	for _, obj := range e.informer.GetStore().List() {
		pod, ok := obj.(*api_v1.Pod)
		if !ok {
			e.log.Errorf("expected type v1.Endpoints got '%s' instead", reflect.TypeOf(obj).String())
		}

		peer := fmt.Sprintf("%s://%s:%d", e.conf.PeerScheme, pod.Status.PodIP, e.conf.PeerPort)

		// if containers are not ready or not running then skip this peer
		for _, status := range pod.Status.ContainerStatuses {
			if !status.Ready || status.State.Running == nil {
				e.log.Debugf("Skipping peer because it's not ready or not running: %+v\n", peer)
				continue main
			}
		}

		e.log.Debugf("Peer: %+v\n", peer)
		peers = append(peers, peer)
	}
	e.conf.OnUpdate(peers)
}

func (e *K8sPool) updatePeersFromEndpoints() {
	e.log.Debugf("Fetching peer list from endpoints API")
	var peers []string
	for _, obj := range e.informer.GetStore().List() {
		endpoint, ok := obj.(*api_v1.Endpoints)
		if !ok {
			e.log.Errorf("expected type v1.Endpoints got '%s' instead", reflect.TypeOf(obj).String())
		}

		for _, s := range endpoint.Subsets {
			for _, addr := range s.Addresses {
				peer := fmt.Sprintf("%s://%s:%d", e.conf.PeerScheme, addr.IP, e.conf.PeerPort)

				peers = append(peers, peer)
				e.log.Debugf("Peer: %+v\n", peer)
			}
		}
	}
	e.conf.OnUpdate(peers)
}

func (e *K8sPool) Close() {
	e.watchCancel()
	close(e.done)
}
