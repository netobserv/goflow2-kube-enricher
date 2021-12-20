## Automated release

Automated release should be performed using prow, along with the other `netobserv` components.

## Manual release

The following steps are provided for information purposes and should only be performed as an exception (e.g. Prow CI being down / unavailable). Write rights on github & quay are necessary.

```bash
# Adapt VERSION below
VERSION=v0.1.5
git tag -a "$VERSION" -m "$VERSION"
TAG=`git describe --long HEAD`
VERSION="$TAG" USER=netobserv make image push
# Assuming "upstream" is https://github.com/netobserv/goflow2-kube-enricher
git push upstream --tags
```
