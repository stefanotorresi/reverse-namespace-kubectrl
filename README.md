Reversed namespaces
===================

This exercise consists in creating a controller that
reverses all namespaces in the cluster. There's no need to perform any
further action inside the namespaces.

* When a namespace is created, a namespace with the reverse name is
  created.
* When a namespace is deleted, a namespace with the reverse name is
  deleted.

After creating a cluster and executing the controller, we
should see something like the following:

```
~ > kubectl get namespaces
NAME              STATUS   AGE
cilbup-ebuk       Active   21m
default           Active   65m
esael-edon-ebuk   Active   21m
kube-node-lease   Active   65m
kube-public       Active   65m
kube-system       Active   65m
metsys-ebuk       Active   21m
tluafed           Active   21m
```

The namespaces that were created by default were: `default`,
`kube-node-lease`, `kube-public` and `kube-system`.

Our controller listed the namespaces and as a result of that created:
`tlaufed`, `esael-edon-ebuk`, `cilbup-ebuk` and `metsys-ebuk`.

When the controller is running, any creation or deletion of a
namespace should result in the appropriate action. For example, after
running `kubectl create namespace namespace-test`, we should
immediately see:

```
~ > kubectl get namespaces
NAME              STATUS   AGE
cilbup-ebuk       Active   32m
default           Active   76m
esael-edon-ebuk   Active   32m
kube-node-lease   Active   76m
kube-public       Active   76m
kube-system       Active   76m
metsys-ebuk       Active   32m
namespace-test    Active   2s
tluafed           Active   32m
tset-ecapseman    Active   2s
```

After performing a `kubectl delete namespace namespace-test`, we
should immediately see:

```
~ > kubectl get namespaces
NAME              STATUS        AGE
cilbup-ebuk       Active        33m
default           Active        77m
esael-edon-ebuk   Active        33m
kube-node-lease   Active        77m
kube-public       Active        77m
kube-system       Active        77m
metsys-ebuk       Active        33m
tluafed           Active        33m
tset-ecapseman    Terminating   52s
```

Eventually, the namespace will be completely deleted, and will not be
present in the list of namespaces:

```
~ > kubectl get namespaces
NAME              STATUS   AGE
cilbup-ebuk       Active   33m
default           Active   77m
esael-edon-ebuk   Active   33m
kube-node-lease   Active   77m
kube-public       Active   77m
kube-system       Active   77m
metsys-ebuk       Active   33m
tluafed           Active   33m
```

For this task, feel free to use any resources from the official
documentation or any other online resource, we will go through the
source code together.

Note: it's not required for the controller to run inside the cluster,
as long as it performs the desired logic.
