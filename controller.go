package main

import (
	crdv1beta1 "controller-demo/pkg/apis/stable/v1beta1"
	informers "controller-demo/pkg/client/informers/externalversions/stable/v1beta1"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

type Controller struct {
	informer informers.CronTabInformer
	queue    workqueue.RateLimitingInterface
}

func NewController(informer informers.CronTabInformer) *Controller {
	controller := &Controller{
		informer: informer,
		queue:    workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Crontab"),
	}
	klog.Info("Setting up crontab controller")

	informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.onAdd,
		UpdateFunc: controller.onUpdate,
		DeleteFunc: controller.onDelete,
	})
	return controller
}

// 控制循环
func (c *Controller) Run(threadindess int, stopCh <-chan struct{}) error {
	// 控制循环crash的处理
	defer runtime.HandleCrash()
	// 控制器停止后关闭队列
	defer c.queue.ShutDown()

	// 启动控制器
	klog.Infof("Starting Crontab controller")

	// 等待数据同步完成
	if !cache.WaitForCacheSync(stopCh, c.informer.Informer().HasSynced) {
		runtime.HandleError(fmt.Errorf("time out waiting sysnc rescoure to cache"))
		return fmt.Errorf("time out waiting for caches to sync")
	}
	for i := 0; i < threadindess; i++ {
		// 直到收到stopch，循环停止, 否则 c.runWorker 消费处理函数。
		go wait.Until(c.runWorker, time.Second, stopCh)
	}
	<-stopCh
	klog.Infof("Stop Crontab controller")
	return nil
}

// 处理workqueue的元素
func (c *Controller) runWorker() {
	for c.proceessNextItem() {
		fmt.Println("get ok")
	}
}

// 实现业务逻辑
func (c *Controller) proceessNextItem() bool {
	// 从workqueue中取出itmes, shutDown标记符，表示队列是否关闭
	obj, shutdown := c.queue.Get()
	if shutdown {
		return false
	}

	// 根据key处理业务逻辑
	err := func(obj interface{}) error {
		// Done表示处理已经完成
		defer c.queue.Done(obj)

		var ok bool
		var key string
		if key, ok = obj.(string); !ok {
			c.queue.Forget(obj)
			return fmt.Errorf("expected string in workqueue but get %#v", obj)
		}
		if err := c.syncHandler(key); err != nil {
			return fmt.Errorf("sync error: %v", err)
		}
		c.queue.Forget(obj)
		klog.Infof("Successfully synced %s", key)
		return nil
	}(obj)
	if err != nil {
		// 更具业务如果处理失败，事件的任务从新回归队列。
		c.handleError(err, obj)
	}
	return true
}

func (c *Controller) handleError(err error, key interface{}) {
	if err == nil {
		c.queue.Forget(key)
		return
	}
	// 如果该任务出现问题的次数小于5次，则重新进入队列
	if c.queue.NumRequeues(key) < 5 {
		// 重新加入队列
		c.queue.AddRateLimited(key)
		return
	}
	// 超过五次都失败,不允许继续重试
	c.queue.Forget(key)
	runtime.HandleError(err)
}

// 业务处理逻辑
func (c *Controller) syncHandler(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	// 从index 中获取的
	crontab, err := c.informer.Lister().CronTabs(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			// 对应的 crontab 对象已经被删除
			klog.Warningf("Crontab delete: %s/%s", namespace, name)
			return nil
		}
		return err
	}

	klog.Infof("Crontab try to process: %#v...", crontab)
	return nil
}

func (c *Controller) onAdd(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		runtime.HandleError(err)
		return
	}
	c.queue.AddRateLimited(key)
}

func (c *Controller) onUpdate(old, new interface{}) {
	oldObj := old.(*crdv1beta1.CronTab)
	newObj := new.(*crdv1beta1.CronTab)

	if oldObj.ResourceVersion == newObj.ResourceVersion {
		return
	}
	c.onAdd(new)
}

func (c *Controller) onDelete(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		runtime.HandleError(err)
		return
	}
	c.queue.AddRateLimited(key)
}
