package infra

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// ServiceEndpoint.URL tests
// ---------------------------------------------------------------------------

func TestServiceEndpoint_URL_HTTP(t *testing.T) {
	ep := &ServiceEndpoint{
		Name: ServiceLetta, Host: "localhost", Port: 8283,
		Protocol: "http",
	}
	assert.Equal(t, "http://localhost:8283", ep.URL())
}

func TestServiceEndpoint_URL_Bolt(t *testing.T) {
	ep := &ServiceEndpoint{
		Name: ServiceNeo4j, Host: "10.0.0.5", Port: 7687,
		Protocol: "bolt",
	}
	assert.Equal(t, "bolt://10.0.0.5:7687", ep.URL())
}

func TestServiceEndpoint_URL_TCP(t *testing.T) {
	ep := &ServiceEndpoint{
		Name: ServiceRedis, Host: "redis.local", Port: 6379,
		Protocol: "tcp",
	}
	assert.Equal(t, "redis.local:6379", ep.URL())
}

func TestServiceEndpoint_URL_UnknownProtocol(t *testing.T) {
	ep := &ServiceEndpoint{
		Name: ServicePostgres, Host: "db.local", Port: 5432,
		Protocol: "unknown",
	}
	// Unknown protocols fall through to the default http:// format.
	assert.Equal(t, "http://db.local:5432", ep.URL())
}

func TestServiceEndpoint_URL_TableDriven(t *testing.T) {
	tests := []struct {
		name     string
		ep       ServiceEndpoint
		expected string
	}{
		{
			name: "letta http",
			ep: ServiceEndpoint{
				Name: ServiceLetta, Host: "127.0.0.1",
				Port: 8283, Protocol: "http",
			},
			expected: "http://127.0.0.1:8283",
		},
		{
			name: "mem0 http",
			ep: ServiceEndpoint{
				Name: ServiceMem0, Host: "mem0.svc",
				Port: 8001, Protocol: "http",
			},
			expected: "http://mem0.svc:8001",
		},
		{
			name: "neo4j bolt",
			ep: ServiceEndpoint{
				Name: ServiceNeo4j, Host: "neo4j.svc",
				Port: 7687, Protocol: "bolt",
			},
			expected: "bolt://neo4j.svc:7687",
		},
		{
			name: "postgres tcp",
			ep: ServiceEndpoint{
				Name: ServicePostgres, Host: "pg.svc",
				Port: 5432, Protocol: "tcp",
			},
			expected: "pg.svc:5432",
		},
		{
			name: "empty protocol defaults to http",
			ep: ServiceEndpoint{
				Name: ServiceQdrant, Host: "localhost",
				Port: 6333, Protocol: "",
			},
			expected: "http://localhost:6333",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.ep.URL())
		})
	}
}

// ---------------------------------------------------------------------------
// DefaultEndpoints tests
// ---------------------------------------------------------------------------

func TestDefaultEndpoints_Count(t *testing.T) {
	eps := DefaultEndpoints()
	assert.Len(t, eps, 7, "should have 7 default service endpoints")
}

func TestDefaultEndpoints_AllServicesPresent(t *testing.T) {
	eps := DefaultEndpoints()
	expected := []ServiceName{
		ServiceLetta, ServiceMem0, ServiceCognee,
		ServiceQdrant, ServiceNeo4j, ServiceRedis, ServicePostgres,
	}
	for _, name := range expected {
		ep, ok := eps[name]
		require.True(t, ok, "service %s must be present", name)
		assert.Equal(t, name, ep.Name)
		assert.Equal(t, "localhost", ep.Host)
		assert.Greater(t, ep.Port, 0)
		assert.NotEmpty(t, ep.Protocol)
	}
}

func TestDefaultEndpoints_Ports(t *testing.T) {
	eps := DefaultEndpoints()
	portMap := map[ServiceName]int{
		ServiceLetta:    8283,
		ServiceMem0:     8001,
		ServiceCognee:   8000,
		ServiceQdrant:   6333,
		ServiceNeo4j:    7687,
		ServiceRedis:    6379,
		ServicePostgres: 5432,
	}
	for svc, port := range portMap {
		assert.Equal(t, port, eps[svc].Port,
			"port mismatch for %s", svc)
	}
}

