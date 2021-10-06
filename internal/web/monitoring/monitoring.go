package monitoring

import (
	"net/http"
	"time"

	gps "nuha.dev/gpstracker/internal/gps/serverimpl"
	"nuha.dev/gpstracker/internal/util"
)

type MonitoringServer struct {
	gsrv   *gps.Server
	server *http.Server
}

type MonitoringConfig struct {
	ListenAddr string
}

func NewMonApi(server *gps.Server, config *MonitoringConfig) *MonitoringServer {
	m := &MonitoringServer{}
	m.gsrv = server
	m.server = &http.Server{
		Addr:           config.ListenAddr,
		Handler:        http.HandlerFunc(m.serve_http),
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	return m
}

func (m *MonitoringServer) Run() {
	err := m.server.ListenAndServe()
	if err != nil {
		panic(err)
	}
}

func (m *MonitoringServer) serve_http(w http.ResponseWriter, r *http.Request) {
	res := m.gsrv.GetClientsStatus()
	util.JsonWrite(w, res)

}

func (m *MonitoringServer) GetHandler() http.Handler {
	return http.HandlerFunc(m.serve_http)
}
