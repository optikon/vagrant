package edge

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
)

// Start listening for table updates on port 8053.
func (e *Edge) startListeningForTableUpdates() {
	e.server = &http.Server{Addr: ":" + pushPort}
	http.HandleFunc("/", e.parseTableUpdate)
	go func() {
		if err := e.server.ListenAndServe(); err != nil {
			log.Errorf("ListenAndServe error: %s", err)
		}
	}()
}

// Parse incoming requests from edge sites.
func (e *Edge) parseTableUpdate(w http.ResponseWriter, r *http.Request) {
	jsn, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Errorln("Error while reading table update:", err)
	}
	update := ServiceTableUpdate{}
	if err = json.Unmarshal(jsn, &update); err != nil {
		log.Errorln("Error while unmarshalling JSON into table update struct:", err)
	}
	e.table.Update(update.Meta.IP, update.Meta.GeoCoords, update.Services)
}

// Stop listening for updates.
func (e *Edge) stopListeningForTableUpdates() {
	e.server.Shutdown(nil)
}
