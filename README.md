# Prometheus Metrics Based HPA

There are a few solutions for Prometheus based scaling [Keda](https://keda.sh/),
[prometheus-adapter](https://github.com/kubernetes-sigs/prometheus-adapter), and 
[kube-metrics-adapter](https://github.com/zalando-incubator/kube-metrics-adapter).

After reviewing and trying to implement these `kube-metrics-adapter` was more my style.  
That style being something that is easy to understand and setup. There are 4 steps to get
HPA working with metrics

1. Deploy a sample queue application
2. Install Prometheus configured to scrape metrics queue workers
3. Deploy the HPA configured to use external metric
4. Deploy kube-metrics-adapter

# Deploying A Queue & Worker

First step is installing an example deployment containing a worker that pulls 
items off a queue at a default rate of 5 per second.

The worker contains 2 http POST endpoints. /add will add the specified
number of items to the queue.  /speed can control the rate at which 
the worker removes items from the queue in milliseconds.

```bash
kubectl apply -f kube/redis.yaml
kubectl apply -f kube/worker.yaml
```

Add some items to the queue by getting a bash shell on a worker

```bash
kubectl exec -it worker-76bf85f569-zdwkd sh
$ curl -d "200" -X POST http://localhost:8011/add
```

And optionally the endpoint to control worker speed

```bash
 curl -d "5000" -X POST http://localhost:8011/speed
```

Will see the worker completing items and the queue depth in the logs

```bash
kubectl logs -f worker-76bf85f569-zdwkd
2022/10/28 22:31:54 Queue depth: 184
2022/10/28 22:31:54 Completed work on 17
2022/10/28 22:31:59 Completed work on 18
```

# Deploy Prometheus & Verify Metrics

The Promethues community [helm chart](https://github.com/prometheus-community/helm-charts/tree/main/charts/prometheus)
is configured to collect metrics from pods out of the box.  The worker deployment
contains annotations for Prometheus to use.

```yaml
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8011"
```

Worker exposes a gauge called `potato_queue_depth` that is the size of the redis queue. I'm installing 
prometheus in the same `default` minikube namespace.

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
helm install prom prometheus-community/prometheus
```

Add some more items to the queue using the curl commend in the worker shell.  To verify
the metric is being collected we'll forward port 9090 to Prometheus

```bash
kubectl port-forward service/prom-prometheus-server 9090:80
```

Open a browser to http://localhost:9090 and verify Prometheus has collected the `potato_queue_depth` metric.

# Deploy the HorizontalPodAutoscaler

`kube-metrics-server` uses annotations on the HPA object to determine what metrics will be exposed in the metrics api.  When the hpa
starts scaling workers they will all report the same `potato_queue_depth` metric. This is why we're using the average 
of all reported queue depths as the measurement in hpa.yaml

```yaml
    metric-config.external.potato-queue-depth.prometheus/query: |
      avg(potato_queue_depth)
    metric-config.external.potato-queue-depth.prometheus/interval: "10s" # optional
```

Apply the hpa.yaml targeting deployment/worker

```bash
kubectl apply -f kube/hpa.yaml
```

The hpa will be created but the metric will be <unknown>

```bash
kubcctl get hpa
NAME         REFERENCE           TARGETS         MINPODS   MAXPODS   REPLICAS   AGE
parser-hpa   Deployment/worker   <unknown>/100   1         10        1          11m
```

The metric once known to the HPA will affect scaling using the following formula.

currentRelicas * (metricValue / targetValue)

Essentially targeting 100 items per worker. Our targetValue is 100 so a queue depth of 
200 would result in desired replicas of 2.

1 * (200 / 100) = 2

# Deploy kube-metrics-adapter

The `KMA` reads the metric annotations in the HPA and queries those metrics from Prometheus.  KMA then exposes those
metrics in the external metrics api for Kubernetes and the autoscaler to see. The KMA deployment has an argument
telling the service the Prometheus address.

```yaml
        args:
        - --prometheus-server=http://prom-prometheus-server.default.svc.cluster.local
```

See the kube-metrics-adapter repository for latest deployment files.  I've included the 4 required files for a
working deployment.  This is installed in the `kube-system` namespace.

```
kubectl apply -f kube/kma
```

Once KMA has found the HPA and collected some metrics can be seen at the api endpoint.

```bash
kubectl get --raw /apis/external.metrics.k8s.io/v1beta1
```

After a few minutes if all goes well HPA now has a metric to use for calculating the number of replicas.  The value is 
sometimes displayed in millis or queue depth * 1000.  704400m is about 704 items in the queue and can see HPA has 
scaled to 5 replicas.

```bash
[minikube|default] dustins-air-5:scaling-potato dustin$ kubectl get hpa
NAME         REFERENCE           TARGETS       MINPODS   MAXPODS   REPLICAS   AGE
worker-hpa   Deployment/worker   704400m/100   1         10        5          4m53s
```

# References

https://livewyer.io/blog/2019/05/28/horizontal-pod-autoscaling/

https://ryanbaker.io/2019-10-07-scaling-rabbitmq-on-k8s/

https://www.youtube.com/watch?v=iodq-4srXA8&t=672s
