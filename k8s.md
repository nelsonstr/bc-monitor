

* k8s
health checks

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: blockchain-monitor
spec:
  # ... other deployment specs ...
  template:
    spec:
      containers:
      - name: blockchain-monitor
        image: your-image:tag
        ports:
        - containerPort: 8080
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 15
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
```

* prometheus metric url
```http

GET http://127.0.0.1:2112/metrics

```