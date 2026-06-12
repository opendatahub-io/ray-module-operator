# ray-module-operator

Module operator for the Ray component in OpenDataHub. Manages the lifecycle of the
KubeRay operator (install, upgrade, status, removal) as part of the ODH modular architecture.

## Overview

This operator reconciles a `Ray` custom resource (`components.platform.opendatahub.io/v1alpha1`)
and is deployed by the ODH platform operator when the Ray module is enabled.

**Epic:** [RHOAIENG-62503](https://redhat.atlassian.net/browse/RHOAIENG-62503)
**Feature:** [RHAISTRAT-1064](https://redhat.atlassian.net/browse/RHAISTRAT-1064)

## Development

### Prerequisites

- Go 1.25+
- Docker or Podman

### Build

```sh
make build
```

### Test

```sh
make test
```

### Lint

```sh
make lint
```

### Docker Build

```sh
make docker-build IMG=quay.io/opendatahub/ray-module-operator:latest
```

## License

Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
