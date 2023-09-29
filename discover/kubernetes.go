package discover

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/util/name"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	watch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type KubernetesD struct {
	target        string
	namespace     string
	labelselector string
	crpcport      int
	cgrpcport     int
	webport       int
	notices       map[chan *struct{}]*struct{}
	status        int32 //0-idle,1-discover,2-stop
	watcher       watch.Interface

	sync.RWMutex
	addrs     map[string]*struct{}
	version   string
	lasterror error
}

var lker sync.Mutex
var kubeclient *kubernetes.Clientset

func initKubernetesClient() {
	if lker.TryLock() {
		defer lker.Unlock()
		if kubeclient != nil {
			return
		}
		if c, e := rest.InClusterConfig(); e != nil {
			panic(e)
		} else if kubeclient, e = kubernetes.NewForConfig(c); e != nil {
			panic(e)
		}
	}
}

func NewKubernetesDiscover(targetproject, targetgroup, targetapp, namespace, labelselector string, crpcport, cgrpcport, webport int) (DI, error) {
	targetfullname, e := name.MakeFullName(targetproject, targetgroup, targetapp)
	if e != nil {
		return nil, e
	}
	initKubernetesClient()
	d := &KubernetesD{
		target:        targetfullname,
		namespace:     namespace,
		labelselector: labelselector,
		crpcport:      crpcport,
		cgrpcport:     cgrpcport,
		webport:       webport,
		notices:       make(map[chan *struct{}]*struct{}, 1),
		status:        1,
	}
	go d.run()
	return d, nil
}
func (d *KubernetesD) Now() {
	if atomic.LoadInt32(&d.status) == 0 {
		d.Lock()
		defer d.Unlock()
		for notice := range d.notices {
			select {
			case notice <- nil:
			default:
			}
		}
	}
}

