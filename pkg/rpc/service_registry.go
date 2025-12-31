package rpc

import "fmt"

// ServiceRegistry maintains mappings from service/method names to IDs for client-side ID lookup
type ServiceRegistry struct {
	serviceNameToID   map[string]uint32
	serviceMethodToID map[string]map[string]uint32 // serviceName -> methodName -> methodID
}

// NewServiceRegistry creates a new ServiceRegistry
func NewServiceRegistry() *ServiceRegistry {
	return &ServiceRegistry{
		serviceNameToID:   make(map[string]uint32),
		serviceMethodToID: make(map[string]map[string]uint32),
	}
}

// RegisterService registers a service and its methods in the registry
func (r *ServiceRegistry) RegisterService(serviceName string, serviceID uint32, methodMap map[string]uint32) {
	r.serviceNameToID[serviceName] = serviceID
	r.serviceMethodToID[serviceName] = methodMap
}

// GetServiceID looks up a service ID by name
func (r *ServiceRegistry) GetServiceID(serviceName string) (uint32, bool) {
	id, ok := r.serviceNameToID[serviceName]
	return id, ok
}

// GetMethodID looks up a method ID by service and method names
func (r *ServiceRegistry) GetMethodID(serviceName, methodName string) (uint32, bool) {
	methods, ok := r.serviceMethodToID[serviceName]
	if !ok {
		return 0, false
	}
	id, ok := methods[methodName]
	return id, ok
}

// GetServiceIDOrPanic looks up a service ID by name and panics if not found
func (r *ServiceRegistry) GetServiceIDOrPanic(serviceName string) uint32 {
	id, ok := r.GetServiceID(serviceName)
	if !ok {
		panic(fmt.Sprintf("service not found in registry: %s", serviceName))
	}
	return id
}

// GetMethodIDOrPanic looks up a method ID by service and method names and panics if not found
func (r *ServiceRegistry) GetMethodIDOrPanic(serviceName, methodName string) uint32 {
	id, ok := r.GetMethodID(serviceName, methodName)
	if !ok {
		panic(fmt.Sprintf("method not found in registry: %s.%s", serviceName, methodName))
	}
	return id
}

