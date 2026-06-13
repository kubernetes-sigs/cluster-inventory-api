# Using Plugin OCI Images

This repository publishes plugin binaries as OCI images:

| Plugin | Image | Binary in image |
|--------|-------|-----------------|
| `secretreader` | `registry.k8s.io/cluster-inventory-api/secretreader:<version>` | `/bin/secretreader-plugin` |
| `kubeconfig-secretreader` | `registry.k8s.io/cluster-inventory-api/kubeconfig-secretreader:<version>` | `/bin/kubeconfig-secretreader-plugin` |

## Image Volume

On clusters that support Kubernetes image volumes (`spec.volumes[].image`), mount
the plugin image directly into the controller Pod. If the image is mounted at
`/plugin`, the executable path is `/plugin/bin/<plugin-name>-plugin`.

Example for `secretreader`:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: controller-with-secretreader
spec:
  containers:
    - name: controller
      image: <your-controller-image>
      workingDir: /plugin
      volumeMounts:
        - name: plugin-volume
          mountPath: /plugin
          readOnly: true
  volumes:
    - name: plugin-volume
      image:
        reference: registry.k8s.io/cluster-inventory-api/secretreader:<version>
        pullPolicy: IfNotPresent
```

With that layout, configure the exec provider command as
`./bin/secretreader-plugin` from `workingDir: /plugin`, or as the absolute path
`/plugin/bin/secretreader-plugin`.

## InitContainer Fallback

Older Kubernetes versions, or clusters where the image volume feature gate is
disabled, cannot rely on `spec.volumes[].image`. In those environments, use an
initContainer to export the plugin image filesystem and write the plugin binary
into an `emptyDir` shared with the main container.

Example for `secretreader`:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: controller-with-secretreader
spec:
  initContainers:
    - name: extract-plugin
      image: gcr.io/go-containerregistry/crane/debug:v0.21.6
      command:
        - /busybox/sh
        - -c
      args:
        - |
          set -eu

          case "$(uname -m)" in
            x86_64) arch=amd64 ;;
            aarch64) arch=arm64 ;;
            s390x) arch=s390x ;;
            ppc64le) arch=ppc64le ;;
            *) echo "unsupported arch: $(uname -m)" >&2; exit 1 ;;
          esac

          crane export --platform "linux/${arch}" "$PLUGIN_IMAGE" - \
            | /busybox/tar -xO "bin/${PLUGIN_NAME}-plugin" > "$PLUGIN_DST"
          /busybox/chmod 0555 "$PLUGIN_DST"
      env:
        - name: PLUGIN_NAME
          value: secretreader
        - name: PLUGIN_IMAGE
          value: registry.k8s.io/cluster-inventory-api/secretreader:<version>
        - name: PLUGIN_DST
          value: /plugin/secretreader-plugin
      volumeMounts:
        - name: plugin-bin
          mountPath: /plugin
  containers:
    - name: controller
      image: <your-controller-image>
      volumeMounts:
        - name: plugin-bin
          mountPath: /plugin
          readOnly: true
  volumes:
    - name: plugin-bin
      emptyDir: {}
```

The initContainer writes the plugin binary to `PLUGIN_DST`, so configure the exec
provider command as `/plugin/secretreader-plugin`.

To use the same pattern for `kubeconfig-secretreader`, change only these values:

```yaml
env:
  - name: PLUGIN_NAME
    value: kubeconfig-secretreader
  - name: PLUGIN_IMAGE
    value: registry.k8s.io/cluster-inventory-api/kubeconfig-secretreader:<version>
  - name: PLUGIN_DST
    value: /plugin/kubeconfig-secretreader-plugin
```

Then configure the exec provider command as
`/plugin/kubeconfig-secretreader-plugin`.