// don't close the returned channel,it will be closed in cases:
// 1.the cancel function be called
// 2.this discover stopped
func (d *KubernetesD) GetNotice() (notice <-chan *struct{}, cancel func()) {
	ch := make(chan *struct{}, 1)
	d.Lock()
	if status := atomic.LoadInt32(&d.status); status == 0 {
		select {
		case ch <- nil:
		default:
		}
		d.notices[ch] = nil
	} else if status == 1 {
		d.notices[ch] = nil
	} else {
		close(ch)
	}
	d.Unlock()
	return ch, func() {
		d.Lock()
		if _, ok := d.notices[ch]; ok {
			delete(d.notices, ch)
			close(ch)
		}
		d.Unlock()
	}
}
func (d *KubernetesD) GetAddrs(pt PortType) (map[string]*RegisterData, Version, error) {
	d.RLock()
	defer d.RUnlock()
	r := make(map[string]*RegisterData)
	reg := &RegisterData{
		DServers: map[string]*struct{}{"kubernetes": nil},
	}
	for addr := range d.addrs {
		switch pt {
		case NotNeed:
		case Crpc:
			if d.crpcport > 0 {
				if strings.Contains(addr, ":") {
					//ipv6
					addr = "[" + addr + "]:" + strconv.Itoa(d.crpcport)
				} else {
					//ipv4
					addr = addr + ":" + strconv.Itoa(d.crpcport)
				}
			}
		case Cgrpc:
			if d.cgrpcport > 0 {
				if strings.Contains(addr, ":") {
					//ipv6
					addr = "[" + addr + "]:" + strconv.Itoa(d.cgrpcport)
				} else {
					//ipv4
					addr = addr + ":" + strconv.Itoa(d.cgrpcport)
				}
			}
		case Web:
			if d.webport > 0 {
				if strings.Contains(addr, ":") {
					//ipv6
					addr = "[" + addr + "]:" + strconv.Itoa(d.webport)
				} else {
					//ipv4
					addr = addr + ":" + strconv.Itoa(d.webport)
				}
			}
		}
		r[addr] = reg
	}
	return r, d.version, d.lasterror
}
func (d *KubernetesD) Stop() {
	if atomic.SwapInt32(&d.status, 2) == 2 {
		return
	}
	if d.watcher != nil {
		d.watcher.Stop()
	}
}
func (d *KubernetesD) CheckTarget(target string) bool {
	return target == d.target
}
func (d *KubernetesD) run() {
	defer func() {
		d.Lock()
		for notice := range d.notices {
			delete(d.notices, notice)
			close(notice)
		}
		d.Unlock()
	}()
	d.list()
	d.watch()
}
func (d *KubernetesD) list() {
	for {
		if atomic.LoadInt32(&d.status) == 2 {
			log.Info(nil, "[discover.kubernetes] discover stopped", log.String("namespace", d.namespace), log.String("labelselector", d.labelselector))
			d.lasterror = cerror.ErrDiscoverStopped
			return
		}
		pods, e := kubeclient.CoreV1().Pods(d.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: d.labelselector})
		if e != nil {
			d.Lock()
			d.lasterror = e
			for notice := range d.notices {
				select {
				case notice <- nil:
				default:
				}
			}
			d.Unlock()
			log.Error(nil, "[discover.kubernetes] list failed", log.String("namespace", d.namespace), log.String("labelselector", d.labelselector), log.CError(e))
			time.Sleep(time.Millisecond * 100)
			continue
		}
		d.version = pods.ResourceVersion
		tmp := make(map[string]*struct{}, len(pods.Items))
		for _, item := range pods.Items {
			if item.Status.PodIP == "" {
				continue
			}
			if item.Status.Phase != v1.PodRunning {
				continue
			}
			find := false
			for _, condition := range item.Status.Conditions {
				if condition.Status == v1.ConditionTrue && condition.Type == v1.PodReady {
					find = true
					break
				}
			}
			if find {
				tmp[item.Status.PodIP] = nil
			}
		}
		d.Lock()
		d.addrs = tmp
		d.lasterror = nil
		atomic.StoreInt32(&d.status, 0)
		for notice := range d.notices {
			select {
			case notice <- nil:
			default:
			}
		}
		d.Unlock()
		break
	}
	return
}
func (d *KubernetesD) watch() {
	if d.lasterror != nil {
		//list failed
		return
	}
	var e error
	var retrayDealy time.Duration
	for {
		if retrayDealy != 0 {
			time.Sleep(retrayDealy)
		}
		if atomic.LoadInt32(&d.status) == 2 {
			log.Info(nil, "[discover.kubernetes] discover stopped", log.String("namespace", d.namespace), log.String("labelselector", d.labelselector))
			d.lasterror = cerror.ErrDiscoverStopped
			return
		}
		opts := metav1.ListOptions{LabelSelector: d.labelselector, Watch: true, ResourceVersion: d.version}
		if d.watcher, e = kubeclient.CoreV1().Pods(d.namespace).Watch(context.Background(), opts); e != nil {
			d.Lock()
			d.lasterror = e
			for notice := range d.notices {
				select {
				case notice <- nil:
				default:
				}
			}
			d.Unlock()
			log.Error(nil, "[discover.kubernetes] watch failed", log.String("namespace", d.namespace), log.String("labelselector", d.labelselector), log.CError(e))
			retrayDealy = time.Millisecond * 100
			continue
		}
		if atomic.LoadInt32(&d.status) == 2 {
			d.watcher.Stop()
			log.Info(nil, "[discover.kubernetes] discover stopped", log.String("namespace", d.namespace), log.String("labelselector", d.labelselector))
			d.lasterror = cerror.ErrDiscoverStopped
			return
		}
		failed := false
		for {
			event, ok := <-d.watcher.ResultChan()
			if !ok {
				if atomic.LoadInt32(&d.status) == 2 {
					log.Info(nil, "[discover.kubernetes] discover stopped", log.String("namespace", d.namespace), log.String("labelselector", d.labelselector))
					d.lasterror = cerror.ErrDiscoverStopped
					return
				}
				retrayDealy = 0
				break
			}
			switch event.Type {
			case watch.Added:
				fallthrough
			case watch.Modified:
				pod := event.Object.(*v1.Pod)
				if pod.Status.PodIP == "" {
					break
				}
				if pod.Status.Phase != v1.PodRunning {
					break
				}
				find := false
				for _, condition := range pod.Status.Conditions {
					if condition.Status == v1.ConditionTrue && condition.Type == v1.PodReady {
						find = true
						break
					}
				}
				if find {
					d.Lock()
					d.addrs[pod.Status.PodIP] = nil
					d.version = pod.ResourceVersion
					d.lasterror = nil
					for notice := range d.notices {
						select {
						case notice <- nil:
						default:
						}
					}
					d.Unlock()
				}
			case watch.Deleted:
				pod := event.Object.(*v1.Pod)
				if pod.Status.PodIP == "" {
					break
				}
				d.Lock()
				delete(d.addrs, pod.Status.PodIP)
				d.version = pod.ResourceVersion
				d.lasterror = nil
				for notice := range d.notices {
					select {
					case notice <- nil:
					default:
					}
				}
				d.Unlock()
			case watch.Bookmark:
				// don't update the version
				// d.version = event.Object.(interface{ GetResourceVersion() string }).GetResourceVersion()
			case watch.Error:
				failed = true
				d.Lock()
				d.lasterror = apierrors.FromObject(event.Object)
				for notice := range d.notices {
					select {
					case notice <- nil:
					default:
					}
				}
				d.Unlock()
				log.Error(nil, "[discover.kubernetes] watch failed", log.CError(e))
				if e, ok := d.lasterror.(*apierrors.StatusError); ok && e.ErrStatus.Details != nil {
					retrayDealy = time.Duration(e.ErrStatus.Details.RetryAfterSeconds) * time.Second
				} else {
					retrayDealy = 0
				}
				break
			}
			if failed {
				break
			}
		}
		d.watcher.Stop()
	}
}
