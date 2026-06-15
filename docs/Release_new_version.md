# Releasing a new version

Instructions to build and release a new operator version.

## Build container images and manifest bundle

1. Login to Quay.io with `podman login quay.io`.
2. Bump version on Makefile

Run `make dist` which will do automatically the following steps:

1. Build binaries for all supported platforms on `output` directory (amd64, arm64, ...)
2. Cross-build container images for all supported platforms and push to Quay.io
3. Build catalog and bundle images and push them to Quay.io
4. Build manifest bundles on `./bundle`
5. Validate the bundle with operator-sdk
6. Update the image version on Readme

If all is ok, commit the remaining changes to the repository and push to GitHub.

## Release to GitHub

1. Use the `make release` target to create a local tag, push it to GitHub and create a release on GitHub based on this tag. The release description should be updated with the new version changes.
2. After the release, run the target `make bump-version` to update the version on Makefile to the next development version (e.g., 0.6.0 -> 0.7.0-dev) and push the change to GitHub.
3. Re-run the `make manifest` target to update the manifest bundle with the new version and push the change to GitHub.
4. Commit and push all the changes to GitHub.

## Update OperatorHub for both community and OpenShift

For OperatorHub community, checkout <https://github.com/k8s-operatorhub/community-operators>, create the version directory in `community-operators/lbconfig-operator` and copy the whole `bundle` diretory to it.

Fork the repo, create a branch, commit and open a PR upstream.

For OpenShift OperatorHub, checkout <https://github.com/redhat-openshift-ecosystem/community-operators-prod> and do a similar process as above.
