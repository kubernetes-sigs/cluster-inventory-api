# Release Process

## Plugin OCI images

Plugin binaries are released as OCI images built with [ko](https://ko.build). Each plugin is versioned and released independently.

### Versioning

- Each plugin has a **VERSION file** under its directory: `plugins/<name>/VERSION`.
- The file contains a single line with a semantic version (e.g. `0.1.0`).
- Bump the VERSION file when you want to release that plugin.

### How to release

1. **Trigger**: Run the release workflow by either:
   - **Manual**: Actions -> Release -> Run workflow, or
   - **Tag**: Push a repository tag `v*` (e.g. `v1.0.0`) to run the release for all plugins.
2. The workflow scans `plugins/*/` and checks whether the image already exists in the container registry for each plugin's VERSION. Only plugins with unreleased versions are built and pushed.
3. Container images are published to:
   - `ghcr.io/kubernetes-sigs/cluster-inventory-api/<plugin_name>:<VERSION>`
   - Example: `ghcr.io/kubernetes-sigs/cluster-inventory-api/secretreader:0.1.0`

### Local build (no push)

- Run `make snapshot` to build all plugin images locally with ko. Images are loaded into the local Docker daemon as `ko.local/<plugin_name>/...`.

## Project release (optional)

For a high-level project release (e.g. announcing a set of plugin versions):

1. Open an issue proposing a release with a changelog since the last release.
2. All [OWNERS](OWNERS) must LGTM the release.
3. An OWNER creates and pushes a tag (e.g. `git tag -s v1.0.0` and `git push v1.0.0`) to trigger the plugin release workflow, or runs the workflow manually.
4. Close the release issue.
5. Optionally send an announcement (e.g. to the project mailing list or Slack).
