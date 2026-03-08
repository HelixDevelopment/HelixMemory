# HelixMemory - Troubleshooting

**Module:** `digital.vasic.helixmemory`

## Common Issues

### 1. Backend Connection Failures

**Symptom:** Log messages like `failed to connect to <backend>` or
circuit breaker opens immediately on startup.

**Causes and solutions:**

| Cause | Solution |
|-------|----------|
| Service not running | Start infrastructure: `cd docker && docker compose up -d` |
| Wrong endpoint URL | Verify `HELIX_MEMORY_*_ENDPOINT` env vars match actual ports |
| Network isolation | Ensure containers share the same Docker network |
| Firewall blocking | Check that the required ports (8283, 8001, 8000, 8003, 6333, 7687, 6379, 5432) are accessible |

**Diagnostic command:**

```bash
# Check all service ports
for port in 8283 8001 8000 8003 6333 7687 6379 5432; do
    nc -z localhost $port && echo "Port $port: OK" || echo "Port $port: FAIL"
done
```

### 2. Circuit Breaker Keeps Opening

**Symptom:** Operations fail with `circuit breaker open` errors after
initial failures.

**Solutions:**

- Wait for the timeout period (`HELIX_MEMORY_CB_TIMEOUT`, default 30s)
  before retrying
- Increase `HELIX_MEMORY_CB_FAILURE_THRESHOLD` if transient errors are
  common (default: 5)
- Check backend health independently to identify root cause
- HelixMemory degrades gracefully: other backends continue serving while
  one is down

### 3. Search Returns Empty or Incomplete Results

**Symptom:** Searches return fewer results than expected.

**Possible causes:**

| Cause | Solution |
|-------|----------|
| Backends unavailable | Check circuit breaker status via health endpoint |
| Dedup threshold too aggressive | Lower `HELIX_MEMORY_FUSION_DEDUP_THRESHOLD` (default 0.92) |
| Query too narrow | Broaden the search query or remove type filters |
| Memory not stored in queried backend | Verify the memory type routes to the expected backend |
| Concurrency limit hit | Increase `HELIX_MEMORY_MAX_CONCURRENT_SEARCHES` |

### 4. Duplicate Memories Appearing

**Symptom:** The same memory shows up multiple times in search results.

**Solutions:**

- Ensure the consolidation engine is enabled
  (`HELIX_MEMORY_CONSOLIDATION_ENABLED=true`)
- Lower the deduplication threshold:
  `HELIX_MEMORY_FUSION_DEDUP_THRESHOLD=0.88`
- Check that embedding vectors are being generated correctly for
  cosine similarity comparison

### 5. High Latency on Search

**Symptom:** Search operations take several seconds.

**Possible causes and solutions:**

| Cause | Solution |
|-------|----------|
| All backends being queried | The Router fans out reads to all available backends in parallel; slow backends increase p99 |
| Backend under load | Scale the slow backend or increase its resources |
| Large result sets | Reduce the `Limit` in `SearchQuery` |
| Network latency | If backends are remote, check network path |
| Search timeout too high | Lower `HELIX_MEMORY_SEARCH_TIMEOUT` to fail-fast on slow backends |

### 6. Consolidation Not Running

**Symptom:** `ConsolidationStatus.Runs` stays at 0.

**Checklist:**

1. Verify `HELIX_MEMORY_CONSOLIDATION_ENABLED=true`
2. Check the interval: `HELIX_MEMORY_CONSOLIDATION_INTERVAL` (default 30m)
3. Ensure at least one backend is registered and healthy
4. Check logs for consolidation errors

### 7. Letta Core Memory Not Updating

**Symptom:** `UpdateCoreMemory` calls succeed but `GetCoreMemory` returns
stale data.

**Solutions:**

- Verify the Letta server is running on port 8283
- Check that the `agentID` matches an existing Letta agent
- Letta may require a brief propagation delay; retry after 1-2 seconds

### 8. Graphiti Temporal Queries Return Nothing

**Symptom:** `SearchTemporal` or `GetTimeline` returns empty results.

**Checklist:**

1. Verify Graphiti is running on port 8003
2. Ensure Neo4j is accessible (Graphiti depends on Neo4j)
3. Check that the time range (`Start`, `End`) encompasses stored data
4. Verify the `entityID` matches stored temporal memories

### 9. MemoryStore Adapter Type Mismatches

**Symptom:** Errors when using the `MemoryStoreAdapter` with the Memory
module interface.

**Solution:**

The adapter converts between `MemoryEntry` (HelixMemory) and `Memory`
(Memory module). Ensure:

- `Metadata` values are JSON-serializable
- `Confidence` is between 0.0 and 1.0
- `Type` uses valid `MemoryType` constants

## Health Check Endpoint

When integrated with HelixAgent, the health status is available at:

```
GET /v1/monitoring/status
```

Individual backend health is reported in the response under the
`helixmemory` section.

## Logging

HelixMemory uses structured logging via `logrus`. Increase verbosity:

```bash
export LOG_LEVEL=debug
```

Key log fields:

| Field | Description |
|-------|-------------|
| `component` | `helixmemory`, `fusion`, `router`, `consolidation` |
| `provider` | Backend name (mem0, cognee, letta, graphiti) |
| `operation` | `search`, `add`, `get`, `update`, `delete` |
| `duration_ms` | Operation latency in milliseconds |

## Getting Help

1. Check [CONFIGURATION.md](CONFIGURATION.md) for all env variables
2. Review [ARCHITECTURE.md](ARCHITECTURE.md) for data flow understanding
3. Inspect Prometheus metrics under the `helixmemory_` namespace
4. Check Docker container logs: `docker compose logs <service>`
