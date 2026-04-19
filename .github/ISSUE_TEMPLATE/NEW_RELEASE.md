---
name: New release
about: Checklist for releasing a new version of cluster-inventory-api
title: "Release vX.Y.Z"
labels: kind/release
---

## Release checklist

### Prepare

- [ ] Changelog added below
- [ ] 2-week lazy consensus period elapsed (no OWNER objections)

### Tag

- [ ] Release tag pushed: `git tag -s v<version> && git push origin v<version>`
- [ ] Draft GitHub release created: `gh release create v<version> --draft --generate-notes --verify-tag`

### Staging

- [ ] `test-infra` postsubmit pushed staging images
- [ ] Staging images verified at `us-central1-docker.pkg.dev/k8s-staging-images/cluster-inventory-api/<plugin>:<tag>`

### Promote

- [ ] Promotion PR created: `kpromo pr --fork <yourname> --project cluster-inventory-api --tag v<version>`
- [ ] Promotion PR reviewed and merged

### Publish

- [ ] Production images verified at `registry.k8s.io/cluster-inventory-api/<plugin>:<tag>`
- [ ] Draft GitHub release published
- [ ] This issue closed
- [ ] Announcement sent

## Changelog

