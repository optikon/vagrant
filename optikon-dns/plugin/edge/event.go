package edge

import (
	"fmt"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"
)

// ServiceEventType specifies whether a service was added or deleted.
type ServiceEventType uint8

const (
	// Add is an event type for services to be added.
	Add ServiceEventType = iota
	// Delete is an event type for services to be deleted.
	Delete
)

// ServiceEvent is a wrapper for service events, to be packaged and sent upstream.
type ServiceEvent struct {
	Type    ServiceEventType `json:"type"`
	Service string           `json:"service"`
}

// Parses a client-go event and converts it to our ServiceEvent type.
func parseEvent(e watch.Event) (ServiceEvent, error) {

	// Start with a base event.
	evt := ServiceEvent{}

	// Convert the event type.
	switch e.Type {
	case watch.Modified:
		fallthrough
	case watch.Added:
		evt.Type = Add
	case watch.Deleted:
		evt.Type = Delete
	default:
		return ServiceEvent{}, errEventParseFailure
	}

	// Convert the event contents to a service string.
	evt.Service = generateServiceDNS(e.Object.(*v1.Service))

	return evt, nil
}

// Generates a services DNS that looks like my-svc.my-namespace.svc.cluster.external
func generateServiceDNS(svc *v1.Service) string {
	return fmt.Sprintf("%s.%s.svc.cluster.external", svc.GetName(), svc.GetNamespace())
}
