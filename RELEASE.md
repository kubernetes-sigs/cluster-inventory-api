# Release Process

## Plugin OCI images

Plugin binaries are released as OCI images built with [Docker Buildx](https://docs.docker.com/build/buildx/). The plugin binary is placed under `/bin/<plugin_name>-plugin` (e.g. `/bin/secretreader-plugin`); when the image is mounted at `/plugin`, the executable path is `/plugin/bin/<plugin_name>-plugin`.

### Versioning

Plugin OCI images are tagged with the same version as the repository release tag. For example, pushing `v0.2.0` publishes all plugin images with tag `v0.2.0`.

### Prerequisites

Promoting images requires [`kpromo`](https://github.com/kubernetes-sigs/promo-tools) and a fork of [kubernetes/k8s.io](https://github.com/kubernetes/k8s.io).

Before the first release, the following must exist in `kubernetes/k8s.io`:

- `registry.k8s.io/images/k8s-staging-cluster-inventory-api/OWNERS`
- `registry.k8s.io/images/k8s-staging-cluster-inventory-api/images.yaml`
- `registry.k8s.io/manifests/k8s-staging-cluster-inventory-api/promoter-manifest.yaml` (maps `k8s-staging-cluster-inventory-api` to `cluster-inventory-api` under `registry.k8s.io`)

See the [registry.k8s.io README](https://github.com/kubernetes/k8s.io/tree/main/registry.k8s.io) for setup details.

### How to release

1. Push a signed release tag (e.g. `git tag -s v1.0.0 && git push origin v1.0.0`).
2. Create a draft GitHub release for the tag (e.g. `gh release create v1.0.0 --draft --generate-notes --verify-tag`).
3. The test-infra postsubmit job builds all plugin images and pushes them to the staging registry (`us-central1-docker.pkg.dev/k8s-staging-images/cluster-inventory-api`).
4. Create a promotion PR in `kubernetes/k8s.io` using [`kpromo`](https://github.com/kubernetes-sigs/promo-tools):
   ```bash
   kpromo pr --fork <yourname> --project cluster-inventory-api --tag v1.0.0
   ```
5. After the promotion PR is reviewed and merged, the images become available at `registry.k8s.io/cluster-inventory-api/<plugin>:<tag>`.
6. Publish the draft GitHub release and close the release issue.

A release checklist is available as an [issue template](.github/ISSUE_TEMPLATE/NEW_RELEASE.md).

### Local build

- Run `make docker-build PLUGIN_NAME=<name>` to build a single plugin image locally (e.g. `make docker-build PLUGIN_NAME=secretreader`). The image is tagged as `<plugin_name>:latest` by default.
- Run `make release-staging VERSION=<tag> REGISTRY=<registry>` to build and push all plugin images to a staging registry.

## Project release (optional)

For a high-level project release (e.g. announcing a set of plugin versions):

1. Open an issue proposing a release with a changelog since the last release.
2. The release proposal follows a **lazy consensus** model: the proposal is approved unless an [OWNER](OWNERS) objects within **two weeks** of the issue being opened. Silence is treated as approval.
3. An OWNER pushes a release tag (e.g. `v1.0.0`) to trigger the release process described above.
4. Close the release issue.
5. Optionally send an announcement (e.g. to the project mailing list or Slack).
