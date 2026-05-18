# core-system-backend

## Requirements

- **Go**: 1.24.2 or higher
- **PostgreSQL**: 18
- **Docker** (optional): for running PostgreSQL container
- **SQLC**: for code generation
- **Mockery**: for generating test mocks

## Installation

1. **Clone the project**

```
git clone https://github.com/NYCU-SDC/core-system-backend.git
cd core-system-backend
```

2. **Install Go dependencies**

```
make prepare
```

3. **Start PostgreSQL container**

```
docker run --name db -e POSTGRES_PASSWORD=password -p 5432:5432 -d postgres:18
```

4. **Create database**

Connect to PostgreSQL and create a database named `core-system`:

```
createdb core-system
```

5. **Configure environment**

```
cp config.example.yaml config.yaml
```

Edit `config.yaml` according to your environment (see Configuration section below).

6. **Start the service**

```
make run
```

Verify the service is running:

```
http://localhost:8080/api/healthz
```

You should see: `OK`

## Configuration

Copy `config.example.yaml` to `config.yaml`. Here is the configuration reference:

| Parameter                  | Description                             | Default                               |
| -------------------------- | --------------------------------------- | ------------------------------------- |
| `debug`                    | Enable debug mode                       | `false`                               |
| `host`                     | Server bind address                     | `localhost`                           |
| `port`                     | Server listening port                   | `8080`                                |
| `base_url`                 | Public base URL for OAuth redirect URIs | `http://localhost:8080`               |
| `secret`                   | Secret key for signing JWT tokens       | -                                     |
| `database_url`             | PostgreSQL connection string            | -                                     |
| `migration_source`         | Database migration source path          | `file://internal/database/migrations` |
| `access_token_expiration`  | Access token expiration                 | `15m`                                 |
| `refresh_token_expiration` | Refresh token expiration                | `720h` (30 days)                      |
| `otel_collector_url`       | OpenTelemetry Collector URL (optional)  | -                                     |

### OAuth Settings

```yaml
google_oauth:
  client_id: "your-google-oauth-client-id"
  client_secret: "your-google-oauth-client-secret"

nycu_oauth:
  client_id: "your-nycu-oauth-client-id"
  client_secret: "your-nycu-oauth-client-secret"

github_oauth:
  client_id: "your-github-oauth-client-id"
  client_secret: "your-github-oauth-client-secret"
```

### Other Options

```yaml
# Allowed emails for onboarding (comma-separated)
allow_onboarding_list:

# Default global user roles
# Format: <email>:<role>,
default_global_roles: |

# Default org roles
# Format: <email>:<role>,
default_org_roles: |
```