func TestDefaultEndpoints_Protocols(t *testing.T) {
	eps := DefaultEndpoints()
	protoMap := map[ServiceName]string{
		ServiceLetta:    "http",
		ServiceMem0:     "http",
		ServiceCognee:   "http",
		ServiceQdrant:   "http",
		ServiceNeo4j:    "bolt",
		ServiceRedis:    "tcp",
		ServicePostgres: "tcp",
	}
	for svc, proto := range protoMap {
		assert.Equal(t, proto, eps[svc].Protocol,
			"protocol mismatch for %s", svc)
	}
}

// ---------------------------------------------------------------------------
// HealthStatus tests
// ---------------------------------------------------------------------------

func TestHealthStatus_AllHealthy_True(t *testing.T) {
	hs := &HealthStatus{Healthy: 3, Total: 3}
	assert.True(t, hs.AllHealthy())
}

func TestHealthStatus_AllHealthy_False(t *testing.T) {
	hs := &HealthStatus{Healthy: 2, Total: 3}
	assert.False(t, hs.AllHealthy())
}

func TestHealthStatus_AllHealthy_ZeroServices(t *testing.T) {
	hs := &HealthStatus{Healthy: 0, Total: 0}
	assert.True(t, hs.AllHealthy(),
		"zero services should count as all healthy")
}

// ---------------------------------------------------------------------------
// ComposeProvider tests
// ---------------------------------------------------------------------------

func TestNewComposeProvider(t *testing.T) {
	cp := NewComposeProvider("/path/to/docker-compose.yml")
	require.NotNil(t, cp)
	assert.Equal(t, "/path/to/docker-compose.yml", cp.composeFile)
	assert.Len(t, cp.endpoints, 7)
}

func TestComposeProvider_GetEndpoint_AllServices(t *testing.T) {
	cp := NewComposeProvider("docker-compose.yml")
	services := []ServiceName{
		ServiceLetta, ServiceMem0, ServiceCognee,
		ServiceQdrant, ServiceNeo4j, ServiceRedis, ServicePostgres,
	}
	for _, svc := range services {
		ep, err := cp.GetEndpoint(svc)
		require.NoError(t, err, "GetEndpoint(%s) should not error", svc)
		assert.Equal(t, svc, ep.Name)
	}
}

func TestComposeProvider_GetEndpoint_Unknown(t *testing.T) {
	cp := NewComposeProvider("docker-compose.yml")
	ep, err := cp.GetEndpoint(ServiceName("nonexistent"))
	assert.Nil(t, ep)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown service")
}

func TestComposeProvider_IsRemote(t *testing.T) {
	cp := NewComposeProvider("docker-compose.yml")
	assert.False(t, cp.IsRemote())
}

func TestComposeProvider_ImplementsInfraProvider(t *testing.T) {
	var _ InfraProvider = (*ComposeProvider)(nil)
}

// ---------------------------------------------------------------------------
// ComposeProvider.HealthCheck with real test servers
// ---------------------------------------------------------------------------

func TestComposeProvider_HealthCheck_HTTPHealthy(t *testing.T) {
	// Start a test HTTP server that returns 200 OK.
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, `{"status":"ok"}`)
		},
	))
	defer srv.Close()

	// Parse host/port from test server.
	host, port := parseHostPort(t, srv.Listener.Addr().String())

	cp := NewComposeProvider("docker-compose.yml")
	// Override a single endpoint to point at our test server.
	cp.endpoints = map[ServiceName]*ServiceEndpoint{
		ServiceLetta: {
			Name:     ServiceLetta,
			Host:     host,
			Port:     port,
			Protocol: "http",
		},
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 5*time.Second,
	)
	defer cancel()

	status, err := cp.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, status.Total)
	assert.Equal(t, 1, status.Healthy)
	assert.True(t, status.AllHealthy())
	assert.True(t, status.Services[ServiceLetta].Healthy)
	assert.False(t, status.CheckedAt.IsZero())
}

func TestComposeProvider_HealthCheck_HTTPUnhealthy(t *testing.T) {
	// Server that returns 500 — should be marked unhealthy.
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		},
	))
	defer srv.Close()

	host, port := parseHostPort(t, srv.Listener.Addr().String())

	cp := NewComposeProvider("docker-compose.yml")
	cp.endpoints = map[ServiceName]*ServiceEndpoint{
		ServiceCognee: {
			Name:     ServiceCognee,
			Host:     host,
			Port:     port,
			Protocol: "http",
		},
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 5*time.Second,
	)
	defer cancel()

	status, err := cp.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, status.Total)
	assert.Equal(t, 0, status.Healthy)
	assert.False(t, status.AllHealthy())
	assert.False(t, status.Services[ServiceCognee].Healthy)
}

