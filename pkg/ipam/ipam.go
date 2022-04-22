package ipam

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	goipam "github.com/metal-stack/go-ipam"
)

var (
	ipamLog = ctrl.Log.WithName("ipam")
)

type ConfigMapIPAM struct {
	name   types.NamespacedName
	client client.Client

	lock sync.RWMutex
}

func NewConfigMapIPAM(name types.NamespacedName, stopCh <-chan struct{}) (*ConfigMapIPAM, error) {
	cmConfig := ctrl.GetConfigOrDie()
	cmCache, err := cache.New(cmConfig, cache.Options{
		Namespace: name.Namespace,
	})
	if err != nil {
		ipamLog.Error(err, "unable to create cache for configmap")
		return nil, err
	}
	cmClient, err := client.New(cmConfig, client.Options{})
	if err != nil {
		ipamLog.Error(err, "unable to create client for configmap")
		return nil, err
	}
	cmCacheClient := &client.DelegatingClient{
		Reader: &client.DelegatingReader{
			CacheReader:  cmCache,
			ClientReader: cmClient,
		},
		Writer:       cmClient,
		StatusClient: cmClient,
	}

	go func() {
		err := cmCache.Start(stopCh)
		if err != nil {
			ipamLog.Error(err, "unable to start cache for configmap")
		}
		<-stopCh
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	cacheOk := cmCache.WaitForCacheSync(ctx.Done())
	if !cacheOk {
		ipamLog.Error(err, "unable to wait for configmap cache")
		return nil, err
	}

	cm := &corev1.ConfigMap{}
	err = cmCacheClient.Get(context.Background(), types.NamespacedName{
		Name:      name.Name,
		Namespace: name.Namespace,
	}, cm)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			ipamLog.Error(err, "error getting ipam configmap")
			return nil, err
		}
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name.Name,
				Namespace: name.Namespace,
			},
			BinaryData: make(map[string][]byte),
		}
		err = cmCacheClient.Create(context.Background(), cm)
		if err != nil {
			ipamLog.Error(err, "error creating ipam configmap")
			return nil, err
		}
	}

	return &ConfigMapIPAM{
		name:   name,
		client: cmCacheClient,
	}, nil
}

func encode(prefix *goipam.Prefix) ([]byte, error) {
	return prefix.GobEncode()
}

func decode(b []byte) (*goipam.Prefix, error) {
	prefix := &goipam.Prefix{}
	err := prefix.GobDecode(b)
	return prefix, err
}

func getCmCIDR(cidr string) string {
	return strings.ReplaceAll(strings.ReplaceAll(cidr, "/", "_"), ":", "-")
}

func (c *ConfigMapIPAM) CreatePrefix(prefix goipam.Prefix) (goipam.Prefix, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.name.Name,
			Namespace: c.name.Namespace,
		},
	}
	err := c.client.Get(context.Background(), c.name, cm)
	if err != nil {
		return goipam.Prefix{}, err
	}
	data, ok := cm.BinaryData[getCmCIDR(prefix.Cidr)]
	if ok {
		p, err := decode(data)
		if err != nil {
			return goipam.Prefix{}, err
		}
		return *p, nil
	}

	data, err = encode(&prefix)
	if err != nil {
		return goipam.Prefix{}, err
	}

	patch := client.MergeFrom(cm.DeepCopy())
	if cm.BinaryData == nil {
		cm.BinaryData = make(map[string][]byte)
	}
	cm.BinaryData[getCmCIDR(prefix.Cidr)] = data

	err = c.client.Patch(context.Background(), cm, patch)
	if err != nil {
		return goipam.Prefix{}, err
	}

	return prefix, nil
}

func (c *ConfigMapIPAM) ReadPrefix(prefix string) (goipam.Prefix, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	cm := &corev1.ConfigMap{}
	err := c.client.Get(context.Background(), c.name, cm)
	if err != nil {
		return goipam.Prefix{}, err
	}
	data, ok := cm.BinaryData[getCmCIDR(prefix)]
	if !ok {
		return goipam.Prefix{}, fmt.Errorf("prefix %s not found", prefix)
	}

	newPrefix, err := decode(data)
	if err != nil {
		return goipam.Prefix{}, err
	}

	return *newPrefix, nil
}

func (c *ConfigMapIPAM) ReadAllPrefixes() ([]goipam.Prefix, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	cm := &corev1.ConfigMap{}
	err := c.client.Get(context.Background(), c.name, cm)
	if err != nil {
		return nil, err
	}

	ps := make([]goipam.Prefix, 0, len(cm.BinaryData))
	for _, v := range cm.BinaryData {
		p, err := decode(v)
		if err != nil {
			return nil, err
		}
		ps = append(ps, *p)
	}
	return ps, nil
}

func (c *ConfigMapIPAM) UpdatePrefix(prefix goipam.Prefix) (goipam.Prefix, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	cm := &corev1.ConfigMap{}
	err := c.client.Get(context.Background(), c.name, cm)
	if err != nil {
		return goipam.Prefix{}, err
	}

	if prefix.Cidr == "" {
		return goipam.Prefix{}, fmt.Errorf("prefix not present:%v", prefix)
	}

	_, ok := cm.BinaryData[getCmCIDR(prefix.Cidr)]
	if !ok {
		return goipam.Prefix{}, fmt.Errorf("prefix %s not found", prefix.Cidr)
	}

	data, err := encode(&prefix)
	if err != nil {
		return goipam.Prefix{}, err
	}

	patch := client.MergeFrom(cm.DeepCopy())
	cm.BinaryData[getCmCIDR(prefix.Cidr)] = data

	return prefix, c.client.Patch(context.Background(), cm, patch)
}

func (c *ConfigMapIPAM) DeletePrefix(prefix goipam.Prefix) (goipam.Prefix, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	cm := &corev1.ConfigMap{}
	err := c.client.Get(context.Background(), c.name, cm)
	if err != nil {
		return goipam.Prefix{}, err
	}

	_, ok := cm.BinaryData[getCmCIDR(prefix.Cidr)]
	if !ok {
		return prefix, nil
	}
	patch := client.MergeFrom(cm.DeepCopy())
	delete(cm.BinaryData, getCmCIDR(prefix.Cidr))

	return prefix, c.client.Patch(context.Background(), cm, patch)
}
