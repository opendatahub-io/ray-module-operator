# Ray module operator Helm chart

This chart contains the module-controller installation bundle only. KubeRay
operand manifests are managed separately by the Ray module operator.

The rendered installation bundle under `files/` is synchronized from the
existing Kustomize installation with:

```sh
make helm-chart-sync
```

The Helm rendering supports overriding the controller image, replica count,
and installation namespace through `values.yaml`. The namespace is intentionally
not managed as a chart resource; use `--create-namespace` or create it before
installing.
