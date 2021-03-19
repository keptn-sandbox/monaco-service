Release Notes 0.8.1

This release allows you to use the Monaco Service with Keptn 0.8.
No additional features have been added. It was just lifted to support Keptn 0.8 and the new eventing

To use it with Keptn 0.8 simply add a "monaco" task to your sequence. Here is an example shipyard for Quality Gates where monaco will be called before the quality gates are evaluated
```
apiVersion: "spec.keptn.sh/0.2.0"
kind: "Shipyard"
metadata:
  name: "shipyard-quality-gates"
spec:
  stages:
  - name: "quality-gate"
    sequences:
      - name: evaluation
        tasks:
        - name: monaco
        - name: evaluation
```
