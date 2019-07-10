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
	SUT := newController(doubles)

	runController(t, SUT, time.Second * 5, doubles.stopCh)

	waitUntilThereAreNActions(doubles.client, 3) // list, watch, create

	action := doubles.client.Actions()[2] // last create is what we want

	assertNamespaceCreatedWithName(t, stringutils.Reverse(namespaces[0].Name), action)

	close(doubles.stopCh)
}

func TestDeletesReverseNamespaces(t *testing.T) {
	namespaces := []*coreApi.Namespace{
		newNamespace("test"),
		newNamespace("tset"),
	}
	doubles := newDoubles(namespaces)
	SUT := newController(doubles)

	runController(t, SUT, time.Second * 5, doubles.stopCh)

	waitUntilThereAreNActions(doubles.client, 2) // list, watch

	_ = doubles.client.CoreV1().Namespaces().Delete(namespaces[0].Name, &metaApi.DeleteOptions{})

	waitUntilThereAreNActions(doubles.client, 4) // delete, delete

	action := doubles.client.Actions()[3] // last delete is what we want

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
func newController(doubles *doubles) *Controller {
	controller := NewController(doubles.client, doubles.informerFactory)
	controller.synced = func() bool { return true }
	controller.eventRecorder = &record.FakeRecorder{}

	return controller
}

func runController(t *testing.T, controller *Controller, timeout time.Duration, stopCh chan struct{}) {
	go func() {
		time.Sleep(timeout)
		close(stopCh)
		t.Error("Controller timed out")
	}()

	go func() {
		err := controller.Run(1, stopCh)
		assert.NoError(t, err )
	}()
}

func newNamespace(name string) *coreApi.Namespace {
	return &coreApi.Namespace{
		ObjectMeta: metaApi.ObjectMeta{
			Name: name,
		},
	}
}

func waitUntilThereAreNActions(client *kubefake.Clientset, n int)  {
	stopCh := make(chan struct{})
	thereAreNActions := func() {
		if len(client.Actions()) == n {
			close(stopCh)
		}
	}
	wait.Until(thereAreNActions, time.Millisecond*100, stopCh)
}

