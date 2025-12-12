# To Do

- [x] Update e2e tests to make tests work with current codebase instead of pushed images
- [x] Update e2e tests to make tests more complete and robust
- [ ] Improve unit test coverage
- [x] Move away from the kube-rbac-proxy to the new scheme (<https://github.com/carlosedp/lbconfig-operator/issues/382>)
- [ ] Check for things to be improved on the tooling side
- [x] Pushing images on e2e tests needs to be logged in and it's not optimal for other users
- [ ] Maybe move the e2e tests to a different scheme instead of Makefile/hack scripts
- [x] After running e2e tests, the bundle manifests have the -dev suffix and use localhost as registry. I have to avoid committing those or re-run `make bundle` before committing. Can this be improved?.
- [ ] Check if docker is being used somewhere instead of podman and make it consistent
- [x] Update all the way to the latest operator-sdk
- [ ] Have dynamic loading for Backend plugins instead of importing on backend_loader
- [x] Add remaining tools to check_versions.sh script
- [x] Adjust readme here new manifest is ./dist instead of ./manifests