func TestComposeProvider_HealthCheck_TCPHealthy(t *testing.T) {
	// Start a TCP listener.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	host, port := parseHostPort(t, ln.Addr().String())

	cp := NewComposeProvider("docker-compose.yml")
	cp.endpoints = map[ServiceName]*ServiceEndpoint{
		ServiceRedis: {
			Name:     ServiceRedis,
			Host:     host,
			Port:     port,
			Protocol: "tcp",
		},
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 5*time.Second,
	)
	defer cancel()

	status, err := cp.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, status.Healthy)
	assert.True(t, status.Services[ServiceRedis].Healthy)
}

func TestComposeProvider_HealthCheck_TCPUnhealthy(t *testing.T) {
	cp := NewComposeProvider("docker-compose.yml")
	// Point to a port that is almost certainly not listening.
	cp.endpoints = map[ServiceName]*ServiceEndpoint{
		ServicePostgres: {
			Name:     ServicePostgres,
			Host:     "127.0.0.1",
			Port:     19999,
			Protocol: "tcp",
		},
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 5*time.Second,
	)
	defer cancel()

	status, err := cp.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, status.Healthy)
	assert.False(t, status.Services[ServicePostgres].Healthy)
}

func TestComposeProvider_HealthCheck_BoltProtocol(t *testing.T) {
	// Bolt protocol uses TCP check internally.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	host, port := parseHostPort(t, ln.Addr().String())

	cp := NewComposeProvider("docker-compose.yml")
	cp.endpoints = map[ServiceName]*ServiceEndpoint{
		ServiceNeo4j: {
			Name:     ServiceNeo4j,
			Host:     host,
			Port:     port,
			Protocol: "bolt",
		},
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 5*time.Second,
	)
	defer cancel()

	status, err := cp.HealthCheck(ctx)
	require.NoError(t, err)
	assert.True(t, status.Services[ServiceNeo4j].Healthy)
}

func TestComposeProvider_HealthCheck_MixedServices(t *testing.T) {
	// HTTP server (healthy).
	httpSrv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	))
	defer httpSrv.Close()
	httpHost, httpPort := parseHostPort(
		t, httpSrv.Listener.Addr().String(),
	)

	// TCP listener (healthy).
	tcpLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer tcpLn.Close()
	tcpHost, tcpPort := parseHostPort(t, tcpLn.Addr().String())

	cp := NewComposeProvider("docker-compose.yml")
	cp.endpoints = map[ServiceName]*ServiceEndpoint{
		ServiceLetta: {
			Name: ServiceLetta, Host: httpHost,
			Port: httpPort, Protocol: "http",
		},
		ServiceRedis: {
			Name: ServiceRedis, Host: tcpHost,
			Port: tcpPort, Protocol: "tcp",
		},
		ServicePostgres: {
			Name: ServicePostgres, Host: "127.0.0.1",
			Port: 19998, Protocol: "tcp",
		},
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 5*time.Second,
	)
	defer cancel()

	status, err := cp.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, status.Total)
	assert.Equal(t, 2, status.Healthy)
	assert.False(t, status.AllHealthy())
	assert.True(t, status.Services[ServiceLetta].Healthy)
	assert.True(t, status.Services[ServiceRedis].Healthy)
	assert.False(t, status.Services[ServicePostgres].Healthy)
}

// ---------------------------------------------------------------------------
// ServiceName constants
// ---------------------------------------------------------------------------

func TestServiceName_Values(t *testing.T) {
	assert.Equal(t, ServiceName("letta-server"), ServiceLetta)
	assert.Equal(t, ServiceName("mem0-api"), ServiceMem0)
	assert.Equal(t, ServiceName("cognee-api"), ServiceCognee)
	assert.Equal(t, ServiceName("qdrant"), ServiceQdrant)
	assert.Equal(t, ServiceName("neo4j"), ServiceNeo4j)
	assert.Equal(t, ServiceName("redis"), ServiceRedis)
	assert.Equal(t, ServiceName("postgres"), ServicePostgres)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// parseHostPort extracts host and port from an address string.
func parseHostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	var port int
	_, err = fmt.Sscanf(portStr, "%d", &port)
	require.NoError(t, err)
	return host, port
}
