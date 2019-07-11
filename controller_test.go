package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	coreApi "k8s.io/api/core/v1"
	metaApi "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	kubetesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/record"

	"reverse-namespace-kubectrl/pkg/stringutils"
)

const timeout = time.Second * 5

type helpers struct {
	t *testing.T
	informerFactory informers.SharedInformerFactory
	client          *kubefake.Clientset
	clientObjects   []runtime.Object
}

func TestCreatesReverseNamespaces(t *testing.T) {
	namespaces := []*coreApi.Namespace{newNamespace("test")}
	helpers := newHelpers(t, namespaces)
	SUT := helpers.newController()

	helpers.runController(SUT)

	helpers.waitUntilThereAreNActions(3) // list, watch, create

	actions := helpers.client.Actions()
	action := actions[2] // last create is what we want

	helpers.assertNamespaceCreatedWithName(stringutils.Reverse(namespaces[0].Name), action)
}

func TestDeletesReverseNamespaces(t *testing.T) {
	namespaces := []*coreApi.Namespace{
		newNamespace("test"),
		newNamespace("tset"),
	}
	helpers := newHelpers(t, namespaces)
	SUT := helpers.newController()

	helpers.runController(SUT)

	helpers.waitUntilThereAreNActions(2) // list, watch

	_ = helpers.client.CoreV1().Namespaces().Delete(namespaces[0].Name, &metaApi.DeleteOptions{})

	helpers.waitUntilThereAreNActions(4) // delete, delete

	actions := helpers.client.Actions()
	action := actions[3] // last delete is what we want

	helpers.assertNamespaceDeleted(namespaces[1], action)
}

func newHelpers(t *testing.T, namespaces []*coreApi.Namespace) *helpers {
	clientObjects := make([]runtime.Object, len(namespaces))

	for i, namespace := range namespaces {
		clientObjects[i] = namespace
	}

	client := fake.NewSimpleClientset(clientObjects...)
	informerFactory := informers.NewSharedInformerFactory(client, 0)

	f := &helpers{
		t:               t,
		informerFactory: informerFactory,
		client:          client,
		clientObjects:   clientObjects,
	}

	return f
}

func (h *helpers) assertNamespaceCreatedWithName(name string, action kubetesting.Action) {
	assert.True(h.t, action.Matches("create", "namespaces"))
	assert.Implements(h.t, (*kubetesting.CreateAction)(nil), action)

	var createAction kubetesting.CreateAction
	createAction = action.(kubetesting.CreateAction)
	namespace := createAction.GetObject().(*coreApi.Namespace)
	assert.Equal(h.t, name, namespace.Name)
}

func (h *helpers) assertNamespaceDeleted( namespace *coreApi.Namespace, action kubetesting.Action) {
	assert.True(h.t, action.Matches("delete", "namespaces"))
	assert.Implements(h.t, (*kubetesting.DeleteAction)(nil), action)

	var deleteAction kubetesting.DeleteAction
	deleteAction = action.(kubetesting.DeleteAction)
	assert.Equal(h.t, namespace.Name, deleteAction.GetName())
}

func (h *helpers) waitUntilThereAreNActions(n int) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	thereAreNActions := func() {
		actions := h.client.Actions()
		if len(actions) == n {
			cancel()
		}
	}

	wait.Until(thereAreNActions, time.Millisecond * 500, ctx.Done())

	if err := ctx.Err(); err != nil && err.Error() == "context deadline exceeded" {
		h.t.Error("not enough client actions recorded before timeout")
	}
}

func (h *helpers) newController() *Controller {
	controller := NewController(h.client, h.informerFactory)
	controller.synced = func() bool { return true }
	controller.eventRecorder = &record.FakeRecorder{}

	return controller
}

func (h *helpers) runController(controller *Controller) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	go func() {
		defer cancel()
		err := controller.Run(1, ctx.Done())
		assert.NoError(h.t, err)
	}()
}

func newNamespace(name string) *coreApi.Namespace {
	return &coreApi.Namespace{
		ObjectMeta: metaApi.ObjectMeta{
			Name: name,
		},
	}
}
