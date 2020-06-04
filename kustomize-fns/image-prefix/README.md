# Change image prefix

This is transform used to change the prefix of images. It works by adding an annotation
which is a map of prefixes: e.g.

```
annotations:
  "image-prefix.kubeflow.org": '{"docker.io": "gcr.io/myproject"}'
```

The transform will  then look for all images in that resource with prefix docker.io and change
it to gcr.io/myproject.

In practice you will typically add the annotation using kustomize's commonAnnotations
transform.
