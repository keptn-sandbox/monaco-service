# Release Notes 0.2.1

* Implementing [#5](https://github.com/keptn-sandbox/monaco-service/issues/5)
* Implementing [#6](https://github.com/keptn-sandbox/monaco-service/issues/6)
* Set distributor image to version 0.2.1

## Fixing issue with referencing configurations
This release fixes an issue that prevented the correct processing of referenced configurations, e.g: when you try to create a dashboard that references a management zone it would have resulted in an error. This is now fixed as described in [#5](https://github.com/keptn-sandbox/monaco-service/issues/5)

## Passing labels as environment variables
Additionally to KEPTN_PROJECT, KEPTN_STAGE & KEPTN_SERVICE the monaco service now also passes every label that is sent as part of the configuration-changed event as an environment variable. If there is a label with the name `createdby` and the value is `student123` then this would result in an environment variable `KEPTN_LABEL_CREATEDBY=student123`