package edge

import (
	"sync"
)

// ServiceTable specifies the mapping from service DNS names to edge sites.
type ServiceTable map[string]Set

// ServiceTableUpdate encapsulates all the information sent in a table update
// from an edge site.
type ServiceTableUpdate struct {
	Meta  Site         `json:"meta"`
	Event ServiceEvent `json:"event"`
}

// ConcurrentServiceTable is a table that can be safely shared between goroutines.
type ConcurrentServiceTable struct {
	sync.RWMutex
	table ServiceTable
}

// NewConcurrentServiceTable creates a new concurrent table.
func NewConcurrentServiceTable() *ConcurrentServiceTable {
	return &ConcurrentServiceTable{
		table: make(ServiceTable),
	}
}

// Lookup performs a locked lookup for edge sites running a particular service.
func (cst *ConcurrentServiceTable) Lookup(svc string) (Set, bool) {
	cst.Lock()
	defer cst.Unlock()
	set, found := cst.table[svc]
	return set, found
}

// Add adds a new entry to the table.
func (cst *ConcurrentServiceTable) Add(meta Site, serviceName string) {

	// Lock down the table.
	cst.Lock()
	defer cst.Unlock()

	// Add the new site.
	if edgeSites, found := cst.table[serviceName]; found {
		edgeSites.Add(meta)
	} else {
		newSet := NewSet()
		newSet.Add(meta)
		cst.table[serviceName] = newSet
	}

	// Log the new table.
	if svcDebugMode {
		log.Infof("Updated table: %+v", cst.table)
	}
}

// Remove deletes an entry from the table.
func (cst *ConcurrentServiceTable) Remove(meta Site, serviceName string) {

	// Lock down the table.
	cst.Lock()
	defer cst.Unlock()

	// Add the new site.
	if edgeSites, found := cst.table[serviceName]; found {
		edgeSites.Remove(meta)
		if edgeSites.Len() == 0 {
			delete(cst.table, serviceName)
		}
	}

	// Log the new table.
	if svcDebugMode {
		log.Infof("Updated table: %+v", cst.table)
	}
}
