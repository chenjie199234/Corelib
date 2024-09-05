package discover

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chenjie199234/Corelib/cerror"
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
	fieldselector string
	labelselector string
	watcher       watch.Interface
	crpcport      int
	cgrpcport     int
	webport       int
	ctx           context.Context
	cancel        context.CancelFunc

	lker      *sync.RWMutex
	notices   map[chan *struct{}]*struct{}
	addrs     map[string]*struct{}
	version   string
	lasterror error
}

var lker sync.Mutex
var kubeclient *kubernetes.Clientset

func initKubernetesClient() error {
	lker.Lock()
	defer lker.Unlock()
	if kubeclient != nil {
		return nil
	}
	if c, e := rest.InClusterConfig(); e != nil {
		return e
	} else if kubeclient, e = kubernetes.NewForConfig(c); e != nil {
		return e
	}
	return nil
}

func NewKubernetesDiscover(targetproject, targetgroup, targetapp, namespace, fieldselector, labelselector string, crpcport, cgrpcport, webport int) (DI, error) {
	targetfullname, e := name.MakeFullName(targetproject, targetgroup, targetapp)
	if e != nil {
		return nil, e
	}
	if e := initKubernetesClient(); e != nil {
		return nil, e
	}
	d := &KubernetesD{
		target:        targetfullname,
		namespace:     namespace,
		fieldselector: fieldselector,
		labelselector: labelselector,
		crpcport:      crpcport,
		cgrpcport:     cgrpcport,
		webport:       webport,
		notices:       make(map[chan *struct{}]*struct{}, 1),
		lker:          &sync.RWMutex{},
	}
	d.ctx, d.cancel = context.WithCancel(context.Background())
	go d.run()
	return d, nil
}
func (d *KubernetesD) Now() {
	d.lker.RLock()
	defer d.lker.RUnlock()
	if d.version == "" {
		return
	}
	for notice := range d.notices {
		select {
		case notice <- nil:
		default:
		}
	}
}
func (d *KubernetesD) Stop() {
	d.cancel()
	if d.watcher != nil {
		d.watcher.Stop()
	}
}

