// compose.go provides a docker-compose-based InfraProvider fallback.
package infra

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"time"
)

// ComposeProvider implements InfraProvider using docker-compose.
type ComposeProvider struct {
	composeFile string
	endpoints   map[ServiceName]*ServiceEndpoint
}

// NewComposeProvider creates a provider using the specified compose file.
func NewComposeProvider(composeFile string) *ComposeProvider {
	return &ComposeProvider{
		composeFile: composeFile,
		endpoints:   DefaultEndpoints(),
	}
}

// Start brings up services via docker compose.
func (c *ComposeProvider) Start(ctx context.Context) error {
	absPath, err := filepath.Abs(c.composeFile)
	if err != nil {
		return fmt.Errorf("infra: resolve compose path: %w", err)
	}

	cmd := exec.CommandContext(
		ctx, "docker", "compose", "-f", absPath, "up", "-d",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf(
			"infra: docker compose up failed: %s: %w",
			string(output), err,
		)
	}
	return nil
}

// Stop shuts down services via docker compose.
func (c *ComposeProvider) Stop(ctx context.Context) error {
	absPath, err := filepath.Abs(c.composeFile)
	if err != nil {
		return fmt.Errorf("infra: resolve compose path: %w", err)
	}

	cmd := exec.CommandContext(
		ctx, "docker", "compose", "-f", absPath, "down",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf(
			"infra: docker compose down failed: %s: %w",
			string(output), err,
		)
	}
	return nil
}

// HealthCheck checks all service endpoints.
func (c *ComposeProvider) HealthCheck(
	ctx context.Context,
) (*HealthStatus, error) {
	status := &HealthStatus{
		Services:  make(map[ServiceName]*ServiceEndpoint),
		CheckedAt: time.Now(),
	}

	for name, ep := range c.endpoints {
		epCopy := *ep
		epCopy.Healthy = c.checkService(ctx, &epCopy)
		status.Services[name] = &epCopy
		status.Total++
		if epCopy.Healthy {
			status.Healthy++
		}
	}

	return status, nil
}

// GetEndpoint returns connection details for a service.
func (c *ComposeProvider) GetEndpoint(
	name ServiceName,
) (*ServiceEndpoint, error) {
	ep, ok := c.endpoints[name]
	if !ok {
		return nil, fmt.Errorf("infra: unknown service %s", name)
	}
	return ep, nil
}

// IsRemote returns false for local compose.
func (c *ComposeProvider) IsRemote() bool {
	return false
}

// checkService tests connectivity to a service endpoint.
func (c *ComposeProvider) checkService(
	ctx context.Context,
	ep *ServiceEndpoint,
) bool {
	switch ep.Protocol {
	case "http":
		return c.checkHTTP(ctx, ep)
	case "tcp", "bolt":
		return c.checkTCP(ctx, ep)
	default:
		return c.checkTCP(ctx, ep)
	}
}

// checkHTTP performs an HTTP health check.
func (c *ComposeProvider) checkHTTP(
	ctx context.Context,
	ep *ServiceEndpoint,
) bool {
	client := &http.Client{Timeout: 3 * time.Second}
	url := fmt.Sprintf("http://%s:%d", ep.Host, ep.Port)

	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, url, nil,
	)
	if err != nil {
		return false
	}

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 500
}

// checkTCP performs a TCP connectivity check.
func (c *ComposeProvider) checkTCP(
	_ context.Context,
	ep *ServiceEndpoint,
) bool {
	addr := net.JoinHostPort(ep.Host, fmt.Sprintf("%d", ep.Port))
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
