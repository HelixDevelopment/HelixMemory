// Package infra provides infrastructure bridge for HelixMemory container
// orchestration. It defines interfaces for container management that can
// be implemented by the digital.vasic.containers module or by a local
// docker-compose fallback.
package infra

import (
	"context"
	"fmt"
	"time"
)

// ServiceName identifies a HelixMemory infrastructure service.
type ServiceName string

const (
	ServiceLetta    ServiceName = "letta-server"
	ServiceMem0     ServiceName = "mem0-api"
	ServiceCognee   ServiceName = "cognee-api"
	ServiceQdrant   ServiceName = "qdrant"
	ServiceNeo4j    ServiceName = "neo4j"
	ServiceRedis    ServiceName = "redis"
	ServicePostgres ServiceName = "postgres"
)

// ServiceEndpoint holds connection details for a service.
type ServiceEndpoint struct {
	Name     ServiceName `json:"name"`
	Host     string      `json:"host"`
	Port     int         `json:"port"`
	Protocol string      `json:"protocol"` // "http", "bolt", "tcp"
	Healthy  bool        `json:"healthy"`
}

// URL returns the full service URL.
func (e *ServiceEndpoint) URL() string {
	switch e.Protocol {
	case "bolt":
		return fmt.Sprintf("bolt://%s:%d", e.Host, e.Port)
	case "tcp":
		return fmt.Sprintf("%s:%d", e.Host, e.Port)
	default:
		return fmt.Sprintf("http://%s:%d", e.Host, e.Port)
	}
}

// HealthStatus aggregates health for all services.
type HealthStatus struct {
	Services  map[ServiceName]*ServiceEndpoint `json:"services"`
	Healthy   int                              `json:"healthy"`
	Total     int                              `json:"total"`
	CheckedAt time.Time                        `json:"checked_at"`
}

// AllHealthy returns true if all services are healthy.
func (h *HealthStatus) AllHealthy() bool {
	return h.Healthy == h.Total
}

// InfraProvider abstracts container infrastructure operations.
// Implementations can use digital.vasic.containers or docker-compose
// directly.
type InfraProvider interface {
	// Start brings up all required services.
	Start(ctx context.Context) error

	// Stop shuts down all services.
	Stop(ctx context.Context) error

	// HealthCheck checks all services and returns aggregate status.
	HealthCheck(ctx context.Context) (*HealthStatus, error)

	// GetEndpoint returns connection details for a specific service.
	GetEndpoint(name ServiceName) (*ServiceEndpoint, error)

	// IsRemote returns true if services are running on remote host(s).
	IsRemote() bool
}

// DefaultEndpoints returns the default local endpoints for all services.
func DefaultEndpoints() map[ServiceName]*ServiceEndpoint {
	return map[ServiceName]*ServiceEndpoint{
		ServiceLetta: {
			Name:     ServiceLetta,
			Host:     "localhost",
			Port:     8283,
			Protocol: "http",
		},
		ServiceMem0: {
			Name:     ServiceMem0,
			Host:     "localhost",
			Port:     8001,
			Protocol: "http",
		},
		ServiceCognee: {
			Name:     ServiceCognee,
			Host:     "localhost",
			Port:     8000,
			Protocol: "http",
		},
		ServiceQdrant: {
			Name:     ServiceQdrant,
			Host:     "localhost",
			Port:     6333,
			Protocol: "http",
		},
		ServiceNeo4j: {
			Name:     ServiceNeo4j,
			Host:     "localhost",
			Port:     7687,
			Protocol: "bolt",
		},
		ServiceRedis: {
			Name:     ServiceRedis,
			Host:     "localhost",
			Port:     6379,
			Protocol: "tcp",
		},
		ServicePostgres: {
			Name:     ServicePostgres,
			Host:     "localhost",
			Port:     5432,
			Protocol: "tcp",
		},
	}
}
