package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	coreApi "k8s.io/api/core/v1"
	metaApi "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	coreInformes "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	kubetesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/record"

	"reversed-namespaces-kubectrl/pkg/stringutils"
)

type doubles struct {
	informerFactory informers.SharedInformerFactory
	informer		coreInformes.NamespaceInformer
	client          *kubefake.Clientset
	clientObjects   []runtime.Object
	stopCh			chan struct{}
}

func TestCreatesReverseNamespaces(t *testing.T) {
	namespaces := []*coreApi.Namespace{newNamespace("test")}
	doubles := newDoubles(namespaces)
	SUT := newController(doubles.client, doubles.informer)

	runController(t, doubles, SUT, time.Second * 10)

	waitUntilThereAreNActions(doubles.client, 3)

	action := doubles.client.Actions()[2]

	assertNamespaceCreatedWithName(t, stringutils.Reverse(namespaces[0].Name), action)

	close(doubles.stopCh)
}

func TestDeletesReverseNamespaces(t *testing.T) {
	namespaces := []*coreApi.Namespace{
		newNamespace("test"),
		newNamespace("tset"),
	}
	doubles := newDoubles(namespaces)
	SUT := newController(doubles.client, doubles.informer)

	runController(t, doubles, SUT, time.Second * 5)

	waitUntilThereAreNActions(doubles.client, 2) // wait for list and watch

	_ = doubles.client.CoreV1().Namespaces().Delete("test", &metaApi.DeleteOptions{})

	waitUntilThereAreNActions(doubles.client, 4)

	action := doubles.client.Actions()[3]

	assertNamespaceDeleted(t, namespaces[1], action)

	close(doubles.stopCh)
}

func assertNamespaceCreatedWithName(t *testing.T, name string, action kubetesting.Action) {
	assert.True(t, action.Matches("create", "namespaces"))
	assert.Implements(t, (*kubetesting.CreateAction)(nil), action)

	var createAction kubetesting.CreateAction
	createAction = action.(kubetesting.CreateAction)
	namespace := createAction.GetObject().(*coreApi.Namespace)
	assert.Equal(t, name, namespace.Name)
}

func assertNamespaceDeleted(t *testing.T, namespace *coreApi.Namespace, action kubetesting.Action) {
	assert.True(t, action.Matches("delete", "namespaces"))
	assert.Implements(t, (*kubetesting.DeleteAction)(nil), action)

	var deleteAction kubetesting.DeleteAction
	deleteAction = action.(kubetesting.DeleteAction)
	assert.Equal(t, namespace.Name, deleteAction.GetName())
}

func waitUntilThereAreNActions(client *kubefake.Clientset, n int)  {
	stopCh := make(chan struct{})
	wait.Until(func() {
		if len(client.Actions()) == n {
			close(stopCh)
		}
	}, time.Millisecond*200, stopCh)
}

func newDoubles(namespaces []*coreApi.Namespace) *doubles {
	clientObjects := make([]runtime.Object, len(namespaces))

	for i, namespace := range namespaces {
		clientObjects[i] = namespace
	}

	client := fake.NewSimpleClientset(clientObjects...)
	informerFactory := informers.NewSharedInformerFactory(client, 0)
	informer := informerFactory.Core().V1().Namespaces()

	f := &doubles{
		informer:        informer,
		informerFactory: informerFactory,
		client:          client,
		clientObjects:   clientObjects,
		stopCh: 		 make(chan struct{}),
	}

	return f
}

func newNamespace(name string) *coreApi.Namespace {
	return &coreApi.Namespace{
		ObjectMeta: metaApi.ObjectMeta{
			Name: name,
		},
	}
}

func newController(client *kubefake.Clientset, informer	coreInformes.NamespaceInformer) *Controller {
	controller := NewController(client, informer)
	controller.synced = func() bool { return true }
	controller.eventRecorder = &record.FakeRecorder{}

	return controller
}

func runController(t *testing.T, doubles *doubles, controller *Controller, timeout time.Duration) {
	go func() {
		time.Sleep(timeout)
		close(doubles.stopCh)
		t.Error("Controller timed out")
	}()

	go func() {
		doubles.informerFactory.Start(doubles.stopCh)
		err := controller.Run(1, doubles.stopCh)
		assert.NoError(t, err )
	}()
}
