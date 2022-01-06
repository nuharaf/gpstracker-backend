package webstream

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/phuslu/log"

	"nhooyr.io/websocket"
	gpsv2 "nuha.dev/gpstracker/internal/gpsv2/server"
	"nuha.dev/gpstracker/internal/gpsv2/sublist"
)

type WebstreamServer struct {
	server     *http.Server
	log        log.Logger
	gsrv       *gpsv2.Server
	config     WebStreamConfig
	db         *pgxpool.Pool
	sublistmap *sublist.SublistMap
}

type WebStreamConfig struct {
	MockToken  bool
	ListenAddr string
}

type WsSubscriber struct {
	loc     chan []byte
	skipped uint64
	pushed  uint64
}

const (
	CLogin string = "login"
	CSub   string = "subscribe"
	CUnsub string = "unsubscribe"
	CLimit string = "limit"
)

func (wsub *WsSubscriber) Push(d []byte) {
	select {
	case wsub.loc <- d:
		atomic.AddUint64(&wsub.pushed, 1)
	default:
		atomic.AddUint64(&wsub.skipped, 1)
	}
}

func NewWebstream(db *pgxpool.Pool, gps_server *gpsv2.Server, sublistmap *sublist.SublistMap, config WebStreamConfig) *WebstreamServer {
	o := &WebstreamServer{config: config}
	o.server = &http.Server{
		Addr:           config.ListenAddr,
		Handler:        http.HandlerFunc(o.serve_http),
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	o.log = log.DefaultLogger
	o.log.Context = log.NewContext(nil).Str("module", "websocket").Value()
	o.gsrv = gps_server
	o.db = db
	o.sublistmap = sublistmap
	return o
}

func (ws *WebstreamServer) Run() {
	ws.log.Info().Msgf("starting ws-server on : %s", ws.server.Addr)
	err := ws.server.ListenAndServe()
	if err != nil {
		ws.log.Error().Err(err).Msg("")
		panic(err)
	}
}

func (ws *WebstreamServer) serve_http(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, CompressionMode: websocket.CompressionDisabled,
	})

	if err != nil {
		ws.log.Error().Err(err).Msg("Error while upgrading websocket")
		return
	}
	//read login info
	readCtx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
	defer cancel()
	_, msg, err := c.Read(readCtx)
	if err != nil {
		ws.log.Error().Err(err).Msg("Error while reading auth token")
		return
	}
	ws.log.Info().Msg("websocket token received")

	valid, session_id := ws.validate_token(r.Context(), string(msg))
	if !valid {
		c.Close(websocket.StatusPolicyViolation, "invalid token")
		ws.log.Info().Msg("invalid websocket token")
		return
	} else {
		wc := &WebstreamClient{sid: session_id, srv: ws, c: c, tok: msg, log: ws.log}
		wc.buf = make([][]byte, 0, 10)
		wc.wg = sync.WaitGroup{}
		wc.lock = sync.Mutex{}
		wc.sublist = make(map[uint64]*sublist.Sublist)
		wc.wg.Add(1)
		go wc.writeLoop()
		wc.wg.Add(1)
		go wc.readloop()
		wc.wg.Wait()
	}
}

func (ws *WebstreamServer) validate_token(ctx context.Context, token string) (bool, string) {
	var session_id string
	row := ws.db.QueryRow(ctx, `SELECT session.session_id
	FROM websocket_session INNER JOIN session ON websocket_session.session_id = session.session_id
	WHERE websocket_session.ws_token = $1 
	AND session.valid_until > NOW()`, token)
	err := row.Scan(&session_id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, ""
		} else {
			ws.log.Error().Err(err).Msg("")
			panic(err)
		}
	} else {
		return true, session_id
	}
}

type WebstreamClient struct {
	lock    sync.Mutex
	wg      sync.WaitGroup
	srv     *WebstreamServer
	c       *websocket.Conn
	sid     string
	tok     []byte
	log     log.Logger
	closed  bool
	err     error
	buf     [][]byte
	sublist map[uint64]*sublist.Sublist
}

func (wc *WebstreamClient) closeErr(err error) {
	wc.closed = true
	wc.err = err
}

func (wc *WebstreamClient) readloop() {

	defer wc.wg.Done()
	for {
		_, msg, err := wc.c.Read(context.Background())
		if err != nil {
			wc.log.Error().Err(err)
			wc.lock.Lock()
			wc.closeErr(err)
			wc.lock.Unlock()
			return
		} else {
			if string(msg[:6]) == "ADDSUB" {
				subname := strings.Split(string(msg[7:]), ",")
				wc.log.Debug().Strs("addsub", subname).Msg("receive add subscription message")
				for _, v := range subname {
					id, err := strconv.ParseUint(v, 10, 64)
					if err == nil {
						_, ok := wc.sublist[id]
						if ok {
							wc.log.Warn().Msgf("already susbcribed tracker_id : %d", id)
						} else {
							slist, _ := wc.srv.sublistmap.GetSublist(id, true)
							slist.Subscribe(wc)
							wc.sublist[id] = slist
							wc.log.Trace().Msgf("subscribing to %d", id)
						}
						if len(wc.sublist) > 5 {
							wc.closeErr(fmt.Errorf("too many subscription"))
							wc.log.Warn().Msg("debug message limit total subscription")
						}
					}

				}
			} else if string(msg[:6]) == "DELSUB" {
				subname := strings.Split(string(msg[7:]), ",")
				wc.log.Debug().Strs("delsub", subname).Msg("receive delete subscription message")
				for _, v := range subname {
					id, err := strconv.ParseUint(v, 10, 64)
					if err == nil {
						slist, ok := wc.sublist[id]
						if ok {
							slist.Unsubscribe(wc)
							delete(wc.sublist, id)
							wc.log.Trace().Msgf("unsubscribing to %d", id)
						} else {
							wc.log.Warn().Uint64("tracker_id", id).Msg("invalid unsub id")
						}
					}
				}
			}
		}
	}
}

func (wc *WebstreamClient) writeLoop() {

	defer wc.wg.Done()
	for {
		wc.lock.Lock()
		l := len(wc.buf)
		for _, d := range wc.buf {
			err := wc.c.Write(context.Background(), websocket.MessageBinary, d)
			if err != nil {
				wc.log.Error().Err(err).Msg("Error while writing to connection")
				wc.closeErr(err)
				wc.lock.Unlock()
				return
			}
		}
		wc.buf = wc.buf[:0]
		wc.lock.Unlock()
		if l == 0 {
			time.Sleep(5 * time.Second)
		} else {
			time.Sleep(time.Second)
		}
	}
}

func (wc *WebstreamClient) Push(sender uint64, data []byte) bool {
	wc.lock.Lock()
	if wc.closed {
		return true
	}
	wc.buf = append(wc.buf, data)
	wc.lock.Unlock()
	return false
}

// func (wc *WebstreamClient) Name() string {
// 	return "mocksub"
// }
