package meta

import (
	"fmt"
	"io"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"

	corev1 "k8s.io/api/core/v1"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
)

const (
	// NamespaceSeparator used by the K8s informers library to create indices
	// that are composed as namespace/name
	NamespaceSeparator = "/"
	IndexIP            = "IP"
)

var ilog = logrus.WithFields(logrus.Fields{
	"component": fmt.Sprintf("%T", Informers{}),
})

type Informers struct {
	informerFactory informers.SharedInformerFactory
	pods            cache.SharedIndexInformer
	services        cache.SharedIndexInformer
	replicaSet      cache.SharedIndexInformer
}

func NewInformers(client kubernetes.Interface) Informers {
	// TODO: configure resync time
	factory := informers.NewSharedInformerFactory(client, 1*time.Hour)
	pods := factory.Core().V1().Pods().Informer()
	if err := pods.AddIndexers(map[string]cache.IndexFunc{
		IndexIP: func(obj interface{}) ([]string, error) {
			pod := obj.(*corev1.Pod)
			ips := make([]string, 0, len(pod.Status.PodIPs))
			for _, ip := range pod.Status.PodIPs {
				// ignoring host-networked Pod IPs
				if ip.IP != pod.Status.HostIP {
					ips = append(ips, ip.IP)
				}
			}
			return ips, nil
		},
	}); err != nil {
		// this should never happen, as it only returns error if the informer has
		// been alrady started
		panic(err)
	}
	services := factory.Core().V1().Services().Informer()
	if err := services.AddIndexers(map[string]cache.IndexFunc{
		IndexIP: func(obj interface{}) ([]string, error) {
			spec := obj.(*corev1.Service).Spec
			if spec.ClusterIP == corev1.ClusterIPNone {
				return []string{}, nil
			}
			return spec.ClusterIPs, nil
		},
	}); err != nil {
		panic(err)
	}
	return Informers{
		informerFactory: factory,
		pods:            pods,
		services:        services,
		replicaSet:      factory.Apps().V1().ReplicaSets().Informer(),
	}
}

func (s *Informers) Start(stopCh <-chan struct{}) error {
	s.informerFactory.Start(stopCh)
	return nil
}

func (s *Informers) WaitForCacheSync(stopCh <-chan struct{}) {
	s.informerFactory.WaitForCacheSync(stopCh)
}

func (s *Informers) PodByIP(ip string) (*corev1.Pod, bool) {
	item, err := s.pods.GetIndexer().ByIndex(IndexIP, ip)
	if err != nil {
		// should never happen as long as we provide the correct index function
		// otherwise it's a bug in our code
		panic(err)
	}
	// our provided indexers only return a key, so it's safe to assume 0<=len()<=1
	if len(item) == 0 {
		// not found
		return nil, false
	}
	// since we are excluding host-networked pods, the relation IP:Pod should be 1:1.
	if len(item) > 1 {
		ilog.WithFields(logrus.Fields{
			"ip":      ip,
			"results": len(item),
		}).Warn("multiple pods for a single IP. Returning the first pod and ignoring the rest")
	}
	return item[0].(*corev1.Pod), true
}

func (s *Informers) ServiceByIP(ip string) (*corev1.Service, bool) {
	item, err := s.services.GetIndexer().ByIndex(IndexIP, ip)
	if err != nil {
		// should never happen as long as we provide the correct index function
		// otherwise it's a bug in our code
		panic(err)
	}
	if len(item) == 0 {
		// not found
		return nil, false
	}
	// we assume a 1:1 relation between Service and ClusterIP
	if len(item) > 1 {
		ilog.WithFields(logrus.Fields{
			"ip":      ip,
			"results": len(item),
		}).Warn("multiple services for a single IP. Returning the first service and ignoring the rest")
	}
	return item[0].(*corev1.Service), true
}

func (s *Informers) ReplicaSet(namespace, name string) (*appsv1.ReplicaSet, bool) {
	item, ok, err := s.replicaSet.GetIndexer().GetByKey(namespace + NamespaceSeparator + name)
	if err != nil {
		// should never happen. Otherwise it's a bug in our code
		panic(err)
	}
	if !ok {
		return nil, false
	}
	return item.(*appsv1.ReplicaSet), true
}

func (s *Informers) DebugInfo(out io.Writer) {
	fmt.Fprintln(out, "==== Services")
	for _, svc := range s.services.GetStore().ListKeys() {
		fmt.Fprintln(out, "-", svc)
	}
	fmt.Fprintln(out, "==== ReplicaSets")
	for _, rs := range s.replicaSet.GetStore().ListKeys() {
		rskeys := strings.Split(rs, NamespaceSeparator)
		rset, _ := s.ReplicaSet(rskeys[0], rskeys[1])
		fmt.Fprintln(out, "-", rs, "replicas:", rset.Status.Replicas)
	}
	fmt.Fprintln(out, "==== Pods")
	for _, pod := range s.pods.GetStore().ListKeys() {
		fmt.Fprintln(out, "-", pod)
	}
	fmt.Fprintln(out, "=== Pods by IP")
	for _, ip := range s.pods.GetIndexer().ListIndexFuncValues(IndexIP) {
		pod, _ := s.PodByIP(ip)
		if pod.Status.PodIP != ip {
			panic("ips not equal")
		}
		fmt.Fprintln(out, "-", ip, ":", pod.Name)
	}
	fmt.Fprintln(out, "=== Services by IP")
	for _, ip := range s.services.GetIndexer().ListIndexFuncValues(IndexIP) {
		svc, ok := s.ServiceByIP(ip)
		if ok {
			if svc.Spec.ClusterIP != ip {
				panic("ips not equal")
			}
			fmt.Fprintln(out, "-", ip, ":", svc.Name)
		} else {
			fmt.Fprintln(out, "-", ip, "not found in index")
		}
	}
}
