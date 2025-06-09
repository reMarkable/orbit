# Orbit

A small proxy to turn a GitHub mono-repo into a Terraform module registry. This
proxy expects releases in the same format as a golang monorepo
(`module/vX.Y.Z`), with each module having a directory on the root of the
repository. You might find [release
please](https://github.com/googleapis/release-please) useful for managing the
release process.

## Supported ENV variables

| Environment Variable       | Type     | Default | Required | Description                            |
| -------------------------- | -------- | ------- | -------- | -------------------------------------- |
| MODULES_PROXY_SECRET       | []byte   |         | Yes      | Secret key for proxy token encryption. |
| CACHE_ENABLED              | bool     |         | No       | Enable or disable caching.             |
| CACHE_PATH                 | string   | /tmp    | No       | Path to store cache files.             |
| CACHE_EXPIRATION           | duration | 10s     | No       | Cache expiration duration.             |
| GITHUB_REPOSITORIES        | map      |         | No       | Allowed repositories (per org).        |
| GITHUB_ORG_MAPPINGS        | map      |         | No       | Organization name mappings.            |
| GITHUB_TOKEN               | string   |         | No       | GitHub API token.                      |
| MODULES_TOKEN_EXPIRATION   | duration | 60s     | No       | Expiration time for proxy tokens.      |
| SERVER_HOST                | string   |         | No       | Server listen host.                    |
| SERVER_PORT                | int      | 8080    | No       | Server listen port.                    |
| SERVER_TIMEOUT_HANDLER     | duration | 10s     | No       | HTTP handler timeout.                  |
| SERVER_TIMEOUT_IDLE        | duration |         | No       | HTTP idle timeout.                     |
| SERVER_TIMEOUT_READ        | duration |         | No       | HTTP read timeout.                     |
| SERVER_TIMEOUT_READ_HEADER | duration | 2s      | No       | HTTP read header timeout.              |
| SERVER_TIMEOUT_SHUTDOWN    | duration | 5s      | No       | Graceful shutdown timeout.             |
| SERVER_TIMEOUT_WRITE       | duration |         | No       | HTTP write timeout.                    |
| SERVER_TLS_ENABLED         | bool     |         | No       | Enable TLS for the server.             |
| SERVER_TLS_CERT_FILE       | string   |         | No       | TLS certificate file path.             |
| SERVER_TLS_KEY_FILE        | string   |         | No       | TLS key file path.                     |
| SERVER_METRICS_ENABLED     | bool     | false   | No       | Enable metrics endpoint.               |
| SERVER_METRICS_PORT        | int      | 9090    | No       | Metrics server port.                   |

**Notes:**

- Duration values (e.g., 10s, 60s) are Go duration strings (e.g., 1m, 30s).
- Prefixes like `CACHE_`, `GITHUB_`, `MODULES_`, and `SERVER_` are used for grouping related variables.
- Some variables (like maps) may require specific formatting (e.g., JSON or comma-separated values).
- MODULES_PROXY_SECRET must be a base64-encoded 16, 24 or 32-byte key for AES encryption.

# Deployment

Orbit can easily be deployed using Docker, or by just running the binary
provided in the release. If you want to deploy using Kubernetes, we're providing
a helm chart under `deploy/charts/orbit`.
