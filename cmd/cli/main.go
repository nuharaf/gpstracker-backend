package main

import (
	"context"
	"flag"
	"sync"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/phuslu/log"
	gpsv2 "nuha.dev/gpstracker/internal/gpsv2/server"
	"nuha.dev/gpstracker/internal/gpsv2/sublist"
	"nuha.dev/gpstracker/internal/store/impl/pgstore"
	"nuha.dev/gpstracker/internal/web"
	"nuha.dev/gpstracker/internal/web/monitoring"
	ws "nuha.dev/gpstracker/internal/web/webstream"
)

func main() {

	db_url := flag.String("db_url", "postgresql://postgres:postgres@localhost/gpsv2", "postgres database url")
	gps_server := flag.Bool("gps_server", true, "run gps server")
	// gps_server_mock_login := flag.Bool("gps_mock_login", true, "mock gps login")
	// gps_server_mock_store := flag.Bool("gps_mock_store", true, "mock gps store")
	gps_server_listen_addr := flag.String("gps_address", ":6000", "gps server address to listen to")
	ws_server := flag.Bool("ws_server", true, "run ws server")
	ws_server_mock_login := flag.Bool("ws_mock_login", true, "mock ws login")
	ws_server_listen_addr := flag.String("ws_address", ":7000", "ws server address to listen to")
	api_server := flag.Bool("api_server", true, "run api server")
	api_server_listen_addr := flag.String("api_address", ":3333", "api server address to listen to")
	api_server_cookie_domain := flag.String("cookie_domain", "localhost", "domain to set the cookie")
	mon_server := flag.Bool("mon_server", true, "run monitoring server")
	mon_server_listen_addr := flag.String("mon_address", "localhost:3334", "monitoring server address to listen to")
	flag.Parse()
	log.DefaultLogger.Level = log.TraceLevel

	pool, err := pgxpool.Connect(context.Background(), *db_url)
	if err != nil {
		panic(err.Error())
	}

	store := pgstore.NewStore(pool, "locations", &pgstore.StoreConfig{BufSize: 10, TickerDur: 50 * time.Second, MaxAgeFlush: 50 * time.Second})
	store.Run()
	wg := sync.WaitGroup{}
	var srv *gpsv2.Server
	// if *gps_server {
	// 	srv = gps.NewServer(pool, store, &gps.ServerConfig{DirectListenerAddr: *gps_server_listen_addr, MockStore: *gps_server_mock_store})
	// 	go srv.Run()
	// 	wg.Add(1)
	// }
	sublistmap := sublist.NewSublistMap()
	if *gps_server {
		srv = gpsv2.NewServer(pool, store, sublistmap, &gpsv2.ServerConfig{ListenerAddr: *gps_server_listen_addr})
		go srv.Run()
		wg.Add(1)
	}

	if *ws_server {
		ws := ws.NewWebstream(pool, srv, sublistmap, ws.WebStreamConfig{MockToken: *ws_server_mock_login, ListenAddr: *ws_server_listen_addr})
		go ws.Run()
		wg.Add(1)
	}
	if *api_server {
		api := web.NewApi(pool, srv, &web.ApiConfig{ListenAddr: *api_server_listen_addr, CookieDomain: *api_server_cookie_domain, VerifyCSRF: true})
		go api.Run()
		wg.Add(1)
	}

	if *mon_server {
		mon := monitoring.NewMonApi(srv, &monitoring.MonitoringConfig{ListenAddr: *mon_server_listen_addr})
		go mon.Run()
		wg.Add(1)
	}
	wg.Wait()

}