// don't close the returned channel,it will be closed in cases:
// 1.the cancel function be called
// 2.this discover stopped
func (d *KubernetesD) GetNotice() (notice <-chan *struct{}, cancel func()) {
	ch := make(chan *struct{}, 1)
	d.lker.Lock()
	if d.version != "" {
		ch <- nil
		d.notices[ch] = nil
	} else {
		select {
		case <-d.ctx.Done():
			close(ch)
		default:
			d.notices[ch] = nil
		}
	}
	d.lker.Unlock()
	return ch, func() {
		d.lker.Lock()
		if _, ok := d.notices[ch]; ok {
			delete(d.notices, ch)
			close(ch)
		}
		d.lker.Unlock()
	}
}
func (d *KubernetesD) GetAddrs(pt PortType) (map[string]*RegisterData, Version, error) {
	d.lker.RLock()
	defer d.lker.RUnlock()
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
func (d *KubernetesD) CheckTarget(target string) bool {
	return target == d.target
}
func (d *KubernetesD) run() {
	defer func() {
		d.lker.Lock()
		for notice := range d.notices {
			delete(d.notices, notice)
			close(notice)
		}
		d.lker.Unlock()
	}()
	d.list()
	d.watch()
}
func (d *KubernetesD) list() {
	for {
		pods, e := kubeclient.CoreV1().Pods(d.namespace).List(d.ctx, metav1.ListOptions{LabelSelector: d.labelselector, FieldSelector: d.fieldselector})
		if e != nil {
			if cerror.Equal(e, cerror.ErrCanceled) {
				slog.InfoContext(nil, "[discover.kubernetes] discover stopped",
					slog.String("target", d.target),
					slog.String("namespace", d.namespace),
					slog.String("labelselector", d.labelselector),
					slog.String("fieldselector", d.fieldselector))
				d.lasterror = cerror.ErrDiscoverStopped
				return
			}
			slog.ErrorContext(nil, "[discover.kubernetes] list failed",
				slog.String("target", d.target),
				slog.String("namespace", d.namespace),
				slog.String("labelselector", d.labelselector),
				slog.String("fieldselector", d.fieldselector),
				slog.String("error", e.Error()))
			d.lker.Lock()
			d.lasterror = e
			for notice := range d.notices {
				select {
				case notice <- nil:
				default:
				}
			}
			d.lker.Unlock()
			time.Sleep(time.Millisecond * 100)
			continue
		}
		addrs := make(map[string]*struct{}, len(pods.Items))
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
				addrs[item.Status.PodIP] = nil
			}
		}
		d.lker.Lock()
		d.version = pods.ResourceVersion
		d.addrs = addrs
		d.lasterror = nil
		for notice := range d.notices {
			select {
			case notice <- nil:
			default:
			}
		}
		d.lker.Unlock()
		break
	}
	return
}
func (d *KubernetesD) watch() {
	var e error
	var retrayDealy time.Duration
	for {
		if retrayDealy != 0 {
			time.Sleep(retrayDealy)
		}
		opts := metav1.ListOptions{
			LabelSelector:        d.labelselector,
			FieldSelector:        d.fieldselector,
			Watch:                true,
			ResourceVersion:      d.version,
			ResourceVersionMatch: metav1.ResourceVersionMatchNotOlderThan,
		}
		if d.watcher, e = kubeclient.CoreV1().Pods(d.namespace).Watch(d.ctx, opts); e != nil {
			if cerror.Equal(e, cerror.ErrCanceled) {
				slog.InfoContext(nil, "[discover.kubernetes] discover stopped",
					slog.String("target", d.target),
					slog.String("namespace", d.namespace),
					slog.String("labelselector", d.labelselector),
					slog.String("fieldselector", d.fieldselector))
				d.lasterror = cerror.ErrDiscoverStopped
				return
			}
			slog.ErrorContext(nil, "[discover.kubernetes] watch failed",
				slog.String("target", d.target),
				slog.String("namespace", d.namespace),
				slog.String("labelselector", d.labelselector),
				slog.String("fieldselector", d.fieldselector),
				slog.String("error", e.Error()))
			d.lker.Lock()
			d.lasterror = e
			for notice := range d.notices {
				select {
				case notice <- nil:
				default:
				}
			}
			d.lker.Unlock()
			retrayDealy = time.Millisecond * 100
			continue
		}
		failed := false
		for {
			var event watch.Event
			var ok bool
			select {
			case <-d.ctx.Done():
				slog.InfoContext(nil, "[discover.kubernetes] discover stopped",
					slog.String("target", d.target),
					slog.String("namespace", d.namespace),
					slog.String("labelselector", d.labelselector),
					slog.String("fieldselector", d.fieldselector))
				d.lasterror = cerror.ErrDiscoverStopped
				return
			case event, ok = <-d.watcher.ResultChan():
				if !ok {
					select {
					case <-d.ctx.Done():
						slog.InfoContext(nil, "[discover.kubernetes] discover stopped",
							slog.String("target", d.target),
							slog.String("namespace", d.namespace),
							slog.String("labelselector", d.labelselector),
							slog.String("fieldselector", d.fieldselector))
						d.lasterror = cerror.ErrDiscoverStopped
						return
					default:
						//error happened
						failed = true
					}
				}
			}
			if failed {
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
					d.lker.Lock()
					d.addrs[pod.Status.PodIP] = nil
					d.version = pod.ResourceVersion
					d.lasterror = nil
					for notice := range d.notices {
						select {
						case notice <- nil:
						default:
						}
					}
					d.lker.Unlock()
				}
			case watch.Deleted:
				pod := event.Object.(*v1.Pod)
				if pod.Status.PodIP == "" {
					break
				}
				d.lker.Lock()
				delete(d.addrs, pod.Status.PodIP)
				d.version = pod.ResourceVersion
				d.lasterror = nil
				for notice := range d.notices {
					select {
					case notice <- nil:
					default:
					}
				}
				d.lker.Unlock()
			case watch.Bookmark:
				d.version = event.Object.(interface{ GetResourceVersion() string }).GetResourceVersion()
			case watch.Error:
				failed = true
				e = apierrors.FromObject(event.Object)
				slog.ErrorContext(nil, "[discover.kubernetes] watch failed",
					slog.String("target", d.target),
					slog.String("namespace", d.namespace),
					slog.String("labelselector", d.labelselector),
					slog.String("fieldselector", d.fieldselector),
					slog.String("error", e.Error()))
				d.lker.Lock()
				d.lasterror = e
				for notice := range d.notices {
					select {
					case notice <- nil:
					default:
					}
				}
				d.lker.Unlock()
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
