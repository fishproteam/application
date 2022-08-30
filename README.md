# application
We've been thinking about what an application is, and an application can be a Git repository or it can be a Helm Chart. After investigation, we believe that the application is a combination of resources, any resources in the namespace can be used as a part of the application. This operator provides an application controller and aggregates application status based on the resources contained in the application

## Description
Application monitors changes in Application, Deployment, StatefuleSet and Pod resources, adjusts applications, and aggregates application states

## Getting Started
You’ll need a Kubernetes cluster to run against. You can use [KIND](https://sigs.k8s.io/kind) to get a local cluster for testing, or run against a remote cluster.
**Note:** Your controller will automatically use the current context in your kubeconfig file (i.e. whatever cluster `kubectl cluster-info` shows).

### Running on the cluster
1. Install Instances of Custom Resources:

```sh
kubectl apply -f config/samples/
```

2. Build and push your image to the location specified by `IMG`:

```sh
make docker-build docker-push IMG=<some-registry>/application:tag
```

3. Deploy the controller to the cluster with the image specified by `IMG`:

```sh
make deploy IMG=<some-registry>/application:tag
```

### Uninstall CRDs
To delete the CRDs from the cluster:

```sh
make uninstall
```

### Undeploy controller
UnDeploy the controller to the cluster:

```sh
make undeploy
```

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

### How it works
This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/) 
which provides a reconcile function responsible for synchronizing resources untile the desired state is reached on the cluster 

### Test It Out
1. Install the CRDs into the cluster:

```sh
make install
```

2. Run your controller (this will run in the foreground, so switch to a new terminal if you want to leave it running):

```sh
make run
```

**NOTE:** You can also run this in one step by running: `make install run`

### Modifying the API definitions
If you are editing the API definitions, generate the manifests such as CRs or CRDs using:

```sh
make manifests
```

**NOTE:** Run `make --help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## example

Deploy a simple application
```sh
~ cat << EOF | kubectl create -f -
apiVersion: applications.app.io/v1beta1
kind: Application
metadata:
  name: application-sample
spec:
  revisionHistoryLimit: 10
  resources:
    - apiVersion: apps/v1
      kind: Deployment
      metadata:
        labels:
          ccnp.cib/app: nginx-deploy
        name: nginx-deploy
        namespace: l002x0
      spec:
        replicas: 2
        selector:
          matchLabels:
            ccnp.cib/component: nginx-deploy
        template:
          metadata:
            labels:
              ccnp.cib/app: nginx-deploy
              ccnp.cib/component: nginx-deploy
          spec:
            containers:
              - image: nginx:latest
                imagePullPolicy: Always
                name: nginx-deploy
                resources:
                  limits:
                    cpu: 128m
                    memory: "67108864"
                  requests:
                    cpu: 64m
                    memory: "67108864"
    - apiVersion: v1
      kind: Service
      metadata:
        labels:
          ccnp.cib/app: nginx-deploy
        name: nginx-deploy
        namespace: l002x0
      spec:
        ports:
          - name: nginx-deploy
            port: 80
            protocol: TCP
            targetPort: 80
        selector:
          ccnp.cib/component: nginx-deploy
        sessionAffinity: None
        type: ClusterIP
  selector: {}
  descriptor:
    type: "nginx"
    keywords:
      - "cms"
      - "blog"
    links:
      - description: About
        url: "https://wordpress.org/"
      - description: Web Server Dashboard
        url: "https://metrics/internal/wordpress-01/web-app"
      - description: Mysql Dashboard
        url: "https://metrics/internal/wordpress-01/mysql"
    version: "4.9.4"
    description: "WordPress is open source software you can use to create a beautiful website, blog, or app."
    icons:
      - src: "https://s.w.org/style/images/about/WordPress-logotype-wmark.png"
        type: "image/png"
        size: "1000x1000"
      - src: "https://s.w.org/style/images/about/WordPress-logotype-standard.png"
        type: "image/png"
        size: "2000x680"
    maintainers:
      - name: Wordpress Dev
        email: dev@wordpress.org
    owners:
      - name: Wordpress Admin
        email: admin@wordpress.org
EOF
application.applications.app.io/application-sample created
~ kubectl get all
NAME                                READY   STATUS    RESTARTS   AGE
pod/nginx-deploy-78689459cd-dswfv   1/1     Running   0          60s
pod/nginx-deploy-78689459cd-gl7vm   1/1     Running   0          60s

NAME                   TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)   AGE
service/kubernetes     ClusterIP   10.96.0.1       <none>        443/TCP   92m
service/nginx-deploy   ClusterIP   10.96.146.165   <none>        80/TCP    60s

NAME                           READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/nginx-deploy   2/2     2            2           60s

NAME                                      DESIRED   CURRENT   READY   AGE
replicaset.apps/nginx-deploy-78689459cd   2         2         2       60s

NAME                                                 TYPE    VERSION   READY   AGE
application.applications.app.io/application-sample   nginx   4.9.4     2/2     60s
```

## License

Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

