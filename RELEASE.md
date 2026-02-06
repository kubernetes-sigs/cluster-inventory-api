# Release Process

## Plugin OCI images

Plugin binaries are released as OCI images built with [Docker Buildx](https://docs.docker.com/build/buildx/). The plugin binary is placed under `/bin/<plugin_name>-plugin` (e.g. `/bin/secretreader-plugin`); when the image is mounted at `/plugin`, the executable path is `/plugin/bin/<plugin_name>-plugin`.

### Versioning

Plugin OCI images are tagged with the same version as the repository release tag. For example, pushing `v0.2.0` publishes all plugin images with tag `0.2.0`.

### How to release

1. Push a repository tag `v*` (e.g. `v1.0.0`). This triggers the `Release` workflow.
2. The workflow scans `plugins/*/` and checks whether the image already exists in the container registry for that version. Only plugins that haven't been published yet are built and pushed.
3. Container images are published to:
   - `ghcr.io/kubernetes-sigs/cluster-inventory-api/<plugin_name>:<version>`
   - Example: `ghcr.io/kubernetes-sigs/cluster-inventory-api/secretreader:1.0.0`

### SBOM and provenance

Each released image includes attestations as OCI referrers:

- **SBOM** (SPDX): `cosign download sbom ghcr.io/kubernetes-sigs/cluster-inventory-api/<plugin>:<version>`
- **Provenance** (SLSA-style): attached by Buildx; verify with [cosign verify attestation](https://docs.sigstore.dev/cosign/verify-attestation/) or your preferred policy engine.

### Local build (no push)

- Run `make snapshot` to build all plugin images locally with Buildx. Images are loaded into the local Docker daemon as `<plugin_name>:latest`.

## Project release (optional)

For a high-level project release (e.g. announcing a set of plugin versions):

1. Open an issue proposing a release with a changelog since the last release.
2. All [OWNERS](OWNERS) must LGTM the release.
3. An OWNER pushes a release tag (e.g. `v1.0.0`) to trigger the Release workflow.
4. Close the release issue.
5. Optionally send an announcement (e.g. to the project mailing list or Slack).
