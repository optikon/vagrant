package edge

import (
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Starts the process of reading Kubernetes services using a list watcher.
func (e *Edge) startReadingServices() {
	go func() {
		w, err := e.clientset.CoreV1().Services("").Watch(metaV1.ListOptions{Watch: true})
		if err != nil {
			log.Fatalf("couldn't read locally running Kubernetes services: %v", err)
		}
		e.watcher = w
		for {
			eventChan := e.watcher.ResultChan()
			for rawEvent := range eventChan {

				// Convert the watch event to our own service event type.
				event, err := parseEvent(rawEvent)
				if err != nil {
					log.Errorf("unable to parse raw Kubernetes watch event while reading local services: %v", err)
					continue
				}

				// Update our local service set accordingly.
				switch event.Type {
				case Add:
					e.services.Add(event.Service)
					e.table.Add(e.site, event.Service)
				case Delete:
					e.services.Remove(event.Service)
					e.table.Remove(e.site, event.Service)
				}

				// Log the updated services.
				if svcDebugMode {
					log.Infof("updated services: %+v", e.services)
				}

				// Push the update upstream.
				for _, p := range e.proxies {
					p.pushServiceEvent(e.site, event)
				}
			}
		}
	}()
}

// Stops reading local Kubernetes services.
func (e *Edge) stopReadingServices() {
	e.watcher.Stop()
}
