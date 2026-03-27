# hanzodns

## Name

*hanzodns* - REST API for dynamic DNS zone and record management.

## Description

The hanzodns plugin exposes a REST API that allows creating, reading, updating, and deleting DNS zones and records at runtime. Records added through the API are immediately available for DNS resolution by CoreDNS.

The plugin runs a side-car HTTP server (default `:8443`) alongside CoreDNS and stores records in-memory. The store is designed to be swappable for a persistent backend (e.g., PostgreSQL) in the future.

## Syntax

```
hanzodns [ADDR]
```

- **ADDR** - the address to listen on for the HTTP API (default `:8443`).

## Authentication

Set the `HANZO_DNS_API_KEY` environment variable to require Bearer token authentication on all API endpoints (except `/api/v1/health`). When unset, all requests are allowed.

```
Authorization: Bearer <HANZO_DNS_API_KEY>
```

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/health` | Health check (no auth) |
| GET | `/api/v1/zones` | List all zones |
| POST | `/api/v1/zones` | Create a zone |
| GET | `/api/v1/zones/{zone}` | Get zone details |
| DELETE | `/api/v1/zones/{zone}` | Delete a zone |
| GET | `/api/v1/zones/{zone}/records` | List records in a zone |
| POST | `/api/v1/zones/{zone}/records` | Create a record |
| GET | `/api/v1/zones/{zone}/records/{id}` | Get a record |
| PATCH | `/api/v1/zones/{zone}/records/{id}` | Update a record |
| DELETE | `/api/v1/zones/{zone}/records/{id}` | Delete a record |

## Record Types

A, AAAA, CNAME, MX, TXT, SRV, NS, SOA, CAA

## Examples

Enable the API on the default port:

```
. {
    hanzodns
    forward . 8.8.8.8
}
```

Listen on a custom port:

```
. {
    hanzodns :9443
    forward . 8.8.8.8
}
```

Create a zone and record via curl:

```bash
# Create zone
curl -X POST http://localhost:8443/api/v1/zones \
  -H "Authorization: Bearer $HANZO_DNS_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"zone": "example.com"}'

# Add A record
curl -X POST http://localhost:8443/api/v1/zones/example.com/records \
  -H "Authorization: Bearer $HANZO_DNS_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"name": "www", "type": "A", "content": "1.2.3.4", "ttl": 300}'

# Query it
dig @localhost www.example.com A
```
