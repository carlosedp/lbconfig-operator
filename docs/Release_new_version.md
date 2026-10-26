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

1. Use the `make release` target (it refuses to run on a `-dev` version, a dirty tree or a bundle that was not generated for the release version) to create a local tag, push it to GitHub and create a release on GitHub based on this tag. The release description should be updated with the new version changes.
2. After the release, run the target `make bump-version` to update the version on Makefile to the next development version (it bumps the patch number, e.g., 0.7.0 -> 0.7.1-dev).
3. Run `make bundle` to regenerate the bundle, `config/manager/kustomization.yaml` and `dist/install.yaml` with the new development version.
4. Commit and push all the changes to GitHub.

## Update OperatorHub for both community and OpenShift

The same bundle is submitted to two repositories:

- OperatorHub community: <https://github.com/k8s-operatorhub/community-operators>
- OpenShift OperatorHub: <https://github.com/redhat-openshift-ecosystem/community-operators-prod>

> [!IMPORTANT]
> Always take the bundle from the **release tag**, never from the `bundle/` directory on `main`. After `make bump-version` and `make bundle`, the working copy holds the next `-dev` version, and the CI fails with `check_operator_version_directory_name` because the CSV version doesn't match the directory name.

In each repository, with an up-to-date `main` on your fork, create a branch:

```sh
VERSION=0.7.0
cd <operatorhub-repo>
git switch main && git pull upstream main
git switch -c lbconfig-operator-$VERSION
```

Then, from the lbconfig-operator repository, copy the released bundle with:

```sh
make operatorhub OPERATORHUB_DIR=<operatorhub-repo> RELEASE=$VERSION
```

The target extracts the *contents* of `bundle/` from the `v$VERSION` tag into `operators/lbconfig-operator/$VERSION` and fails if the tag doesn't exist, the directory already exists or the CSV version doesn't match. To do it by hand instead:

```sh
mkdir -p operators/lbconfig-operator/$VERSION
git -C <path-to>/lbconfig-operator archive v$VERSION bundle | tar -x --strip-components=1 -C operators/lbconfig-operator/$VERSION
```

Check the result before committing:

```sh
ls operators/lbconfig-operator/$VERSION
# manifests  metadata  tests   <- same layout as the previous versions, no "bundle" subdirectory

grep -E "^  name:|^  version:|containerImage" operators/lbconfig-operator/$VERSION/manifests/lbconfig-operator.clusterserviceversion.yaml
# all must show $VERSION, without a -dev suffix
```

Commit with a sign-off (DCO is required) using the title format the repositories expect, push to your fork and open a PR upstream:

```sh
git add operators/lbconfig-operator/$VERSION
git commit -s -m "operator lbconfig-operator ($VERSION)"
git push -u origin lbconfig-operator-$VERSION
```

If the CI fails and you need to fix the files, amend the commit (`git commit --amend -s --no-edit`) and force-push the branch (`git push --force-with-lease`) so the PR stays a single signed-off commit.
