# Releasing a new version

Instructions to build and release a new operator version.

## Build docker images and manifest bundle

1. If Docker doesn't have a crossbuild instance, create one with `docker buildx create --use`.
2. Login to Quay.io with `docker login quay.io`.
3. Bump version on Makefile

Run `make dist` which will do automatically the following steps:

1. Build binaries for all supported platforms on `output` directory (amd64, arm64, ...)
2. Cross-build docker images for all supported platforms and push to Quay.io
3. Build catalog and bundle images and push them to Quay.io
4. Build manifest bundles on `./bundle`
5. Validate the bundle with operator-sdk
6. Update the image version on Readme

If all is ok, commit the remaining changes to the repository and push to GitHub.

## Release to GitHub

1. Tag the release with `git tag -a $(make print-VERSION) -m "$(make print-VERSION) - Description"`
2. Push the tag to the repo with `git push origin $(make print-VERSION)`
3. On GitHub, create a release based on this tag.

## Update OperatorHub for both community and OpenShift

For OperatorHub community, checkout <https://github.com/k8s-operatorhub/community-operators>, create the version directory in `community-operators/lbconfig-operator` and copy the whole `bundle` diretory to it.

Fork the repo, create a branch, commit and open a PR upstream.

For OpenShift OperatorHub, checkout <https://github.com/redhat-openshift-ecosystem/community-operators-prod> and do a similar process as above.
